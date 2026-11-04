// Package inprocess is the in-process transport for client.Client: it calls
// the application layer and an optional daemon controller directly. It is a
// separate package so HTTP-only users of pkg/client, such as the native app,
// do not link the server and its embedded web app.
package inprocess

import (
	"context"
	"errors"
	"io"

	"github.com/flexdinesh/servef/internal/app"
	"github.com/flexdinesh/servef/internal/daemon"
	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Client calls the application layer directly, with no encoding.
type Client struct {
	app     *app.App
	control *daemon.Controller
}

var _ client.Client = (*Client)(nil)

// New returns a client over application; control, when not nil,
// answers the daemon calls, otherwise they return client.ErrNoDaemon.
func New(application *app.App, control *daemon.Controller) *Client {
	return &Client{app: application, control: control}
}

func (c *Client) Projects(ctx context.Context) (contract.ProjectsResponse, error) {
	if err := ctx.Err(); err != nil {
		return contract.ProjectsResponse{}, err
	}
	return c.app.Projects.List(), nil
}

func (c *Client) Directories(ctx context.Context, path string) (contract.DirectoriesResponse, error) {
	return call(ctx, func() (contract.DirectoriesResponse, error) { return c.app.Projects.Directories(path) })
}

func (c *Client) Page(ctx context.Context, request contract.PageRequest) (contract.PageData, error) {
	if err := ctx.Err(); err != nil {
		return contract.PageData{}, err
	}
	page, err := c.app.Documents.Page(request)
	return page, clientError(err)
}

func (c *Client) Format(ctx context.Context, request contract.FormatRequest) (contract.FormatResult, error) {
	return call(ctx, func() (contract.FormatResult, error) { return c.app.Documents.Format(request) })
}

func (c *Client) SearchDocuments(ctx context.Context, project string) (contract.SearchDocumentsResponse, error) {
	return call(ctx, func() (contract.SearchDocumentsResponse, error) { return c.app.Documents.SearchDocuments(project) })
}

func (c *Client) Image(ctx context.Context, project, path string) (contract.Image, error) {
	return call(ctx, func() (contract.Image, error) { return c.app.Documents.ReadImage(project, path) })
}

func (c *Client) Events(ctx context.Context, request contract.EventsRequest) (*client.Stream[contract.Event], error) {
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
	return client.NewStream(next, func() { cancel(); subscription.Close() }), nil
}

func (c *Client) Changes(ctx context.Context, project string) (contract.ChangesResponse, error) {
	return call(ctx, func() (contract.ChangesResponse, error) { return c.app.SourceControl.Changes(ctx, project) })
}

func (c *Client) Diff(ctx context.Context, request contract.DiffRequest) (contract.DiffResponse, error) {
	return call(ctx, func() (contract.DiffResponse, error) { return c.app.SourceControl.Diff(ctx, request) })
}

func (c *Client) Commits(ctx context.Context, request contract.CommitsRequest) (contract.CommitsResponse, error) {
	return call(ctx, func() (contract.CommitsResponse, error) { return c.app.SourceControl.Commits(ctx, request) })
}

func (c *Client) CommitFiles(ctx context.Context, project, commit string) (contract.CommitFilesResponse, error) {
	return call(ctx, func() (contract.CommitFilesResponse, error) {
		return c.app.SourceControl.CommitFiles(ctx, project, commit)
	})
}

func (c *Client) BranchFiles(ctx context.Context, request contract.BranchFilesRequest) (contract.BranchFilesResponse, error) {
	return call(ctx, func() (contract.BranchFilesResponse, error) { return c.app.SourceControl.BranchFiles(ctx, request) })
}

func (c *Client) RangeFiles(ctx context.Context, project, from, to string) (contract.RangeFilesResponse, error) {
	return call(ctx, func() (contract.RangeFilesResponse, error) {
		return c.app.SourceControl.RangeFiles(ctx, project, from, to)
	})
}

func (c *Client) Refs(ctx context.Context, project string) (contract.RefsResponse, error) {
	return call(ctx, func() (contract.RefsResponse, error) { return c.app.SourceControl.Refs(project) })
}

func (c *Client) BranchStack(ctx context.Context, project, base string) (contract.StackResponse, error) {
	return call(ctx, func() (contract.StackResponse, error) { return c.app.SourceControl.BranchStack(ctx, project, base) })
}

func (c *Client) CompareStats(ctx context.Context, request contract.CompareStatsRequest) (contract.CompareStatsResponse, error) {
	return call(ctx, func() (contract.CompareStatsResponse, error) { return c.app.SourceControl.CompareStats(ctx, request) })
}

func (c *Client) NavigationStatus(ctx context.Context, request contract.NavigationStatusRequest) (contract.NavigationStatus, error) {
	return call(ctx, func() (contract.NavigationStatus, error) { return c.app.Navigation.Status(request) })
}

func (c *Client) Definition(ctx context.Context, request contract.NavigationRequest) (contract.NavigationResult, error) {
	return call(ctx, func() (contract.NavigationResult, error) { return c.app.Navigation.Definition(ctx, request) })
}

func (c *Client) References(ctx context.Context, request contract.NavigationRequest) (contract.NavigationResult, error) {
	return call(ctx, func() (contract.NavigationResult, error) { return c.app.Navigation.References(ctx, request) })
}

func (c *Client) Metrics(ctx context.Context) (contract.ProcessMetrics, error) {
	return call(ctx, func() (contract.ProcessMetrics, error) { return c.app.Metrics.Sample(), nil })
}

func (c *Client) DaemonStatus(ctx context.Context) (contract.DaemonStatus, error) {
	return daemonCall(ctx, c.control, func() (contract.DaemonStatus, error) { return c.control.Status(), nil })
}

func (c *Client) DaemonRoots(ctx context.Context) (contract.DaemonRoots, error) {
	return daemonCall(ctx, c.control, func() (contract.DaemonRoots, error) { return c.control.Roots(), nil })
}

func (c *Client) DaemonOpen(ctx context.Context, path string) (contract.DaemonOpenResult, error) {
	return daemonCall(ctx, c.control, func() (contract.DaemonOpenResult, error) { return c.control.Open(path) })
}

func (c *Client) DaemonDirectories(ctx context.Context, path string) (contract.DaemonDirectories, error) {
	return daemonCall(ctx, c.control, func() (contract.DaemonDirectories, error) { return daemon.Directories(path) })
}

func (c *Client) DaemonEvents(ctx context.Context) (*client.Stream[contract.DaemonEvent], error) {
	if c.control == nil {
		return nil, client.ErrNoDaemon
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
	return client.NewStream(next, func() { cancel(); cancelNotices() }), nil
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
		return zero, client.ErrNoDaemon
	}
	return call(ctx, operation)
}

// clientError turns an application *Error into a *client.Error.
func clientError(err error) error {
	if err == nil {
		return nil
	}
	var failure *app.Error
	if errors.As(err, &failure) {
		return &client.Error{Kind: failure.Kind, Code: failure.Code, Message: failure.Message}
	}
	return err
}
