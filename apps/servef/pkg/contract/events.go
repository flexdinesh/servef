package contract

// Event names of a project's live-update stream, GET /api/events (decision
// 0050). HTTP sends each as a server-sent event of that name.
const (
	EventStatus = "status"
	EventChange = "change"
	EventHead   = "head"
	// EventWalkthrough announces a walkthrough published for the stream's
	// project (decision 0099). Its body is the control stream's WalkthroughEvent.
	EventWalkthrough = "walkthrough"
)

// CodeInvalidLease rejects an events request whose lease is neither open nor
// watched.
const CodeInvalidLease = "invalidLease"

// StatusEvent is the stream's "status": how the project is followed.
type StatusEvent struct {
	// Mode is watching, polling, or off.
	Mode   string `json:"mode"`
	Reason string `json:"reason"`
}

// ChangeEvent is the stream's "change": one debounced batch of
// project-relative paths.
type ChangeEvent struct {
	Paths    []string `json:"paths"`
	Tree     bool     `json:"tree"`
	Git      bool     `json:"git"`
	Overflow bool     `json:"overflow"`
}

// HeadEvent is the stream's "head": what a checkout has checked out. Branch
// is empty when detached and Commit when unborn. Parent is the commit's first
// parent, empty without one, so a client can tell a new commit from any other
// move of HEAD (decision 0099). Previous is the stream's last head, null for
// the first.
type HeadEvent struct {
	Branch   string   `json:"branch"`
	Commit   string   `json:"commit"`
	Parent   string   `json:"parent"`
	Subject  string   `json:"subject"`
	Previous *HeadRef `json:"previous"`
}

// HeadRef is a head without its subject.
type HeadRef struct {
	Branch string `json:"branch"`
	Commit string `json:"commit"`
}

// Event is one event of a project stream: Name says which field is set.
type Event struct {
	Name        string
	Status      *StatusEvent
	Change      *ChangeEvent
	Head        *HeadEvent
	Walkthrough *WalkthroughEvent
}

// Payload is the event's body, encoded as the stream's data.
func (e Event) Payload() any {
	switch e.Name {
	case EventStatus:
		return e.Status
	case EventChange:
		return e.Change
	case EventHead:
		return e.Head
	case EventWalkthrough:
		return e.Walkthrough
	}
	return nil
}
