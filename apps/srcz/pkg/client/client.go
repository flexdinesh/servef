// Package client is srcz's Go client (decision 0080): one interface over
// three transports with identical results. HTTP over TCP reaches a CLI
// server; HTTP over a Unix socket reaches the daemon; in-process calls the
// application layer (internal/app) and, when given one, a daemon controller
// directly. Every result is a pkg/contract type; every failure an *Error or
// ErrNoDaemon.
package client

import (
	"context"
	"errors"
	"sync"

	"github.com/flexdinesh/srcz/pkg/contract"
)

// Client is the data API, the project event stream, and the daemon control
// calls apps need.
type Client interface {
	// Projects lists the served roots and the projects within them.
	Projects(ctx context.Context) (contract.ProjectsResponse, error)
	// Directories lists a served folder's child directories.
	Directories(ctx context.Context, path string) (contract.DirectoriesResponse, error)
	// Page reads a project's tree and, optionally, a document. A failed page
	// returns both the page, which carries its project and error, and an
	// *Error.
	Page(ctx context.Context, request contract.PageRequest) (contract.PageData, error)
	// Format returns a formattable document's formatted text.
	Format(ctx context.Context, request contract.FormatRequest) (contract.FormatResult, error)
	// SearchDocuments returns every searchable file's original text.
	SearchDocuments(ctx context.Context, project string) (contract.SearchDocumentsResponse, error)
	// Image reads a project image (a Markdown image dependency, root-confined,
	// image types only) up to contract.MaxImageBytes.
	Image(ctx context.Context, project, path string) (contract.Image, error)
	// Events follows a project's live updates until ctx ends or the stream
	// is closed.
	Events(ctx context.Context, request contract.EventsRequest) (*Stream[contract.Event], error)

	// Changes lists the working tree's changes since HEAD.
	Changes(ctx context.Context, project string) (contract.ChangesResponse, error)
	// Diff reads one changed file's sides and their line diff.
	Diff(ctx context.Context, request contract.DiffRequest) (contract.DiffResponse, error)
	// Commits pages history.
	Commits(ctx context.Context, request contract.CommitsRequest) (contract.CommitsResponse, error)
	// CommitFiles lists a commit's changes against its first parent.
	CommitFiles(ctx context.Context, project, commit string) (contract.CommitFilesResponse, error)
	// BranchFiles lists a branch's net changes.
	BranchFiles(ctx context.Context, request contract.BranchFilesRequest) (contract.BranchFilesResponse, error)
	// RangeFiles lists a commit range's net changes, from (the oldest
	// commit) to to (the newest).
	RangeFiles(ctx context.Context, project, from, to string) (contract.RangeFilesResponse, error)
	// Refs lists the branches a compare may use as its base.
	Refs(ctx context.Context, project string) (contract.RefsResponse, error)
	// BranchStack lists the local branches stacked with HEAD on base, or
	// else the default base branch.
	BranchStack(ctx context.Context, project, base string) (contract.StackResponse, error)
	// CompareStats counts the changed lines of a compare's files.
	CompareStats(ctx context.Context, request contract.CompareStatsRequest) (contract.CompareStatsResponse, error)

	// NavigationStatus reports whether a file can navigate and its language
	// server's state; it never starts a server.
	NavigationStatus(ctx context.Context, request contract.NavigationStatusRequest) (contract.NavigationStatus, error)
	// Definition asks the file's language server for definitions.
	Definition(ctx context.Context, request contract.NavigationRequest) (contract.NavigationResult, error)
	// References asks the file's language server for references.
	References(ctx context.Context, request contract.NavigationRequest) (contract.NavigationResult, error)

	// Metrics samples the server process.
	Metrics(ctx context.Context) (contract.ProcessMetrics, error)

	// Walkthroughs lists a project's walkthroughs, most recently updated
	// first.
	Walkthroughs(ctx context.Context, project string) ([]contract.WalkthroughSummary, error)
	// Walkthrough reads a project's walkthrough with its Markdown rendered.
	Walkthrough(ctx context.Context, project, id string) (contract.Walkthrough, error)
	// PublishWalkthrough checks a draft's targets, stores it, and announces
	// it; Open is honored over HTTP only.
	PublishWalkthrough(ctx context.Context, draft contract.WalkthroughDraft) (contract.WalkthroughPublished, error)
	// DeleteWalkthrough removes a project's walkthrough.
	DeleteWalkthrough(ctx context.Context, project, id string) error
	// WalkthroughSchema returns the JSON Schema of a draft.
	WalkthroughSchema(ctx context.Context) ([]byte, error)

	// DaemonStatus returns the daemon's status.
	DaemonStatus(ctx context.Context) (contract.DaemonStatus, error)
	// DaemonRoots returns the daemon's served and skipped roots.
	DaemonRoots(ctx context.Context) (contract.DaemonRoots, error)
	// DaemonOpen serves an absolute directory, covering it with a root.
	DaemonOpen(ctx context.Context, path string) (contract.DaemonOpenResult, error)
	// DaemonEvents follows the daemon's control stream: status, then
	// walkthrough announcements.
	DaemonEvents(ctx context.Context) (*Stream[contract.DaemonEvent], error)
}

// ErrNoDaemon reports a daemon call on a transport without a daemon: a CLI
// server over TCP, or an in-process client without a controller.
var ErrNoDaemon = errors.New("srcz: no daemon on this transport")

// Error is a failed operation: its kind (over HTTP, from the status), the
// optional code, and the server's message.
type Error struct {
	Kind    contract.ErrorKind
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Stream yields a stream's events in order. Next returns io.EOF once the
// server ends the stream and the context's error once it ends.
type Stream[T any] struct {
	next  func() (T, error)
	close func()
	once  sync.Once
}

// NewStream returns a stream over next, for transports outside this package;
// close runs once, on the first Close.
func NewStream[T any](next func() (T, error), close func()) *Stream[T] {
	return &Stream[T]{next: next, close: close}
}

// Next blocks until the next event.
func (s *Stream[T]) Next() (T, error) { return s.next() }

// Close ends the stream; Next then fails.
func (s *Stream[T]) Close() { s.once.Do(s.close) }
