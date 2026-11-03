package client

import (
	"context"
	"errors"
	"io"

	"github.com/flexdinesh/servef/internal/app"
	"github.com/flexdinesh/servef/internal/daemon"
	"github.com/flexdinesh/servef/pkg/contract"
)

// InProcess calls the application layer directly, with no encoding.
type InProcess struct {
	app     *app.App
	control *daemon.Controller
}

var _ Client = (*InProcess)(nil)

// NewInProcess returns a client over application; control, when not nil,
// answers the daemon calls, otherwise they return ErrNoDaemon.
func NewInProcess(application *app.App, control *daemon.Controller) *InProcess {
	return &InProcess{app: application, control: control}
}

func (c *InProcess) Projects(ctx context.Context) (contract.ProjectsResponse, error) {
	if err := ctx.Err(); err != nil {
		return contract.ProjectsResponse{}, err
	}
	return c.app.Projects.List(), nil
}

func (c *InProcess) Directories(ctx context.Context, path string) (contract.DirectoriesResponse, error) {
	return call(ctx, func() (contract.DirectoriesResponse, error) { return c.app.Projects.Directories(path) })
}

func (c *InProcess) Page(ctx context.Context, request contract.PageRequest) (contract.PageData, error) {
	if err := ctx.Err(); err != nil {
		return contract.PageData{}, err
	}
	page, err := c.app.Documents.Page(request)
	return page, clientError(err)
}

func (c *InProcess) Format(ctx context.Context, request contract.FormatRequest) (contract.FormatResult, error) {
	return call(ctx, func() (contract.FormatResult, error) { return c.app.Documents.Format(request) })
}

func (c *InProcess) SearchDocuments(ctx context.Context, project string) (contract.SearchDocumentsResponse, error) {
	return call(ctx, func() (contract.SearchDocumentsResponse, error) { return c.app.Documents.SearchDocuments(project) })
}

func (c *InProcess) Events(ctx context.Context, request contract.EventsRequest) (*Stream[contract.Event], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	subscription, err := c.app.Events.Subscribe(request)
	if err != nil {
		return nil, clientError(err)
	}
	ctx, cancel := context.WithCancel(ctx)
	var pending []contract.Event
	next := func() (contract.Event, error) {
		for len(pending) == 0 {
			select {
			case <-ctx.Done():
				return contract.Event{}, ctx.Err()
			case <-subscription.Done():
				return contract.Event{}, io.EOF
			case <-subscription.Ready():
				pending = subscription.Next()
			}
		}
		event := pending[0]
		pending = pending[1:]
		return event, nil
	}
	return &Stream[contract.Event]{next: next, close: func() { cancel(); subscription.Close() }}, nil
}

func (c *InProcess) Changes(ctx context.Context, project string) (contract.ChangesResponse, error) {
	return call(ctx, func() (contract.ChangesResponse, error) { return c.app.SourceControl.Changes(ctx, project) })
}

func (c *InProcess) Diff(ctx context.Context, request contract.DiffRequest) (contract.DiffResponse, error) {
	return call(ctx, func() (contract.DiffResponse, error) { return c.app.SourceControl.Diff(ctx, request) })
}

func (c *InProcess) Commits(ctx context.Context, request contract.CommitsRequest) (contract.CommitsResponse, error) {
	return call(ctx, func() (contract.CommitsResponse, error) { return c.app.SourceControl.Commits(ctx, request) })
}

func (c *InProcess) CommitFiles(ctx context.Context, project, commit string) (contract.CommitFilesResponse, error) {
	return call(ctx, func() (contract.CommitFilesResponse, error) {
		return c.app.SourceControl.CommitFiles(ctx, project, commit)
	})
}

func (c *InProcess) BranchFiles(ctx context.Context, request contract.BranchFilesRequest) (contract.BranchFilesResponse, error) {
	return call(ctx, func() (contract.BranchFilesResponse, error) { return c.app.SourceControl.BranchFiles(ctx, request) })
}

