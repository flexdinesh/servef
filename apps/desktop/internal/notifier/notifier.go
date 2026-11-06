// Package notifier decides what the desktop app tells the user about projects
// that changed while they look elsewhere (decision 0099). The page closes its
// event stream when hidden, which is exactly when notices matter, so one
// Notifier per connection window holds its own streams through client.Client:
// the local daemon and a remote server are the same code. It knows no mygo: the
// app shows what it decides.
package notifier

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/flexdinesh/srcz/pkg/client"
	"github.com/flexdinesh/srcz/pkg/connection"
	"github.com/flexdinesh/srcz/pkg/contract"
)

// Defaults for Options.
const (
	// maxFollowed bounds a connection's followed projects, within the server's
	// watched cap (decision 0066).
	maxFollowed = 16
	// defaultSettle is the quiet after which edits have settled.
	defaultSettle = 10 * time.Second
	minBackoff    = time.Second
	maxBackoff    = 30 * time.Second
	// A stream open this long has recovered: the next failure starts over.
	healthyStream = 30 * time.Second
	// maxPendingPaths bounds what a folder's settling edits remember.
	maxPendingPaths = 256
)

const (
	leaseOpen    = "open"
	leaseWatched = "watched"
)

// ErrNoConnection is what Connect answers when a saved connection is gone.
var ErrNoConnection = errors.New("this connection is no longer saved")

// Notice is what to show. Everything in it from a server has been cleaned.
type Notice struct {
	// ID is stable for a connection, project, and kind.
	ID      string
	Kind    Kind
	Project string
	Title   string
	Body    string
	// Route is the web app's route and query for a click.
	Route string
}

// Options composes a Notifier.
type Options struct {
	// Connection is the connection's scheme host; Label names a remote one for
	// people, and is empty for this computer.
	Connection string
	Label      string
	// Connect returns a client of the server. failed is why the last attempt
	// ended, so a daemon that has gone can be ensured again.
	Connect func(ctx context.Context, failed error) (client.Client, error)
	// Focused reports that the connection's window has focus. Called without
	// locks held, since it may wait for the main thread.
	Focused func() bool
	// Show shows a notice, without blocking on the user.
	Show func(Notice)
	// Now and Settle are for tests; zero uses the clock and defaultSettle.
	Now    func() time.Time
	Settle time.Duration
	// MinBackoff is for tests; zero uses the default.
	MinBackoff time.Duration
}

// Notifier follows one connection's projects. It runs until Stop.
type Notifier struct {
	options Options
	ctx     context.Context
	cancel  context.CancelFunc

	mu       sync.Mutex
	settings Settings
	current  string
	// recent is the projects opened this run, newest first.
	recent    []string
	followers map[string]*follower
	limit     limiter
}

// New returns a Notifier following nothing until Configure.
func New(options Options) *Notifier {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Settle == 0 {
		options.Settle = defaultSettle
	}
	if options.MinBackoff == 0 {
		options.MinBackoff = minBackoff
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Notifier{options: options, ctx: ctx, cancel: cancel, followers: map[string]*follower{}}
}

// Stop ends every stream. A stopped server can then exit: streams count as
// requests in flight (decision 0098).
func (n *Notifier) Stop() { n.cancel() }

// Configure applies what the page says: the project it shows ("" for none) and
// the user's settings. A page load also lifts a stop after an identity failure,
// since it rereads the saved connection.
func (n *Notifier) Configure(project string, settings Settings) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ctx.Err() != nil {
		return
	}
	n.settings = settings
	n.current = project
	if project != "" {
		n.recent = slices.DeleteFunc(n.recent, func(other string) bool { return other == project })
		n.recent = slices.Insert(n.recent, 0, project)
		n.recent = n.recent[:min(len(n.recent), maxFollowed)]
	}
	n.reconcile()
}

// desiredLease is the lease project should be followed at, "" for none.
func (n *Notifier) desiredLease(project string) string {
	if project == n.current && n.settings.Edits {
		return leaseOpen
	}
	if n.settings.followsOtherProjects() {
		return leaseWatched
	}
	return ""
}

