// Package workspace is native's view model: the connection, its projects,
// the open project's tree and document, and live updates. It calls servef
// only through pkg/client and knows no UI toolkit, so
// it runs headless in tests. All state changes happen on the UI thread:
// the view calls methods there, and background work hands its result back
// through a Runner.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/flexdinesh/servef/apps/native/internal/connect"
	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/highlight"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/apps/native/internal/live"
	"github.com/flexdinesh/servef/apps/native/internal/mdlink"
	"github.com/flexdinesh/servef/apps/native/internal/review"
	"github.com/flexdinesh/servef/apps/native/internal/tree"
	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Runner runs work off the UI thread. Go runs work and then the apply
// function it returns, if any, on the UI thread; Post runs apply on the UI
// thread.
type Runner interface {
	Go(work func() (apply func()))
	Post(apply func())
}

// Connector makes connections.
type Connector interface {
	Local(ctx context.Context) (connect.Connection, error)
	Server(target link.Target) connect.Connection
	Servers(ctx context.Context) ([]connect.ListedServer, error)
}

// Panel is the sidebar's view.
type Panel uint8

const (
	// Explorer is the project's tree.
	Explorer Panel = iota
	// SourceControl is the working tree's changes since HEAD.
	SourceControl
)

// Review is the open working-tree diff, shown in place of the document.
type Review struct {
	File contract.ChangedFile
	// Diff is the server's answer, nil before the first one arrives; a
	// reread keeps the last one shown until the next arrives.
	Diff *contract.DiffResponse
	// Layout is Diff laid out for the viewer; Version changes with it.
	Layout  review.Layout
	Version int
	// Error is the last failure to read the diff, "" for none.
	Error string
	// Old and New are the sides' highlighted lines (full files) once
	// ready for Version, nil until then.
	Old, New     [][]highlight.Span
	LinesVersion int
	request      int
}

// requestTimeout bounds one data request.
const requestTimeout = 30 * time.Second

// reconnectDelay waits before reopening a project's event stream that
// ended unexpectedly.
const reconnectDelay = 2 * time.Second

// Model is the workspace state the view shows.
type Model struct {
	run       Runner
	connector Connector

	// Connection is the current connection; nil before one is made.
	Connection *connect.Connection
	// Connecting names the connection being made, "" when idle.
	Connecting string
	// Error is the last failure to show, "" for none.
	Error string
	// Confirm is a link to a server on another computer that waits for
	// the user to accept it.
	Confirm *link.Target
	trusted map[string]bool
	// Servers are the CLI servers on this computer, for the connect page.
	Servers []connect.ListedServer

	Projects contract.ProjectsResponse
	// Project is the open project's ID, "" for none.
	Project string
	Page    contract.PageData
	Tree    tree.Index
	// TreeRead changes each time a page brings the tree, so the view
	// rebuilds its rows from it.
	TreeRead int
	// TreeVersion changes when a project's first page arrives, so the view
	// resets which directories are open to TreeOpen, those the server
	// marked open. RevealVersion changes when an opened document's row
	// should show: the view opens the directories holding it.
	TreeVersion   int
	TreeOpen      []string
	RevealVersion int
	View          docview.View
	// Lines are the shown code's highlighted lines once ready, for
	// CodeVersion. CodeVersion changes when the code shown does: another
	// document, a reread, or Source and Format switching.
	Lines           [][]highlight.Span
	LinesVersion    int
	CodeVersion     int
	DocumentVersion int
	documents       documents
	// Anchor is a heading to scroll a Markdown preview to.
	Anchor        string
	AnchorVersion uint64
	// RevealLine is a line a link asked to see, once LineRevealVersion
	// changes; ScrollResetVersion changes when the document's scroll starts
	// over.
	RevealLine         int
	RevealKey          string
	LineRevealVersion  uint64
	ScrollResetVersion uint64

	// Panel is the sidebar's view.
	Panel Panel
	// Changes is the working tree's change list once read (ChangesRead);
	// ChangesError the last failure to read it.
	Changes      contract.ChangesResponse
	ChangesRead  bool
	ChangesError string
	// ChangesVersion changes with each read of Changes.
	ChangesVersion int
	// Review is the open diff, nil for none. FullFile shows whole files
	// rather than hunks; IgnoreWhitespace compares lines without leading
	// and trailing whitespace.
	Review           *Review
	FullFile         bool
	IgnoreWhitespace bool

	// Live is the event stream's status, and Head the checkout's head.
	Live live.Indicator
	Head *contract.HeadEvent

	reconnect      time.Duration
	pageRequest    int
	changesRequest int
	// changesLoading is a changes read in flight; changesAt when the last
	// one started, for refreshes on return.
	changesLoading bool
	changesAt      time.Time
	now            func() time.Time
	stream         context.CancelFunc
	streamID       int
	closed         bool
}

