package watch

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// project is one followed root. Every field is guarded by the hub's mutex.
type project struct {
	hub         *Hub
	target      Target
	subscribers map[*Subscription]struct{}
	status      Status
	stopped     bool

	// dirs and gitDirs are the directories this project owns in the hub.
	dirs       map[string]struct{}
	gitDirs    map[string]struct{}
	gitRecords func(string) bool

	pending batch
	first   time.Time
	timer   *time.Timer
	grace   *time.Timer
	// stopPoll ends the poller of a polling project.
	stopPoll chan struct{}
}

// startProject follows target: watching when the watcher covers it, else
// polling, else off.
func (h *Hub) startProject(target Target) *project {
	p := &project{
		hub:         h,
		target:      target,
		subscribers: map[*Subscription]struct{}{},
		dirs:        map[string]struct{}{},
		gitDirs:     map[string]struct{}{},
		pending:     newBatch(),
	}
	h.projects[target.Root] = p
	dirs, entries, err := target.Directories()
	if err != nil {
		p.status = Status{Mode: ModeOff, Reason: fmt.Sprintf("Couldn't list the project's directories: %v.", err)}
		return p
	}
	if reason := h.overLimit(len(dirs), entries); reason != "" {
		p.startPolling(dirs, reason)
		return p
	}
	if err := p.watch(dirs); err != nil {
		p.release()
		p.startPolling(dirs, fmt.Sprintf("Watching failed: %v.", err))
		return p
	}
	p.status = Status{Mode: ModeWatching}
	return p
}

// overLimit explains why a project of this size polls, or is empty.
func (h *Hub) overLimit(dirs, entries int) string {
	if dirs > h.options.MaxDirectories {
		return fmt.Sprintf("%d directories exceed the %d-directory watch limit.", dirs, h.options.MaxDirectories)
	}
	if h.perEntry && entries > h.options.MaxEntries {
		return fmt.Sprintf("%d entries exceed the %d-entry watch limit.", entries, h.options.MaxEntries)
	}
	return ""
}

// watch owns dirs and the git state directories in the hub.
func (p *project) watch(dirs []string) error {
	for _, dir := range dirs {
		if err := p.hub.watchDirectory(p, dir); err != nil {
			return err
		}
		p.dirs[dir] = struct{}{}
	}
	return p.watchGit()
}

func (p *project) watchGit() error {
	if p.target.Git == nil {
		return nil
	}
	dirs, records := p.target.Git()
	p.gitRecords = records
	next := map[string]struct{}{}
	for _, dir := range dirs {
		next[dir] = struct{}{}
	}
	for dir := range p.gitDirs {
		if _, keep := next[dir]; !keep {
			p.hub.unwatchDirectory(p, dir)
			delete(p.gitDirs, dir)
		}
	}
	for _, dir := range dirs {
		if _, ok := p.gitDirs[dir]; ok {
			continue
		}
		if err := p.hub.watchDirectory(p, dir); err != nil {
			return err
		}
		p.gitDirs[dir] = struct{}{}
	}
	return nil
}

// release gives up every directory this project owns.
func (p *project) release() {
	for dir := range p.dirs {
		p.hub.unwatchDirectory(p, dir)
	}
	for dir := range p.gitDirs {
		p.hub.unwatchDirectory(p, dir)
	}
	p.dirs = map[string]struct{}{}
	p.gitDirs = map[string]struct{}{}
}

// startPolling takes the first snapshot before reporting polling, so no
// change after the status goes unseen.
func (p *project) startPolling(dirs []string, reason string) {
	p.status = Status{Mode: ModePolling, Reason: reason}
	p.stopPoll = make(chan struct{})
	poller := newPoller(p.target, dirs)
	go p.hub.poll(p, poller, p.stopPoll)
}

// fallBack switches a watching project to polling and tells subscribers.
func (p *project) fallBack(dirs []string, reason string) {
	p.release()
	p.startPolling(dirs, reason)
	for subscription := range p.subscribers {
		subscription.setStatus(p.status)
	}
}

func (p *project) stop() {
	if p.stopped {
		return
	}
	p.stopped = true
	if p.timer != nil {
		p.timer.Stop()
	}
	if p.grace != nil {
		p.grace.Stop()
	}
	p.release()
	if p.stopPoll != nil {
		close(p.stopPoll)
	}
	delete(p.hub.projects, p.target.Root)
}

func (p *project) subscribe(subscription *Subscription) {
	if p.grace != nil {
		p.grace.Stop()
		p.grace = nil
	}
	p.subscribers[subscription] = struct{}{}
	subscription.project = p
	subscription.setStatus(p.status)
}