// reconcile starts, stops, and re-leases followers. Called with mu held.
func (n *Notifier) reconcile() {
	wanted := map[string]string{}
	for _, project := range n.recent {
		if lease := n.desiredLease(project); lease != "" {
			wanted[project] = lease
		}
	}
	for project, running := range n.followers {
		if _, ok := wanted[project]; !ok {
			running.cancel()
			delete(n.followers, project)
		}
	}
	for project, lease := range wanted {
		running, ok := n.followers[project]
		if !ok {
			ctx, cancel := context.WithCancel(n.ctx)
			running = &follower{notifier: n, project: project, lease: lease, cancel: cancel}
			n.followers[project] = running
			go running.run(ctx)
			continue
		}
		if running.lease != lease {
			running.lease = lease
			if running.cancelStream != nil {
				running.cancelStream()
			}
		}
	}
}

// forget stops following project, which the server does not have.
func (n *Notifier) forget(project string, running *follower) {
	n.mu.Lock()
	defer n.mu.Unlock()
	running.cancel()
	if n.followers[project] == running {
		delete(n.followers, project)
	}
	n.recent = slices.DeleteFunc(n.recent, func(other string) bool { return other == project })
}

// halt stops every follower: the server's identity or our credential is wrong,
// and retrying cannot fix that. The page shows the fix; its next Configure
// starts over.
func (n *Notifier) halt() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for project, running := range n.followers {
		running.cancel()
		delete(n.followers, project)
	}
}

// deliver shows m unless its kind is off, the user is looking at that project,
// or it is over the limits.
func (n *Notifier) deliver(project string, m message) {
	n.mu.Lock()
	wanted, current := n.settings.enabled(m.kind), n.current
	n.mu.Unlock()
	if !wanted || (project == current && n.options.Focused()) {
		return
	}
	n.mu.Lock()
	allowed := n.limit.allow(n.options.Now(), project, m.kind)
	n.mu.Unlock()
	if allowed {
		n.options.Show(addressed(m, n.options.Connection, n.options.Label, project))
	}
}

// follower holds one project's stream, across reconnects and lease changes.
// Its state is the goroutine's own but for lease and cancelStream, under the
// notifier's mu.
type follower struct {
	notifier *Notifier
	project  string
	cancel   context.CancelFunc

	lease        string
	cancelStream context.CancelFunc

	head *contract.HeadRef
	// repository is known after the first Changes; changes is its baseline.
	changes changeSet
	// walkthroughs is id to update time, nil before the first list.
	walkthroughs map[string]string
	// paths and overflow are a folder's edits since they last settled.
	paths    []string
	overflow bool
	dirty    bool
}

var errLeaseChanged = errors.New("the lease changed")

func (f *follower) run(ctx context.Context) {
	n := f.notifier
	var failed error
	delay := n.options.MinBackoff
	for ctx.Err() == nil {
		started := n.options.Now()
		err := f.follow(ctx, failed)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errLeaseChanged) {
			continue
		}
		switch {
		case identityFailed(err):
			n.halt()
			return
		case projectGone(err):
			n.forget(f.project, f)
			return
		}
		failed = err
		if n.options.Now().Sub(started) >= healthyStream {
			delay = n.options.MinBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, maxBackoff)
	}
}

func identityFailed(err error) bool {
	var refused *client.Error
	return errors.Is(err, connection.ErrFingerprint) || errors.Is(err, ErrNoConnection) ||
		(errors.As(err, &refused) && refused.Code == contract.CodeUnauthorized)
}

func projectGone(err error) bool {
	var refused *client.Error
	return errors.As(err, &refused) && refused.Kind == contract.KindNotFound
}

type streamResult struct {
	event contract.Event
	err   error
}