// New returns an empty workspace.
func New(run Runner, connector Connector) *Model {
	m := &Model{run: run, connector: connector, trusted: map[string]bool{}, reconnect: reconnectDelay, now: time.Now}
	m.documents.memory = docview.NewMemory()
	return m
}

// Close ends background streams.
func (m *Model) Close() {
	m.closed = true
	m.stopStream()
}

// Handle opens what a launch or a second instance asked for: a link, a
// path (through the daemon), or, when empty, the local daemon unless a
// connection exists.
func (m *Model) Handle(request link.Request) {
	switch {
	case request.Link != nil:
		m.OpenLink(*request.Link)
	case request.Path != "":
		m.ConnectLocal(func() { m.OpenPath(request.Path) })
	case m.Connection == nil && m.Connecting == "":
		m.ConnectLocal(nil)
	}
}

// OpenLink opens a parsed servef-native:// link. Servers on other
// computers wait in Confirm until AcceptLink, once per host and port per
// run.
func (m *Model) OpenLink(target link.Target) {
	then := func() { m.openTarget(target) }
	if target.Kind == link.Local {
		m.ConnectLocal(then)
		return
	}
	if !link.Loopback(target.Host) && !m.trusted[target.Origin()] {
		m.Confirm = &target
		return
	}
	m.connectServer(target, then)
}

// AcceptLink opens the server Confirm names.
func (m *Model) AcceptLink() {
	if m.Confirm == nil {
		return
	}
	target := *m.Confirm
	m.Confirm = nil
	m.trusted[target.Origin()] = true
	m.connectServer(target, func() { m.openTarget(target) })
}

// RejectLink drops the link Confirm names.
func (m *Model) RejectLink() { m.Confirm = nil }

// ConnectAddress connects to an address the user typed.
func (m *Model) ConnectAddress(address string) {
	target, err := link.ParseServer(address)
	if err != nil {
		m.Error = err.Error()
		return
	}
	m.connectServer(target, func() { m.openTarget(target) })
}

// ConnectLocal ensures the daemon, connects, and then runs then, if any.
func (m *Model) ConnectLocal(then func()) {
	if m.Connection != nil && m.Connection.Daemon {
		m.afterProjects(then)
		return
	}
	m.Connecting, m.Error = "This computer", ""
	m.run.Go(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), connect.EnsureTimeout+requestTimeout)
		defer cancel()
		connection, err := m.connector.Local(ctx)
		return func() {
			m.Connecting = ""
			if err != nil {
				m.Error = "Could not start the servef daemon: " + err.Error()
				return
			}
			m.use(connection, then)
		}
	})
}

func (m *Model) connectServer(target link.Target, then func()) {
	connection := m.connector.Server(target)
	if m.Connection != nil && m.Connection.ID == connection.ID {
		m.afterProjects(then)
		return
	}
	m.use(connection, then)
}

// use switches to connection, loads its projects, then runs then.
func (m *Model) use(connection connect.Connection, then func()) {
	m.stopStream()
	m.Connection = &connection
	m.Error = ""
	m.Projects = contract.ProjectsResponse{}
	m.closeProject()
	m.loadProjects(then)
}

// Disconnect returns to the connect page.
func (m *Model) Disconnect() {
	m.stopStream()
	m.Connection = nil
	m.Projects = contract.ProjectsResponse{}
	m.closeProject()
	m.RefreshServers()
}

// RefreshServers lists the CLI servers on this computer.
func (m *Model) RefreshServers() {
	m.run.Go(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		servers, err := m.connector.Servers(ctx)
		return func() {
			if err == nil {
				m.Servers = servers
			}
		}
	})
}

// RefreshProjects rereads the connection's projects.
func (m *Model) RefreshProjects() { m.loadProjects(nil) }

func (m *Model) afterProjects(then func()) {
	if then != nil {
		then()
	}
}

func (m *Model) loadProjects(then func()) {
	connection := m.Connection
	if connection == nil {
		return
	}
	m.run.Go(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		projects, err := connection.Client.Projects(ctx)
		return func() {
			if m.Connection != connection {
				return
			}
			if err != nil {
				m.Error = "Could not list projects: " + message(err)
				return
			}
			m.Projects = projects
			m.afterProjects(then)
		}
	})
}

