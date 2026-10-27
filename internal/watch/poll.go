package watch

import (
	"os"
	"path/filepath"
	"time"
)

// stamp is what a poll compares. Directories compare by presence only: their
// modified time changes with their children, which are compared themselves.
type stamp struct {
	size     int64
	modified time.Time
	dir      bool
}

// snapshot stamps the entries of dirs and the git state files in gitDirs.
type snapshot struct {
	tree map[string]stamp
	git  map[string]stamp
}

func takeSnapshot(dirs, gitDirs []string, records func(string) bool) snapshot {
	s := snapshot{tree: map[string]stamp{}, git: map[string]stamp{}}
	stampDir := func(dir string, into map[string]stamp, keep func(string) bool) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			name := filepath.Join(dir, entry.Name())
			if entry.Name() == ".git" || !keep(name) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.IsDir() {
				into[name] = stamp{dir: true}
				continue
			}
			into[name] = stamp{size: info.Size(), modified: info.ModTime()}
		}
	}
	for _, dir := range dirs {
		stampDir(dir, s.tree, func(string) bool { return true })
	}
	if records != nil {
		for _, dir := range gitDirs {
			stampDir(dir, s.git, records)
		}
	}
	return s
}

// compare reports what changed from previous to current below root.
func compare(previous, current snapshot, root string, maxPaths int) batch {
	b := newBatch()
	for name, now := range current.tree {
		before, ok := previous.tree[name]
		if ok && before == now {
			continue
		}
		if rel, inside := relative(root, name); inside {
			b.addPath(rel, maxPaths)
		}
		if !ok || before.dir != now.dir {
			b.tree = true
		}
	}
	for name := range previous.tree {
		if _, ok := current.tree[name]; ok {
			continue
		}
		if rel, inside := relative(root, name); inside {
			b.addPath(rel, maxPaths)
		}
		b.tree = true
	}
	if len(previous.git) != len(current.git) {
		b.git = true
	}
	for name, now := range current.git {
		if before, ok := previous.git[name]; !ok || before != now {
			b.git = true
		}
	}
	return b
}

// poller is one polling project's walk state, owned by its goroutine.
type poller struct {
	target   Target
	dirs     []string
	gitDirs  []string
	records  func(string) bool
	previous snapshot
}

func newPoller(target Target, dirs []string) *poller {
	p := &poller{target: target, dirs: dirs}
	p.readGit()
	p.previous = p.snapshot()
	return p
}

func (p *poller) readGit() {
	if p.target.Git != nil {
		p.gitDirs, p.records = p.target.Git()
	}
}

func (p *poller) snapshot() snapshot { return takeSnapshot(p.dirs, p.gitDirs, p.records) }

// poll follows p by snapshots until stop closes. Each interval is four
// times the last walk, between PollInterval and MaxPollInterval, so a slow
// tree costs at most a quarter of a core.
func (h *Hub) poll(p *project, walker *poller, stop <-chan struct{}) {
	interval := h.options.PollInterval
	for {
		timer := time.NewTimer(interval)
		select {
		case <-stop:
			timer.Stop()
			return
		case <-timer.C:
		}
		started := time.Now()
		current := walker.snapshot()
		changed := compare(walker.previous, current, walker.target.Root, h.options.MaxPaths)
		if changed.tree || changed.git {
			// New or removed directories and refs change what is walked;
			// the next poll compares against the relisted set.
			if listed, _, err := walker.target.Directories(); err == nil {
				walker.dirs = listed
			}
			walker.readGit()
			current = walker.snapshot()
		}
		walker.previous = current
		interval = min(max(4*time.Since(started), h.options.PollInterval), h.options.MaxPollInterval)
		if changed.empty() {
			continue
		}
		h.mu.Lock()
		if !p.stopped {
			p.deliver(changed.change())
		}
		h.mu.Unlock()
	}
}