func (c *InProcess) RangeFiles(ctx context.Context, project, from, to string) (contract.RangeFilesResponse, error) {
	return call(ctx, func() (contract.RangeFilesResponse, error) {
		return c.app.SourceControl.RangeFiles(ctx, project, from, to)
	})
}

func (c *InProcess) Refs(ctx context.Context, project string) (contract.RefsResponse, error) {
	return call(ctx, func() (contract.RefsResponse, error) { return c.app.SourceControl.Refs(project) })
}

func (c *InProcess) BranchStack(ctx context.Context, project, base string) (contract.StackResponse, error) {
	return call(ctx, func() (contract.StackResponse, error) { return c.app.SourceControl.BranchStack(ctx, project, base) })
}

func (c *InProcess) CompareStats(ctx context.Context, request contract.CompareStatsRequest) (contract.CompareStatsResponse, error) {
	return call(ctx, func() (contract.CompareStatsResponse, error) { return c.app.SourceControl.CompareStats(ctx, request) })
}

func (c *InProcess) DaemonStatus(ctx context.Context) (contract.DaemonStatus, error) {
	return daemonCall(ctx, c.control, func() (contract.DaemonStatus, error) { return c.control.Status(), nil })
}

func (c *InProcess) DaemonRoots(ctx context.Context) (contract.DaemonRoots, error) {
	return daemonCall(ctx, c.control, func() (contract.DaemonRoots, error) { return c.control.Roots(), nil })
}

func (c *InProcess) DaemonOpen(ctx context.Context, path string) (contract.DaemonOpenResult, error) {
	return daemonCall(ctx, c.control, func() (contract.DaemonOpenResult, error) { return c.control.Open(path) })
}

func (c *InProcess) DaemonDirectories(ctx context.Context, path string) (contract.DaemonDirectories, error) {
	return daemonCall(ctx, c.control, func() (contract.DaemonDirectories, error) { return daemon.Directories(path) })
}

func (c *InProcess) DaemonEvents(ctx context.Context) (*Stream[contract.DaemonEvent], error) {
	if c.control == nil {
		return nil, ErrNoDaemon
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	first, notices, cancelNotices := c.control.Subscribe()
	ctx, cancel := context.WithCancel(ctx)
	pending := []contract.DaemonEvent{first}
	next := func() (contract.DaemonEvent, error) {
		if len(pending) > 0 {
			event := pending[0]
			pending = pending[1:]
			return event, nil
		}
		select {
		case <-ctx.Done():
			return contract.DaemonEvent{}, ctx.Err()
		case <-c.control.Done():
			return contract.DaemonEvent{}, io.EOF
		case notice, ok := <-notices:
			if !ok {
				return contract.DaemonEvent{}, io.EOF
			}
			return daemon.WalkthroughEvent(notice), nil
		}
	}
	return &Stream[contract.DaemonEvent]{next: next, close: func() { cancel(); cancelNotices() }}, nil
}

// call runs an operation unless ctx has ended, translating its error.
func call[T any](ctx context.Context, operation func() (T, error)) (T, error) {
	if err := ctx.Err(); err != nil {
		var zero T
		return zero, err
	}
	result, err := operation()
	return result, clientError(err)
}

// daemonCall is call for daemon operations, which need a controller.
func daemonCall[T any](ctx context.Context, control *daemon.Controller, operation func() (T, error)) (T, error) {
	if control == nil {
		var zero T
		return zero, ErrNoDaemon
	}
	return call(ctx, operation)
}

// clientError turns an application *Error into the client's *Error.
func clientError(err error) error {
	if err == nil {
		return nil
	}
	var failure *app.Error
	if errors.As(err, &failure) {
		return &Error{Kind: failure.Kind, Code: failure.Code, Message: failure.Message}
	}
	return err
}