// follow holds one stream until it ends.
func (f *follower) follow(ctx context.Context, failed error) error {
	n := f.notifier
	server, err := n.options.Connect(ctx, failed)
	if err != nil {
		return err
	}
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	n.mu.Lock()
	lease := f.lease
	f.cancelStream = cancelStream
	n.mu.Unlock()
	stream, err := server.Events(streamCtx, contract.EventsRequest{Project: f.project, Lease: lease})
	if err != nil {
		if streamCtx.Err() != nil && ctx.Err() == nil {
			return errLeaseChanged
		}
		return err
	}
	defer stream.Close()
	results := make(chan streamResult)
	go func() {
		for {
			event, err := stream.Next()
			select {
			case results <- streamResult{event, err}:
			case <-streamCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	settle := time.NewTimer(n.options.Settle)
	settle.Stop()
	defer settle.Stop()
	for {
		select {
		case <-streamCtx.Done():
			if ctx.Err() == nil {
				return errLeaseChanged
			}
			return ctx.Err()
		case result := <-results:
			if result.err != nil {
				if streamCtx.Err() != nil && ctx.Err() == nil {
					return errLeaseChanged
				}
				return result.err
			}
			switch result.event.Name {
			case contract.EventStatus:
				f.connected(streamCtx, server, lease)
			case contract.EventHead:
				if result.event.Head != nil {
					f.moved(*result.event.Head)
				}
			case contract.EventChange:
				if result.event.Change != nil && f.edited(*result.event.Change, lease) {
					settle.Reset(n.options.Settle)
				}
			case contract.EventWalkthrough:
				f.listWalkthroughs(streamCtx, server)
			}
		case <-settle.C:
			f.settled(streamCtx, server)
		}
	}
}

// connected catches up on what a stream's absence may have missed. Every status
// after the first repeats the mode only, and costs a list.
func (f *follower) connected(ctx context.Context, server client.Client, lease string) {
	f.listWalkthroughs(ctx, server)
	if lease == leaseOpen {
		f.settled(ctx, server)
	}
}

func (f *follower) moved(next contract.HeadEvent) {
	message, ok := headMessage(f.head, next)
	f.head = &contract.HeadRef{Branch: next.Branch, Commit: next.Commit}
	if ok {
		f.notifier.deliver(f.project, message)
	}
}

// edited notes a change, and reports whether it starts or extends a settle.
func (f *follower) edited(change contract.ChangeEvent, lease string) bool {
	if lease != leaseOpen || (len(change.Paths) == 0 && !change.Overflow) {
		return false
	}
	f.dirty = true
	f.overflow = f.overflow || change.Overflow
	for _, name := range change.Paths {
		if len(f.paths) == maxPendingPaths {
			f.overflow = true
			break
		}
		if !slices.Contains(f.paths, name) {
			f.paths = append(f.paths, name)
		}
	}
	return true
}

// settled reports edits once a burst of them has stopped. The first call only
// records the baseline. A failed read leaves the edits for the next settle.
func (f *follower) settled(ctx context.Context, server client.Client) {
	changes, err := server.Changes(ctx, f.project)
	if err != nil {
		return
	}
	paths, overflow, dirty := f.paths, f.overflow, f.dirty
	f.paths, f.overflow, f.dirty = nil, false, false
	if !changes.Repository {
		if dirty {
			slices.Sort(paths)
			if message, ok := folderEditsMessage(paths, overflow); ok {
				f.notifier.deliver(f.project, message)
			}
		}
		return
	}
	current := newChangeSet(changes.Files)
	previous := f.changes
	f.changes = current
	if previous == nil {
		return
	}
	if message, ok := repositoryEditsMessage(previous, current); ok {
		f.notifier.deliver(f.project, message)
	}
}

// listWalkthroughs reports what changed since the last list; the first records
// the baseline. A list that fails is made again at the next connection.
func (f *follower) listWalkthroughs(ctx context.Context, server client.Client) {
	summaries, err := server.Walkthroughs(ctx, f.project)
	if err != nil {
		return
	}
	message, ok := walkthroughMessage(f.walkthroughs, summaries)
	f.walkthroughs = walkthroughTimes(summaries)
	if ok {
		f.notifier.deliver(f.project, message)
	}
}