// OpenPath serves an absolute directory through the daemon, as
// `servef [path]` does, and opens the project showing it.
func (m *Model) OpenPath(path string) {
	connection := m.Connection
	if connection == nil || !connection.Daemon {
		m.Error = "Folders open through the servef daemon on this computer."
		return
	}
	m.run.Go(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		result, err := connection.Client.DaemonOpen(ctx, path)
		return func() {
			if m.Connection != connection {
				return
			}
			if err != nil {
				m.Error = fmt.Sprintf("Could not open %s: %s", path, message(err))
				return
			}
			m.loadProjects(func() { m.OpenProject(result.Project) })
		}
	})
}

// openTarget opens a link's project and path; a link without a project
// keeps the project list.
func (m *Model) openTarget(target link.Target) {
	project, path := target.Project, target.Path
	if project == "" {
		return
	}
	lines := docview.ParseLineAnchor(target.Fragment)
	if project != m.Project {
		m.openProject(project, path)
		if path != "" {
			m.visit(path, lines)
		}
		return
	}
	if path != "" {
		m.openLocation(path, lines)
	}
}

// OpenProject opens a project's tree.
func (m *Model) OpenProject(id string) { m.openProject(id, "") }

func (m *Model) openProject(id, path string) {
	m.closeProject()
	m.Project = id
	m.loadPage(path, path != "", true, path != "")
	// The web reads changes on project load: the Explorer shows their
	// statuses (decision 0024).
	m.LoadChanges()
	m.startStream()
}

// CloseProject returns to the project list.
func (m *Model) CloseProject() {
	m.stopStream()
	m.closeProject()
}

func (m *Model) closeProject() {
	m.Project = ""
	m.Page = contract.PageData{}
	m.Tree = tree.Index{}
	m.TreeRead++
	m.TreeOpen = nil
	m.View = docview.View{}
	m.Lines = nil
	m.Head = nil
	m.Live = live.Indicator{}
	m.pageRequest++
	m.Panel = Explorer
	m.Changes, m.ChangesRead, m.ChangesError = contract.ChangesResponse{}, false, ""
	m.ChangesVersion++
	m.Review = nil
	m.changesRequest++
	m.changesLoading = false
	m.documents.close()
}

// OpenDocument opens a project-relative path in the open project, in
// place of an open diff.
func (m *Model) OpenDocument(path string) { m.openLocation(path, nil) }

// ShowPanel switches the sidebar; opening Source Control rereads the
// changes, as the web's view does, unless a read is in flight.
func (m *Model) ShowPanel(panel Panel) {
	opened := panel == SourceControl && m.Panel != SourceControl
	m.Panel = panel
	if opened && !m.changesLoading {
		m.LoadChanges()
	}
}

// returnThrottle is how soon after a read a return rereads nothing, as
// the web's focusThrottle.
const returnThrottle = 2 * time.Second

// Returned rereads the changes when the window regains the focus, as the
// web's useRefreshOnReturn does: at most once per returnThrottle since
// the last read started, never while one is in flight.
func (m *Model) Returned() {
	if m.changesLoading || m.now().Sub(m.changesAt) < returnThrottle {
		return
	}
	m.LoadChanges()
}

// LoadChanges rereads the working tree's changes.
func (m *Model) LoadChanges() {
	connection, project := m.Connection, m.Project
	if connection == nil || project == "" {
		return
	}
	m.changesRequest++
	request := m.changesRequest
	m.changesLoading, m.changesAt = true, m.now()
	m.run.Go(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		changes, err := connection.Client.Changes(ctx, project)
		return func() {
			if request != m.changesRequest || m.Connection != connection || m.Project != project {
				return
			}
			m.changesLoading = false
			if err != nil {
				m.ChangesError = "Could not read changes: " + message(err)
				return
			}
			m.Changes, m.ChangesRead, m.ChangesError = changes, true, ""
			m.ChangesVersion++
		}
	})
}

// OpenChange shows a changed file's diff in place of the document.
func (m *Model) OpenChange(file contract.ChangedFile) {
	if m.Project == "" {
		return
	}
	m.Review = &Review{File: file}
	m.loadDiff()
}

// CloseReview closes the open diff, showing the document again.
func (m *Model) CloseReview() { m.Review = nil }

// SetFullFile shows whole files, or only hunks, in the open diff: the
// viewer expands every gap.
func (m *Model) SetFullFile(full bool) { m.FullFile = full }

