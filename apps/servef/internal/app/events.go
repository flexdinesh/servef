package app

import (
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/git"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/watch"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Events follows projects for live updates (decisions 0050, 0066).
type Events struct{ *state }

// Subscription is one follower of a project. The subscription is the
// project's interest: the hub watches a project only while some subscription
// follows it. Close it when done.
type Subscription struct {
	watch *watch.Subscription
	root  string
	// last is the head this subscription last reported. A folder, or a
	// checkout whose head can't be read, reports none.
	last     *contract.HeadRef
	baseline bool
}

// Subscribe follows request.Project at its lease level. The lease is checked
// before the project: it needs no file system.
func (e *Events) Subscribe(request contract.EventsRequest) (*Subscription, error) {
	level, ok := leaseLevel(request.Lease)
	if !ok {
		return nil, &Error{Kind: contract.KindInvalid, Code: contract.CodeInvalidLease, Message: "lease must be open or watched."}
	}
	project, err := ResolveProject(e.config.Roots.List(), request.Project)
	if err != nil {
		return nil, err
	}
	if e.config.Watches == nil {
		return nil, fail(contract.KindUnavailable, "Live updates are unavailable.")
	}
	subscription, err := e.config.Watches.Subscribe(e.watchTarget(project.Root), level)
	if err != nil {
		return nil, fail(contract.KindUnavailable, "Live updates are unavailable.")
	}
	return &Subscription{watch: subscription, root: project.Root}, nil
}

// leaseLevel parses a lease: open (the default) follows files and git state;
// watched follows git state only (decision 0066).
func leaseLevel(value string) (watch.Level, bool) {
	switch value {
	case "", "open":
		return watch.LevelOpen, true
	case "watched":
		return watch.LevelWatched, true
	}
	return 0, false
}

// watchTarget is what following a project watches: the directories its scan
// reads and, for a checkout, its git state.
func (e *Events) watchTarget(root string) watch.Target {
	return watch.Target{
		Root: root,
		Directories: func() ([]string, int, error) {
			return files.Directories(root, e.config.Depth)
		},
		Git: func() ([]string, func(string) bool) {
			state, ok := projects.ReadGitState(root)
			if !ok {
				return nil, nil
			}
			return state.Directories, state.Records
		},
	}
}

// Ready signals that Next has events.
func (s *Subscription) Ready() <-chan struct{} { return s.watch.Ready() }

// Done closes when the hub stops following the project.
func (s *Subscription) Done() <-chan struct{} { return s.watch.Done() }

// Close ends the subscription.
func (s *Subscription) Close() { s.watch.Close() }

// Next returns the pending events in order: a status (the first followed by
// the checkout's head), then a change (followed by the head when git state
// changed and the head moved).
func (s *Subscription) Next() []contract.Event {
	var events []contract.Event
	status, change := s.watch.Next()
	if status != nil {
		events = append(events, contract.Event{Name: contract.EventStatus, Status: &contract.StatusEvent{Mode: string(status.Mode), Reason: status.Reason}})
		if !s.baseline {
			s.baseline = true
			events = s.appendHead(events)
		}
	}
	if change != nil {
		events = append(events, contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: change.Paths, Tree: change.Tree, Git: change.Git, Overflow: change.Overflow}})
		if change.Git {
			events = s.appendHead(events)
		}
	}
	return events
}

// appendHead adds a head event when the checkout's head differs from the
// last one reported.
func (s *Subscription) appendHead(events []contract.Event) []contract.Event {
	head, err := git.ReadHead(s.root)
	if err != nil {
		return events
	}
	current := contract.HeadRef{Branch: head.Branch, Commit: head.Commit}
	if s.last != nil && *s.last == current {
		return events
	}
	event := contract.HeadEvent{Branch: head.Branch, Commit: head.Commit, Subject: head.Subject, Previous: s.last}
	s.last = &current
	return append(events, contract.Event{Name: contract.EventHead, Head: &event})
}
