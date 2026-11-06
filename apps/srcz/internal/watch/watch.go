// Package watch reports file changes below the project roots browsers
// subscribe to (decisions 0050 and 0066). A subscription holds a project at
// a level: open follows the directories a scan reads and git state, watched
// follows git state only; a project runs at the highest level held. One
// file-system watcher serves every project, reference-counting directories
// that overlapping projects share; a project the watcher cannot cover is
// polled by size and modified time. It knows absolute paths only: callers
// decide what a project watches.
package watch

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrClosed is returned by Subscribe after Close.
var ErrClosed = errors.New("watch hub is closed")

// Mode is how a subscription learns about changes.
type Mode string

const (
	// ModeWatching receives file-system events.
	ModeWatching Mode = "watching"
	// ModePolling compares size and modified time on an interval.
	ModePolling Mode = "polling"
	// ModeOff receives nothing.
	ModeOff Mode = "off"
)

// Status is a subscription's mode and, unless watching, why.
type Status struct {
	Mode   Mode
	Reason string
}

// Change is one batch of changes in a project.
type Change struct {
	// Paths are the changed root-relative slash paths, sorted.
	Paths []string
	// Tree reports that an entry was created, removed, or renamed.
	Tree bool
	// Git reports that HEAD, the index, or a ref changed.
	Git bool
	// Overflow reports that more changed than Paths lists.
	Overflow bool
}

// Level is how much of a project a subscription follows (decision 0066). A
// project runs at the highest level its subscriptions hold; when the last
// subscription at a level ends, it keeps that level for the grace period.
type Level int

const (
	// LevelWatched follows git state only: HEAD, the index, and refs.
	LevelWatched Level = iota + 1
	// LevelOpen follows the directories a scan reads and git state.
	LevelOpen
)

// Target is what one project watches.
type Target struct {
	// Root is the canonical project root; it keys the project.
	Root string
	// Directories lists the absolute directories to watch, root first, and
	// counts the entries they hold. It is called on start and after the tree
	// changes.
	Directories func() ([]string, int, error)
	// Git lists the absolute directories holding git state and recognizes
	// its files; records is nil for a folder. Nil watches no git state.
	Git func() (dirs []string, records func(name string) bool)
}

// Options bound the hub.
type Options struct {
	// Debounce is the quiet time before a batch is sent; MaxWait caps how
	// long continuous changes delay one.
	Debounce time.Duration
	MaxWait  time.Duration
	// Grace keeps a project at a level after its last subscriber at that
	// level leaves.
	Grace time.Duration
	// PollInterval is the shortest poll interval; polls back off to four
	// times their walk, up to MaxPollInterval.
	PollInterval    time.Duration
	MaxPollInterval time.Duration
	// MaxDirectories caps one project's watched directories; MaxEntries caps
	// their entries where the watcher holds one descriptor per entry
	// (kqueue). Larger projects poll. At watched level they cap the git
	// state directories.
	MaxDirectories int
	MaxEntries     int
	// MaxProjects caps the projects followed at open level, including those
	// in their open grace period; more open subscriptions are off.
	MaxProjects int
	// MaxWatched caps the projects held only by watched subscriptions; more
	// watched subscriptions are off.
	MaxWatched int
	// MaxPaths caps the paths of one Change.
	MaxPaths int
}

// DefaultOptions are decisions 0050 and 0066's limits.
func DefaultOptions() Options {
	return Options{
		Debounce:        500 * time.Millisecond,
		MaxWait:         2 * time.Second,
		Grace:           30 * time.Second,
		PollInterval:    2 * time.Second,
		MaxPollInterval: 30 * time.Second,
		MaxDirectories:  4096,
		MaxEntries:      8192,
		MaxProjects:     8,
		MaxWatched:      16,
		MaxPaths:        256,
	}
}

// Hub follows the subscribed projects.
type Hub struct {
	options    Options
	newBackend func(func(event), func()) (backend, error)
	perEntry   bool

	mu       sync.Mutex
	closed   bool
	backend  backend
	projects map[string]*project
	// owners are the projects watching each directory.
	owners        map[string]map[*project]struct{}
	subscriptions map[*Subscription]struct{}
}

// NewHub returns a hub that watches nothing until a subscription.
func NewHub(options Options) *Hub {
	return &Hub{
		options:       options,
		newBackend:    newNotifyBackend,
		perEntry:      perEntryWatches,
		projects:      map[string]*project{},
		owners:        map[string]map[*project]struct{}{},
		subscriptions: map[*Subscription]struct{}{},
	}
}

// Subscription receives one project's status and changes at its level: a
// watched subscription receives only git changes, as Change{Git: true}.
// Pending updates merge until Next takes them, so a slow reader loses
// nothing but detail.
type Subscription struct {
	hub     *Hub
	project *project
	level   Level
	ready   chan struct{}
	done    chan struct{}
	status  *Status
	change  *Change
	closed  bool
}

// Ready receives when Next has something.
func (s *Subscription) Ready() <-chan struct{} { return s.ready }

// Done is closed when the hub closes.
func (s *Subscription) Done() <-chan struct{} { return s.done }

// Wake makes Ready receive, for events its owner keeps beside this
// subscription's own.
func (s *Subscription) Wake() { s.signal() }

// Next takes the pending status and change; either may be nil.
func (s *Subscription) Next() (*Status, *Change) {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	status, change := s.status, s.change
	s.status, s.change = nil, nil
	return status, change
}