// SetIgnoreWhitespace rereads the open diff comparing lines with or
// without their leading and trailing whitespace.
func (m *Model) SetIgnoreWhitespace(ignore bool) {
	m.IgnoreWhitespace = ignore
	if m.Review != nil {
		m.loadDiff()
	}
}

// loadDiff reads the open diff; a reread keeps the last one shown.
func (m *Model) loadDiff() {
	connection, project, open := m.Connection, m.Project, m.Review
	if connection == nil || project == "" || open == nil {
		return
	}
	open.request++
	request := open.request
	query := contract.DiffRequest{Project: project, Path: open.File.Path, OldPath: open.File.OldPath, IgnoreWhitespace: m.IgnoreWhitespace}
	m.run.Go(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		diff, err := connection.Client.Diff(ctx, query)
		return func() {
			if m.Review != open || request != open.request || m.Connection != connection || m.Project != project {
				return
			}
			if err != nil {
				open.Diff, open.Error = nil, diffError(open.File.Path, err)
				open.Version++
				return
			}
			open.Diff, open.Error = &diff, ""
			m.layReview()
		}
	})
}

// layReview lays out the open diff and highlights its sides' full files
// in the server's language.
func (m *Model) layReview() {
	open := m.Review
	open.Layout = review.Lay(*open.Diff)
	open.Version++
	open.Old, open.New = nil, nil
	if open.Layout.Kind != review.Text {
		return
	}
	version, language := open.Version, open.Layout.Language
	original, modified := sideText(open.Diff.Original), sideText(open.Diff.Modified)
	m.run.Go(func() func() {
		old, new := highlight.Lines(original, language), highlight.Lines(modified, language)
		return func() {
			if m.Review == open && open.Version == version {
				open.Old, open.New, open.LinesVersion = old, new, version
			}
		}
	})
}

func sideText(side *contract.DiffSide) string {
	if side == nil || side.Content == nil {
		return ""
	}
	return *side.Content
}

// diffError is a failed diff read's text; a path with no changes left
// says so, as the web UI does.
func diffError(path string, err error) string {
	var clientErr *client.Error
	if errors.As(err, &clientErr) && clientErr.Code == contract.CodeUnchanged {
		return "No changes to " + path + " since the last commit."
	}
	return "Could not read the diff: " + message(err)
}

// FollowLink handles a link clicked in a Markdown preview: it opens
// project documents and headings, and returns an external URL for the
// system to open ("" for none).
func (m *Model) FollowLink(destination string) string {
	target := mdlink.Resolve(m.Page.Selected, destination)
	switch target.Kind {
	case mdlink.External:
		return target.URL
	case mdlink.Anchor:
		if lines := docview.ParseLineAnchor(target.Fragment); lines != nil {
			m.openLocation(m.Page.Selected, lines)
			break
		}
		m.Anchor = target.Fragment
		m.AnchorVersion++
	case mdlink.Document:
		if lines := docview.ParseLineAnchor(target.Fragment); lines != nil {
			m.openLocation(target.Path, lines)
			break
		}
		m.Anchor = target.Fragment
		m.AnchorVersion++
		m.OpenDocument(target.Path)
	}
	return ""
}

// ImageReader returns a reader of the open document's images, made on the
// UI thread and safe on any goroutine: a destination of its Markdown
// resolved as the server resolves it, or "" for the open image file
// itself. Only project images load, through the root-confined endpoint.
func (m *Model) ImageReader() func(ctx context.Context, destination string) ([]byte, error) {
	var images client.Client
	if m.Connection != nil {
		images = m.Connection.Client
	}
	project, document := m.Project, m.Page.Selected
	return func(ctx context.Context, destination string) ([]byte, error) {
		if images == nil || project == "" || document == "" {
			return nil, errors.New("no open document")
		}
		path := document
		if destination != "" {
			resolved, ok := mdlink.Image(document, destination)
			if !ok {
				return nil, errors.New("native loads only project images")
			}
			path = resolved
		}
		image, err := images.Image(ctx, project, path)
		return image.Content, err
	}
}

// loadPage reads the project's page and, with required, the document at
// path. newTree resets the tree's open directories; reveal opens the
// document's.
func (m *Model) loadPage(path string, required, newTree, reveal bool) {
	connection, project := m.Connection, m.Project
	if connection == nil || project == "" {
		return
	}
	m.pageRequest++
	request := m.pageRequest
	m.run.Go(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		page, err := connection.Client.Page(ctx, contract.PageRequest{Project: project, Path: path, Required: required})
		return func() {
			if request != m.pageRequest || m.Connection != connection || m.Project != project {
				return
			}
			if err != nil && page.Project == "" && !page.ProjectUnavailable {
				m.Error = "Could not read the project: " + message(err)
				return
			}
			m.Error = ""
			m.setPage(page, newTree, reveal)
		}
	})
}

