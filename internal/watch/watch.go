// Package watch reports file changes below the project roots browsers
// subscribe to (decision 0050). One file-system watcher serves every
// project, reference-counting directories that overlapping projects share;
// a project the watcher cannot cover is polled by size and modified time.
// It knows absolute paths only: callers decide what a project watches.
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
	// Grace keeps a project watched after its last subscriber leaves.
	Grace time.Duration
	// PollInterval is the shortest poll interval; polls back off to four
	// times their walk, up to MaxPollInterval.
	PollInterval    time.Duration
	MaxPollInterval time.Duration
	// MaxDirectories caps one project's watched directories; MaxEntries caps
	// their entries where the watcher holds one descriptor per entry
	// (kqueue). Larger projects poll.
	MaxDirectories int
	MaxEntries     int
	// MaxProjects caps the projects followed at once; more are off.
	MaxProjects int
	// MaxPaths caps the paths of one Change.
	MaxPaths int
}

// DefaultOptions are decision 0050's limits.
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

// Subscription receives one project's status and changes. Pending updates
// merge until Next takes them, so a slow reader loses nothing but detail.
type Subscription struct {
	hub     *Hub
	project *project
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

// Next takes the pending status and change; either may be nil.
func (s *Subscription) Next() (*Status, *Change) {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	status, change := s.status, s.change
	s.status, s.change = nil, nil
	return status, change
}

// Close ends the subscription. A project without subscribers stops after
// the grace period.
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

// Subscribe follows target's project, starting it unless it already runs.
// The first update is the project's status.
func (h *Hub) Subscribe(target Target) (*Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	subscription := &Subscription{hub: h, ready: make(chan struct{}, 1), done: make(chan struct{})}
	h.subscriptions[subscription] = struct{}{}
	p, ok := h.projects[target.Root]
	if !ok {
		if len(h.projects) >= h.options.MaxProjects && !h.evictIdle() {
			subscription.setStatus(Status{Mode: ModeOff, Reason: fmt.Sprintf("Live updates already follow %d projects.", h.options.MaxProjects)})
			return subscription, nil
		}
		p = h.startProject(target)
	}
	p.subscribe(subscription)
	return subscription, nil
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

// evictIdle stops a project no one subscribes to, freeing its slot.
func (h *Hub) evictIdle() bool {
	for _, p := range h.projects {
		if len(p.subscribers) == 0 {
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

// overflow reports lost events to every watching project.
func (h *Hub) overflow() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range h.projects {
		if p.status.Mode == ModeWatching {
			p.pending.overflow = true
			p.schedule()
		}
	}
}
