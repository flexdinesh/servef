// Package session hosts any mygo view headless, in a ui.Tester, and drives
// it through mygo's automation protocol (plugins/automation) over a Unix
// socket, so an agent sees and acts on the app on a machine without a
// display. The same client and CLI drive a built app's MYGO_AUTOMATION
// socket.
//
// ui.Tester has no public accessibility tree, so the session builds its
// tree from Tester.Texts and Tester.Find: one node per text the frame shows,
// role "text", with the box of the first element showing it. Texts that
// repeat are listed, but only the first can be acted on by ID; the open
// context menu's items follow as role "menuItem". Nodes carry no key (the
// Tester exposes none), so act takes an id; a key answers "unsupported".
//
// Unlike plugins/automation's idle, which waits for frames only, the
// session's idle also waits for work the host started with Go, applies it
// posted, and its pumps: the fetch a click started has landed when idle
// answers. Against a built app, wait for a text instead.
//
// Every Tester call runs on the session's own goroutine. Hosts hand work to
// it with Go and Post (Session is a runner: Go runs work on a goroutine and
// its apply function on the session's), or with pumps for queues of their
// own; idle waits until all of them are drained and no frame is pending.
package session

import (
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/packages/mygo/agent/protocol"
)

// Options configure a session.
type Options struct {
	// View builds the view to host. It runs on the session's goroutine,
	// after the session exists, so the view may use the session as its
	// runner.
	View func(s *Session) func(c *ui.Context)
	// Width and Height are the window's size in DIPs; 1100×700 when zero.
	Width, Height int
	// Dark starts in the dark appearance.
	Dark bool
	// Title reports the window title; "mygo" when nil.
	Title func() string
	// Pumps run on the session's goroutine before every frame and while
	// idle waits; each reports whether it did work (then the session draws).
	Pumps []func() bool
}

// Session is a headless window.
type Session struct {
	opts  Options
	tt    *ui.Tester
	w, h  int
	calls chan func()
	wake  chan struct{}

	mu       sync.Mutex
	posts    []func()
	inflight atomic.Int64
	pumps    []func() bool

	wall     time.Time
	done     chan struct{}
	exited   chan struct{}
	stopOnce sync.Once
}

// tick is how often pumps run while no request comes.
const tick = 50 * time.Millisecond

// New starts a session and builds its view.
func New(opts Options) *Session {
	if opts.Width <= 0 {
		opts.Width = 1100
	}
	if opts.Height <= 0 {
		opts.Height = 700
	}
	s := &Session{opts: opts, w: opts.Width, h: opts.Height, calls: make(chan func()), wake: make(chan struct{}, 1), done: make(chan struct{}), exited: make(chan struct{}), pumps: opts.Pumps}
	ready := make(chan struct{})
	go s.loop(ready)
	<-ready
	return s
}

func (s *Session) loop(ready chan struct{}) {
	defer close(s.exited)
	view := s.opts.View(s)
	s.tt = ui.NewTester(view, s.w, s.h)
	if s.opts.Dark {
		s.tt.SetDark(true)
	}
	s.wall = time.Now()
	s.settle()
	close(ready)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case call := <-s.calls:
			s.sync()
			call()
		case <-s.wake:
			if s.pump() {
				s.tt.Frame()
			}
		case <-ticker.C:
			if s.pump() {
				s.tt.Frame()
			}
		}
	}
}

// Do runs fn on the session's goroutine with its tester and waits for it.
func (s *Session) Do(fn func(tt *ui.Tester)) error {
	finished := make(chan struct{})
	select {
	case s.calls <- func() { defer close(finished); fn(s.tt) }:
	case <-s.done:
		return errors.New("session: closed")
	}
	<-finished
	return nil
}

// Go runs work on a goroutine and then its apply function, if any, on the
// session's goroutine; idle waits for both.
func (s *Session) Go(work func() func()) {
	s.inflight.Add(1)
	go func() {
		defer s.inflight.Add(-1)
		if apply := work(); apply != nil {
			s.Post(apply)
		}
	}()
}

// Post runs apply on the session's goroutine, then draws.
func (s *Session) Post(apply func()) {
	s.mu.Lock()
	s.posts = append(s.posts, apply)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// AddPump adds a hook that runs before frames; see Options.Pumps.
func (s *Session) AddPump(pump func() bool) {
	s.mu.Lock()
	s.pumps = append(s.pumps, pump)
	s.mu.Unlock()
}

// Close stops the session.
func (s *Session) Close() { s.stopOnce.Do(func() { close(s.done) }) }

// Done is closed when the session stops.
func (s *Session) Done() <-chan struct{} { return s.done }

// Exited is closed once the session's goroutine returned: from then on,
// what only it touched (the view's state) is the caller's.
func (s *Session) Exited() <-chan struct{} { return s.exited }

// pump runs posted applies and pumps until none does work; it reports
// whether any did.
func (s *Session) pump() bool {
	ran := false
	for range 100 {
		s.mu.Lock()
		posts := s.posts
		s.posts = nil
		pumps := append([]func() bool(nil), s.pumps...)
		s.mu.Unlock()
		did := len(posts) > 0
		for _, apply := range posts {
			apply()
		}
		for _, p := range pumps {
			if p() {
				did = true
			}
		}
		if !did {
			return ran
		}
		ran = true
	}
	return ran
}

func (s *Session) pending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.posts) > 0 || s.inflight.Load() > 0
}