func (m *Model) setPage(page contract.PageData, newTree, reveal bool) {
	documentChanged := page.Selected != m.Page.Selected || page.At != m.Page.At || page.Source != m.Page.Source || page.HasFile != m.Page.HasFile
	m.Page = page
	m.Tree = tree.New(page.Tree)
	m.TreeRead++
	if newTree {
		m.TreeOpen = tree.Open(page.Tree)
		m.TreeVersion++
	}
	if reveal && page.Selected != "" {
		m.RevealVersion++
	}
	if documentChanged {
		m.DocumentVersion++
		m.Lines = nil
	}
	m.documents.seed(page)
	m.decide()
}

// startStream follows the open project's live updates at open lease.
func (m *Model) startStream() {
	m.stopStream()
	connection, project := m.Connection, m.Project
	if connection == nil || project == "" || m.closed {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.stream = cancel
	m.streamID++
	id := m.streamID
	go m.follow(ctx, id, connection.Client, project, m.reconnect)
}

func (m *Model) stopStream() {
	if m.stream != nil {
		m.stream()
		m.stream = nil
	}
}

// follow reads a project's event stream until ctx ends, reopening it after
// an unexpected end and rereading what it may have missed.
func (m *Model) follow(ctx context.Context, id int, c client.Client, project string, delay time.Duration) {
	for first := true; ctx.Err() == nil; first = false {
		stream, err := c.Events(ctx, contract.EventsRequest{Project: project, Lease: "open"})
		if err == nil {
			if !first {
				m.run.Post(func() {
					if id == m.streamID {
						m.apply(contract.Event{Name: contract.EventChange, Change: &live.Everything})
					}
				})
			}
			for {
				event, err := stream.Next()
				if err != nil {
					break
				}
				m.run.Post(func() {
					if id == m.streamID {
						m.apply(event)
					}
				})
			}
			stream.Close()
		}
		if ctx.Err() != nil {
			return
		}
		m.run.Post(func() {
			if id == m.streamID {
				m.Live = live.Indicator{Label: "Off", Description: "Live updates disconnected; reconnecting."}
			}
		})
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// apply handles one live-update event on the UI thread.
func (m *Model) apply(event contract.Event) {
	switch event.Name {
	case contract.EventStatus:
		if event.Status != nil {
			m.Live = live.Describe(*event.Status)
		}
	case contract.EventHead:
		m.Head = event.Head
	case contract.EventChange:
		if event.Change == nil {
			return
		}
		document := ""
		if m.Page.At == "" {
			document = m.Page.Selected
		}
		targets := live.For(*event.Change, document)
		if targets.Page {
			m.loadPage(m.Page.Selected, m.Page.Selected != "", false, false)
		}
		if targets.Changes {
			// Changes are read on project load, so a change rereads them,
			// after a failed first read too.
			m.LoadChanges()
		}
		if open := m.Review; open != nil && live.Diff(*event.Change, open.File.Path, open.File.OldPath) {
			m.loadDiff()
		}
	}
}

// Title is the window title: the document and project, or the
// connection.
func (m *Model) Title() string {
	switch {
	case m.Review != nil:
		return m.Review.File.Path + " (changes) - " + m.projectName() + " - servef"
	case m.Page.Selected != "":
		return m.Page.Selected + " - " + m.projectName() + " - servef"
	case m.Project != "":
		return m.projectName() + " - servef"
	case m.Connection != nil:
		return m.Connection.Label + " - servef"
	}
	return "servef"
}

func (m *Model) projectName() string {
	for _, project := range m.Projects.Projects {
		if project.ID == m.Project {
			return project.Name
		}
	}
	if m.Page.RootName != "" {
		return m.Page.RootName
	}
	return m.Project
}

// message is an error's user-facing text.
func message(err error) string {
	var clientErr *client.Error
	if errors.As(err, &clientErr) && clientErr.Message != "" {
		return clientErr.Message
	}
	if errors.Is(err, client.ErrNoDaemon) {
		return "this server has no daemon"
	}
	if errors.Is(err, io.EOF) {
		return "the server closed the connection"
	}
	return err.Error()
}
