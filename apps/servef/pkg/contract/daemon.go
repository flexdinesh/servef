package contract

import "time"

// Daemon control (/_daemon/*, decisions 0061, 0066) error codes.
const (
	CodeBrowserRequest         = "browserRequest"
	CodeUnsupportedMediaType   = "unsupportedMediaType"
	CodeInvalidRequest         = "invalidRequest"
	CodeInvalidRoot            = "invalidRoot"
	CodeRootsNotSaved          = "rootsNotSaved"
	CodeProjectUnavailable     = "projectUnavailable"
	CodeWalkthroughUnavailable = "walkthroughUnavailable"
)

// Remote serving (decision 0098) error codes.
const (
	// CodeUnauthorized: a TCP request without the connection string's token.
	CodeUnauthorized = "unauthorized"
	// CodePortBusy: the port asked for, or every default port, is taken.
	CodePortBusy = "portBusy"
	// CodeRemoteUnavailable: this daemon does not serve over TCP.
	CodeRemoteUnavailable = "remoteUnavailable"
)

// RemoteInfoPath answers RemoteInfo over TCP, after authentication. The
// desktop app's pages cannot reach it: /_servef is refused there.
const RemoteInfoPath = "/_servef/remote"

// RemoteInfo describes a remote server to a client checking a connection
// string.
type RemoteInfo struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
	// Name labels the connection: the machine's host name.
	Name string `json:"name"`
}

// DaemonRemoteRequest is the body of POST /_daemon/remote. An empty Host binds
// every interface. A nil Port tries the last port, then the default range. The
// durations (time.ParseDuration text) default to 10m and 5m.
type DaemonRemoteRequest struct {
	Host      string `json:"host,omitempty"`
	Port      *int   `json:"port,omitempty"`
	FirstWait string `json:"firstWait,omitempty"`
	Grace     string `json:"grace,omitempty"`
}

// DaemonRemote is the daemon's TCP listener, /_daemon/remote. Credential is a
// secret: only the Unix socket answers this. Clients is the UIs connected.
type DaemonRemote struct {
	Listening  bool   `json:"listening"`
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	ID         string `json:"id,omitempty"`
	Credential string `json:"credential,omitempty"`
	Clients    int    `json:"clients"`
}

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
