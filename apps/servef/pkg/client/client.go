// Package client is servef's Go client (decision 0080): one interface over
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

	"github.com/flexdinesh/servef/pkg/contract"
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

	// DaemonStatus returns the daemon's status.
	DaemonStatus(ctx context.Context) (contract.DaemonStatus, error)
	// DaemonRoots returns the daemon's served and skipped roots.
	DaemonRoots(ctx context.Context) (contract.DaemonRoots, error)
	// DaemonOpen serves an absolute directory, covering it with a root.
	DaemonOpen(ctx context.Context, path string) (contract.DaemonOpenResult, error)
	// DaemonDirectories lists any absolute directory for a folder picker;
	// "" lists the home directory.
	DaemonDirectories(ctx context.Context, path string) (contract.DaemonDirectories, error)
	// DaemonEvents follows the daemon's control stream: status, then
	// walkthrough announcements.
	DaemonEvents(ctx context.Context) (*Stream[contract.DaemonEvent], error)
}

// ErrNoDaemon reports a daemon call on a transport without a daemon: a CLI
// server over TCP, or an in-process client without a controller.
var ErrNoDaemon = errors.New("servef: no daemon on this transport")

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

// Next blocks until the next event.
func (s *Stream[T]) Next() (T, error) { return s.next() }

// Close ends the stream; Next then fails.
func (s *Stream[T]) Close() { s.once.Do(s.close) }
