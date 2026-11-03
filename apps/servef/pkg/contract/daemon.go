package contract

import "time"

// Daemon control (/_daemon/*, decisions 0061, 0066, 0076) error codes.
const (
	CodeBrowserRequest         = "browserRequest"
	CodeUnsupportedMediaType   = "unsupportedMediaType"
	CodeInvalidRequest         = "invalidRequest"
	CodeInvalidRoot            = "invalidRoot"
	CodeRootsNotSaved          = "rootsNotSaved"
	CodeProjectUnavailable     = "projectUnavailable"
	CodeWalkthroughUnavailable = "walkthroughUnavailable"
	CodePathUnavailable        = "pathUnavailable"
	CodeInvalidPath            = "invalidPath"
	CodeNotDirectory           = "notDirectory"
	CodePathUnreadable         = "pathUnreadable"
)

// Daemon control stream event names, GET /_daemon/events.
const (
	DaemonEventStatus      = "status"
	DaemonEventWalkthrough = "walkthrough"
)

// DaemonStatus describes a running daemon, GET /_daemon/status. Its shape
// is frozen across protocols.
type DaemonStatus struct {
	Protocol  int       `json:"protocol"`
	Version   string    `json:"version"`
	PID       int       `json:"pid"`
	Socket    string    `json:"socket"`
	Roots     []string  `json:"roots"`
	Depth     int       `json:"depth"`
	ScanDepth int       `json:"scanDepth"`
	IdleExit  string    `json:"idleExit"` // time.Duration.String(); "0s" never exits
	StartedAt time.Time `json:"startedAt"`
}

// DaemonRoots is GET /_daemon/roots: the served roots and the saved roots
// skipped at start.
type DaemonRoots struct {
	Roots   []string `json:"roots"`
	Skipped []string `json:"skipped"`
}

// DaemonPathRequest is the body of POST /_daemon/roots and /_daemon/open.
type DaemonPathRequest struct {
	Path string `json:"path"`
}

// DaemonOpenResult is POST /_daemon/open's answer: the root serving the
// path, whether it was added, the roots it replaced (inside it), and the
// project that shows the path.
type DaemonOpenResult struct {
	Root     string   `json:"root"`
	Added    bool     `json:"added"`
	Replaced []string `json:"replaced"`
	Project  string   `json:"project"`
}

// BrowseEntry is one child directory in a folder picker listing.
type BrowseEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Repository bool   `json:"repository"`
	Hidden     bool   `json:"hidden"`
}

// DaemonDirectories is GET /_daemon/directories: a directory's child
// directories for a folder picker (decision 0076).
type DaemonDirectories struct {
	Path      string        `json:"path"`
	Parent    string        `json:"parent"`
	Entries   []BrowseEntry `json:"entries"`
	Truncated bool          `json:"truncated"`
}

// DaemonStatusEvent is the control stream's first event.
type DaemonStatusEvent struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
}

// WalkthroughEvent is the control stream's "walkthrough": a published
// walkthrough announcement.
type WalkthroughEvent struct {
	Project string `json:"project"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Created bool   `json:"created"`
}

// DaemonEvent is one control stream event: Name says which field is set.
type DaemonEvent struct {
	Name        string
	Status      *DaemonStatusEvent
	Walkthrough *WalkthroughEvent
}

// Payload is the event's body, encoded as the stream's data.
func (e DaemonEvent) Payload() any {
	switch e.Name {
	case DaemonEventStatus:
		return e.Status
	case DaemonEventWalkthrough:
		return e.Walkthrough
	}
	return nil
}