// Close ends the subscription. A project without subscribers at a level
// leaves that level after the grace period.
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	delete(s.hub.subscriptions, s)
	if s.project != nil {
		s.project.unsubscribe(s)
	}
}

func (s *Subscription) setStatus(status Status) {
	s.status = &status
	s.signal()
}

func (s *Subscription) addChange(change Change, maxPaths int) {
	if s.change == nil {
		s.change = &Change{}
	}
	merged := newBatch()
	merged.add(*s.change, maxPaths)
	merged.add(change, maxPaths)
	next := merged.change()
	s.change = &next
	s.signal()
}

func (s *Subscription) signal() {
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

// Subscribe follows target's project at level, starting it unless it
// already runs and raising it to open for an open subscription. The first
// update is the status at the subscription's level.
func (h *Hub) Subscribe(target Target, level Level) (*Subscription, error) {
	if level != LevelWatched && level != LevelOpen {
		return nil, fmt.Errorf("unknown watch level %d", level)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	subscription := &Subscription{hub: h, level: level, ready: make(chan struct{}, 1), done: make(chan struct{})}
	h.subscriptions[subscription] = struct{}{}
	p, ok := h.projects[target.Root]
	switch {
	case level == LevelOpen && (!ok || p.level != LevelOpen):
		if h.count(LevelOpen) >= h.options.MaxProjects && !h.evictIdle() {
			subscription.setStatus(Status{Mode: ModeOff, Reason: fmt.Sprintf("Live updates already follow %d projects.", h.options.MaxProjects)})
			return subscription, nil
		}
		// Eviction may have stopped this project while it idled.
		if p, ok = h.projects[target.Root]; !ok {
			p = h.newProject(target)
		}
		p.open()
	case !ok:
		if h.count(LevelWatched) >= h.options.MaxWatched && !h.evictIdleWatched() {
			subscription.setStatus(watchedOff(h.options.MaxWatched))
			return subscription, nil
		}
		p = h.newProject(target)
		p.level = LevelWatched
		p.startGit()
	}
	p.subscribe(subscription)
	return subscription, nil
}

// watchedOff is the status of a watched subscription past MaxWatched.
func watchedOff(limit int) Status {
	return Status{Mode: ModeOff, Reason: fmt.Sprintf("Live updates already watch the git state of %d projects.", limit)}
}

// Close stops every project and ends every subscription.
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	for subscription := range h.subscriptions {
		close(subscription.done)
	}
	for _, p := range h.projects {
		p.stop()
	}
	b := h.backend
	h.backend = nil
	h.mu.Unlock()
	if b != nil {
		_ = b.Close()
	}
}

// count is the number of projects running at level.
func (h *Hub) count(level Level) int {
	n := 0
	for _, p := range h.projects {
		if p.level == level {
			n++
		}
	}
	return n
}

// evictIdle frees an open slot: it stops an open project no one subscribes
// to or, failing that, drops an open project held only by watched
// subscriptions to watched level without waiting for its grace period.
func (h *Hub) evictIdle() bool {
	for _, p := range h.projects {
		if p.level == LevelOpen && len(p.subscribers) == 0 {
			p.stop()
			return true
		}
	}
	for _, p := range h.projects {
		if p.level == LevelOpen && p.count(LevelOpen) == 0 {
			p.downgrade()
			return true
		}
	}
	return false
}

// evictIdleWatched frees a watched slot by stopping a watched project no
// one subscribes to.
func (h *Hub) evictIdleWatched() bool {
	for _, p := range h.projects {
		if p.level == LevelWatched && len(p.subscribers) == 0 {
			p.stop()
			return true
		}
	}
	return false
}

// watchDirectory adds p as an owner of dir, watching it on first use.
func (h *Hub) watchDirectory(p *project, dir string) error {
	owners, ok := h.owners[dir]
	if !ok {
		if h.backend == nil {
			b, err := h.newBackend(h.dispatch, h.overflow)
			if err != nil {
				return err
			}
			h.backend = b
		}
		if err := h.backend.Add(dir); err != nil {
			return err
		}
		owners = map[*project]struct{}{}
		h.owners[dir] = owners
	}
	owners[p] = struct{}{}
	return nil
}

// unwatchDirectory drops p as an owner of dir, unwatching it when unowned.
// The backend closes with the last directory.
func (h *Hub) unwatchDirectory(p *project, dir string) {
	owners, ok := h.owners[dir]
	if !ok {
		return
	}
	delete(owners, p)
	if len(owners) > 0 {
		return
	}
	delete(h.owners, dir)
	if h.backend == nil {
		return
	}
	_ = h.backend.Remove(dir)
	if len(h.owners) == 0 {
		b := h.backend
		h.backend = nil
		// Closing waits for the backend's reader, which may wait for h.mu.
		go func() { _ = b.Close() }()
	}
}

// dispatch routes one event to the projects watching its directory.
func (h *Hub) dispatch(e event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for p := range h.owners[parentDir(e.name)] {
		p.record(e)
	}
}

// overflow reports lost events to every watching project. A lost event may
// have been a git change.
func (h *Hub) overflow() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range h.projects {
		if p.watchingTree() || p.gitStatus.Mode == ModeWatching {
			p.pending.overflow = true
			p.pending.git = p.pending.git || p.gitRecords != nil
			p.schedule()
		}
	}
}