func (p *project) unsubscribe(subscription *Subscription) {
	delete(p.subscribers, subscription)
	if len(p.subscribers) > 0 || p.stopped || p.hub.closed {
		return
	}
	p.grace = time.AfterFunc(p.hub.options.Grace, func() {
		p.hub.mu.Lock()
		defer p.hub.mu.Unlock()
		if len(p.subscribers) == 0 {
			p.stop()
		}
	})
}

// record adds one watcher event to the pending batch.
func (p *project) record(e event) {
	if p.stopped || e.op == opChmod {
		return
	}
	dir := filepath.Dir(e.name)
	if _, ok := p.gitDirs[dir]; ok {
		if p.gitRecords != nil && p.gitRecords(e.name) {
			p.pending.git = true
			p.schedule()
		}
		return
	}
	if _, ok := p.dirs[dir]; !ok {
		return
	}
	rel, ok := relative(p.target.Root, e.name)
	if !ok || path.Base(rel) == ".git" {
		return
	}
	p.pending.addPath(rel, p.hub.options.MaxPaths)
	if e.op&(opCreate|opRemove|opRename) != 0 {
		p.pending.tree = true
	}
	p.schedule()
}

// schedule sends the pending batch after Debounce of quiet, or MaxWait
// after its first change.
func (p *project) schedule() {
	now := time.Now()
	if p.timer == nil {
		p.first = now
		p.timer = time.AfterFunc(p.hub.options.Debounce, func() {
			p.hub.mu.Lock()
			defer p.hub.mu.Unlock()
			p.flush()
		})
		return
	}
	wait := p.hub.options.Debounce
	if remaining := p.first.Add(p.hub.options.MaxWait).Sub(now); remaining < wait {
		wait = max(remaining, 0)
	}
	p.timer.Reset(wait)
}

// flush sends the pending batch, first rewatching what it changed.
func (p *project) flush() {
	if p.stopped {
		return
	}
	p.timer = nil
	if p.pending.empty() {
		return
	}
	change := p.pending.change()
	p.pending = newBatch()
	if p.status.Mode == ModeWatching {
		if change.Tree || change.Overflow {
			p.rewatch()
		}
		if change.Git && p.status.Mode == ModeWatching {
			if err := p.watchGit(); err != nil {
				p.fallBack(p.listedDirs(), fmt.Sprintf("Watching failed: %v.", err))
			}
		}
	}
	p.deliver(change)
}

// rewatch follows the directories the tree holds now.
func (p *project) rewatch() {
	dirs, entries, err := p.target.Directories()
	if err != nil {
		return
	}
	if reason := p.hub.overLimit(len(dirs), entries); reason != "" {
		p.fallBack(dirs, reason)
		return
	}
	next := map[string]struct{}{}
	for _, dir := range dirs {
		next[dir] = struct{}{}
	}
	for dir := range p.dirs {
		if _, keep := next[dir]; !keep {
			p.hub.unwatchDirectory(p, dir)
			delete(p.dirs, dir)
		}
	}
	for _, dir := range dirs {
		if _, ok := p.dirs[dir]; ok {
			continue
		}
		if err := p.hub.watchDirectory(p, dir); err != nil {
			p.fallBack(dirs, fmt.Sprintf("Watching failed: %v.", err))
			return
		}
		p.dirs[dir] = struct{}{}
	}
}

func (p *project) listedDirs() []string {
	dirs := make([]string, 0, len(p.dirs))
	for dir := range p.dirs {
		dirs = append(dirs, dir)
	}
	return dirs
}

func (p *project) deliver(change Change) {
	for subscription := range p.subscribers {
		subscription.addChange(change, p.hub.options.MaxPaths)
	}
}

// batch accumulates changes; past the path cap it only reports overflow.
type batch struct {
	paths    map[string]struct{}
	tree     bool
	git      bool
	overflow bool
}

func newBatch() batch { return batch{paths: map[string]struct{}{}} }

func (b *batch) addPath(rel string, maxPaths int) {
	if _, ok := b.paths[rel]; ok {
		return
	}
	if len(b.paths) >= maxPaths {
		b.overflow = true
		return
	}
	b.paths[rel] = struct{}{}
}

func (b *batch) add(change Change, maxPaths int) {
	for _, rel := range change.Paths {
		b.addPath(rel, maxPaths)
	}
	b.tree = b.tree || change.Tree
	b.git = b.git || change.Git
	b.overflow = b.overflow || change.Overflow
}

func (b batch) empty() bool {
	return len(b.paths) == 0 && !b.tree && !b.git && !b.overflow
}

func (b batch) change() Change {
	paths := make([]string, 0, len(b.paths))
	for rel := range b.paths {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return Change{Paths: paths, Tree: b.tree, Git: b.git, Overflow: b.overflow}
}

// relative returns name's slash path below root.
func relative(root, name string) (string, bool) {
	rel, err := filepath.Rel(root, name)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
