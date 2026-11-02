package watch

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// project is one followed root. Every field is guarded by the hub's mutex.
//
// At open level it watches the directories a scan reads and git state, or
// polls both. At watched level it owns only git state directories, or polls
// git state alone, and never lists the tree.
type project struct {
	hub         *Hub
	target      Target
	subscribers map[*Subscription]struct{}
	// level is the level the project runs at; zero until it starts.
	level Level
	// status is what open subscribers see; gitStatus is what watched
	// subscribers see.
	status    Status
	gitStatus Status
	stopped   bool

	// dirs and gitDirs are the directories this project owns in the hub.
	dirs       map[string]struct{}
	gitDirs    map[string]struct{}
	gitRecords func(string) bool

	pending batch
	first   time.Time
	timer   *time.Timer
	// grace stops a project without subscribers; openGrace drops an open
	// project without open subscribers to watched level.
	grace     *time.Timer
	openGrace *time.Timer
	// stopPoll ends the poller of a polling project; pollTree reports that
	// it walks the tree as well as git state.
	stopPoll chan struct{}
	pollTree bool
}

// newProject registers a project for target that follows nothing yet.
func (h *Hub) newProject(target Target) *project {
	p := &project{
		hub:         h,
		target:      target,
		subscribers: map[*Subscription]struct{}{},
		dirs:        map[string]struct{}{},
		gitDirs:     map[string]struct{}{},
		pending:     newBatch(),
	}
	h.projects[target.Root] = p
	return p
}

// open raises p to open level: watching its directories and git state when
// the watcher covers them, else polling both. A project whose directories
// cannot be listed is off at open level and keeps following git state.
func (p *project) open() {
	started := p.level != 0
	p.level = LevelOpen
	dirs, entries, err := p.target.Directories()
	if err != nil {
		p.status = Status{Mode: ModeOff, Reason: fmt.Sprintf("Couldn't list the project's directories: %v.", err)}
		if !started {
			p.startGit()
		}
		return
	}
	if reason := p.hub.overLimit(len(dirs), entries); reason != "" {
		p.pollAll(dirs, reason)
		return
	}
	// A git state poller gives way to watching git state with the tree.
	p.stopPolling()
	if err := p.watch(dirs); err != nil {
		p.pollAll(dirs, fmt.Sprintf("Watching failed: %v.", err))
		return
	}
	p.status = Status{Mode: ModeWatching}
	p.setGitStatus(p.status)
}