// sync moves the tester's clock on with the wall clock, at most a second at
// a time, so that clicks of separate requests are no double click and
// tooltips and animations follow real time.
func (s *Session) sync() {
	now := time.Now()
	elapsed := now.Sub(s.wall)
	s.wall = now
	if elapsed > 0 {
		s.tt.Advance(min(elapsed, time.Second))
	}
}

// settle drains work and draws until nothing is pending, for at most 50
// rounds; it does not wait for work in flight.
func (s *Session) settle() {
	for range 50 {
		ran := s.pump()
		if ran {
			s.tt.Frame()
		}
		if !ran && !s.tt.FramePending() {
			return
		}
		if s.tt.FramePending() {
			s.tt.Advance(16 * time.Millisecond)
		}
	}
}

// idle waits until no work is in flight or posted, pumps do nothing, and
// the view asks for no frame; timeout 0 waits for ever.
func (s *Session) idle(timeout time.Duration) error {
	var deadline <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		deadline = timer.C
	}
	for {
		ran := s.pump()
		if ran {
			s.tt.Frame()
		}
		framing := s.tt.FramePending()
		if !ran && !framing && !s.pending() {
			return nil
		}
		if framing {
			s.tt.Advance(16 * time.Millisecond)
		}
		select {
		case <-deadline:
			return protocol.Errorf(protocol.CodeTimeout, "the window did not settle in %v", timeout)
		case <-s.done:
			return protocol.Errorf(protocol.CodeFailed, "the session closed")
		case <-s.wake:
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (s *Session) title() string {
	if s.opts.Title != nil {
		return s.opts.Title()
	}
	return "mygo"
}

// Tree returns the elements of the last frame.
func (s *Session) Tree() protocol.TreeResult {
	var tree protocol.TreeResult
	_ = s.Do(func(*ui.Tester) { tree = s.tree() })
	return tree
}

// element is a node of the tree with what acting on it needs.
type element struct {
	node       protocol.Node
	occurrence int
	menu       bool
}

func (s *Session) elements() []element {
	var out []element
	seen := map[string]int{}
	for _, text := range s.tt.Texts() {
		n := seen[text]
		seen[text]++
		node := protocol.Node{ID: nodeID("text", text, n), Role: "text", Label: text}
		if n == 0 {
			if r, ok := s.tt.Find(text); ok {
				node.Bounds = protocol.Rect{X: float64(r.X), Y: float64(r.Y), Width: float64(r.W), Height: float64(r.H)}
				x, y := node.Bounds.Center()
				if x >= 0 && y >= 0 && x < float64(s.w) && y < float64(s.h) {
					node.Actions = []string{"press", "focus", "setValue"}
				} else {
					node.States = []string{"offscreen"}
				}
			}
		} else {
			node.Description = "repeats text " + strconv.Quote(text) + " (occurrence " + strconv.Itoa(n+1) + "); act on its position instead"
		}
		out = append(out, element{node: node, occurrence: n})
	}
	for i, item := range s.tt.Menu() {
		if item == "-" {
			continue
		}
		out = append(out, element{node: protocol.Node{ID: nodeID("menu", item, i), Role: "menuItem", Label: item, Actions: []string{"press"}}, menu: true})
	}
	return out
}

func (s *Session) tree() protocol.TreeResult {
	elements := s.elements()
	tree := protocol.TreeResult{Window: 1, Nodes: make([]protocol.Node, 0, len(elements))}
	for _, e := range elements {
		tree.Nodes = append(tree.Nodes, e.node)
	}
	tree.Announcements = s.tt.Announcements()
	return tree
}

// nodeID is stable for a text and its occurrence, so IDs last across
// frames that show the same texts in the same order.
func nodeID(kind, text string, n int) protocol.NodeID {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%d", kind, text, n)
	id := h.Sum64()
	if id == 0 {
		id = 1
	}
	return protocol.NodeID(id)
}

// Listen listens for clients on a Unix socket at path, mode 0600. A stale
// socket left at path is replaced; a live one, or a file that is no socket,
// is an error.
func Listen(path string) (net.Listener, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("session: %s exists and is not a socket", path)
		}
		if c, err := net.DialTimeout("unix", path, 200*time.Millisecond); err == nil {
			_ = c.Close()
			return nil, fmt.Errorf("session: another process listens on %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	// The user's alone from the start, as plugins/automation makes its
	// socket: the umask is the process's, so this briefly narrows it for
	// files other goroutines create.
	old := syscall.Umask(0o177)
	l, err := net.Listen("unix", path)
	syscall.Umask(old)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}

// ListenAndServe listens at path and serves until the session closes.
// Closing the listener unlinks the socket it made, and only that one.
func (s *Session) ListenAndServe(path string) error {
	l, err := Listen(path)
	if err != nil {
		return err
	}
	defer l.Close()
	go func() {
		<-s.done
		_ = l.Close()
	}()
	err = s.Serve(l)
	select {
	case <-s.done:
		return nil
	default:
		return err
	}
}