// startGit follows git state alone: watching its directories when the
// watcher covers them, else polling them.
func (p *project) startGit() {
	var dirs []string
	var records func(string) bool
	if p.target.Git != nil {
		dirs, records = p.target.Git()
	}
	if reason := p.hub.gitLimit(dirs); reason != "" {
		p.pollGit(reason)
		return
	}
	if err := p.watchGitDirs(dirs, records); err != nil {
		p.pollGit(fmt.Sprintf("Watching failed: %v.", err))
		return
	}
	p.setGitStatus(Status{Mode: ModeWatching})
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

// gitLimit is overLimit for git state directories, counting their entries
// only where the watcher holds a descriptor per entry.
func (h *Hub) gitLimit(dirs []string) string {
	entries := 0
	if h.perEntry {
		for _, dir := range dirs {
			if list, err := os.ReadDir(dir); err == nil {
				entries += len(list)
			}
		}
	}
	return h.overLimit(len(dirs), entries)
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
	return p.watchGitDirs(dirs, records)
}

// watchGitDirs owns exactly dirs as git state directories.
func (p *project) watchGitDirs(dirs []string, records func(string) bool) error {
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

// releaseTree gives up the tree directories, keeping git state directories.
func (p *project) releaseTree() {
	for dir := range p.dirs {
		if _, git := p.gitDirs[dir]; !git {
			p.hub.unwatchDirectory(p, dir)
		}
	}
	p.dirs = map[string]struct{}{}
}

// pollAll polls the tree and git state at open level.
func (p *project) pollAll(dirs []string, reason string) {
	p.release()
	p.startPolling(dirs, true, reason)
	p.status = Status{Mode: ModePolling, Reason: reason}
	p.setGitStatus(p.status)
}

// pollGit polls git state alone.
func (p *project) pollGit(reason string) {
	p.release()
	p.startPolling(nil, false, reason)
	p.setGitStatus(Status{Mode: ModePolling, Reason: reason})
}

// startPolling takes the first snapshot before reporting polling, so no
// change after the status goes unseen.
func (p *project) startPolling(dirs []string, tree bool, reason string) {
	p.stopPolling()
	p.stopPoll = make(chan struct{})
	p.pollTree = tree
	poller := newPoller(p.target, dirs, tree)
	go p.hub.poll(p, poller, p.stopPoll)
}

func (p *project) stopPolling() {
	if p.stopPoll != nil {
		close(p.stopPoll)
		p.stopPoll = nil
	}
	p.pollTree = false
}

// setGitStatus sets what watched subscribers see, telling them when the
// mode changes.
func (p *project) setGitStatus(status Status) {
	changed := status.Mode != p.gitStatus.Mode
	p.gitStatus = status
	if !changed {
		return
	}
	for subscription := range p.subscribers {
		if subscription.level == LevelWatched {
			subscription.setStatus(status)
		}
	}
}

// watchingTree reports that p watches its directories at open level.
func (p *project) watchingTree() bool {
	return p.level == LevelOpen && p.status.Mode == ModeWatching
}

// fallBack switches a watching open project to polling and tells
// subscribers.
func (p *project) fallBack(dirs []string, reason string) {
	p.pollAll(dirs, reason)
	for subscription := range p.subscribers {
		if subscription.level == LevelOpen {
			subscription.setStatus(p.status)
		}
	}
}

// downgrade drops an open project to watched level: it releases the tree
// and follows git state alone. Past MaxWatched it first stops a watched
// project no one subscribes to; failing that, its watched subscribers are
// off and the project stops.
func (p *project) downgrade() {
	if p.level != LevelOpen {
		return
	}
	if p.openGrace != nil {
		p.openGrace.Stop()
		p.openGrace = nil
	}
	if p.hub.count(LevelWatched) >= p.hub.options.MaxWatched && !p.hub.evictIdleWatched() {
		off := watchedOff(p.hub.options.MaxWatched)
		for subscription := range p.subscribers {
			subscription.project = nil
			subscription.setStatus(off)
		}
		p.subscribers = map[*Subscription]struct{}{}
		p.stop()
		return
	}
	p.level = LevelWatched
	p.releaseTree()
	if p.pollTree {
		p.stopPolling()
		p.startGit()
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
	if p.openGrace != nil {
		p.openGrace.Stop()
	}
	p.release()
	p.stopPolling()
	delete(p.hub.projects, p.target.Root)
}

// count is the number of subscribers at level.
func (p *project) count(level Level) int {
	n := 0
	for subscription := range p.subscribers {
		if subscription.level == level {
			n++
		}
	}
	return n
}

func (p *project) subscribe(subscription *Subscription) {
	if p.grace != nil {
		p.grace.Stop()
		p.grace = nil
	}
	p.subscribers[subscription] = struct{}{}
	subscription.project = p
	if subscription.level == LevelOpen {
		if p.openGrace != nil {
			p.openGrace.Stop()
			p.openGrace = nil
		}
		subscription.setStatus(p.status)
		return
	}
	subscription.setStatus(p.gitStatus)
	// An idle open project that only a watched subscriber returns to keeps
	// open level for the grace period alone.
	if p.level == LevelOpen && p.count(LevelOpen) == 0 {
		p.startOpenGrace()
	}
}

func (p *project) unsubscribe(subscription *Subscription) {
	delete(p.subscribers, subscription)
	if p.stopped || p.hub.closed {
		return
	}
	if len(p.subscribers) == 0 {
		if p.openGrace != nil {
			p.openGrace.Stop()
			p.openGrace = nil
		}
		var timer *time.Timer
		timer = time.AfterFunc(p.hub.options.Grace, func() {
			p.hub.mu.Lock()
			defer p.hub.mu.Unlock()
			if p.grace == timer && len(p.subscribers) == 0 {
				p.stop()
			}
		})
		p.grace = timer
		return
	}
	if subscription.level == LevelOpen && p.level == LevelOpen && p.count(LevelOpen) == 0 {
		p.startOpenGrace()
	}
}

// startOpenGrace drops p to watched level after the grace period unless an
// open subscriber returns.
func (p *project) startOpenGrace() {
	if p.openGrace != nil {
		return
	}
	var timer *time.Timer
	timer = time.AfterFunc(p.hub.options.Grace, func() {
		p.hub.mu.Lock()
		defer p.hub.mu.Unlock()
		if p.openGrace != timer || p.stopped {
			return
		}
		p.openGrace = nil
		if p.count(LevelOpen) == 0 && len(p.subscribers) > 0 {
			p.downgrade()
		}
	})
	p.openGrace = timer
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
	if p.watchingTree() {
		if change.Tree || change.Overflow {
			p.rewatch()
		}
		if change.Git && p.watchingTree() {
			if err := p.watchGit(); err != nil {
				p.fallBack(p.listedDirs(), fmt.Sprintf("Watching failed: %v.", err))
			}
		}
	} else if change.Git && p.gitStatus.Mode == ModeWatching {
		p.rewatchGit()
	}
	p.deliver(change)
}

// rewatchGit follows the git state directories that exist now, polling
// them past the watch limits or on a watch failure.
func (p *project) rewatchGit() {
	var dirs []string
	var records func(string) bool
	if p.target.Git != nil {
		dirs, records = p.target.Git()
	}
	if reason := p.hub.gitLimit(dirs); reason != "" {
		p.pollGit(reason)
		return
	}
	if err := p.watchGitDirs(dirs, records); err != nil {
		p.pollGit(fmt.Sprintf("Watching failed: %v.", err))
	}
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

// deliver sends change to open subscribers and, when git state changed,
// Change{Git: true} to watched subscribers.
func (p *project) deliver(change Change) {
	for subscription := range p.subscribers {
		switch {
		case subscription.level == LevelOpen:
			subscription.addChange(change, p.hub.options.MaxPaths)
		case change.Git:
			subscription.addChange(Change{Git: true}, p.hub.options.MaxPaths)
		}
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
