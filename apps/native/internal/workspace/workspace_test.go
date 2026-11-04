package workspace

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/apps/native/internal/connect"
	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/highlight"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/apps/native/internal/review"
	"github.com/flexdinesh/servef/pkg/contract"
)

const project = "/home/dee/demo"

// connector connects Local to a fake daemon and Server through TCP.
type connector struct {
	daemon   *fakeservef.Server
	localErr error
	servers  []connect.ListedServer
	locals   int
}

func (c *connector) Local(context.Context) (connect.Connection, error) {
	c.locals++
	if c.localErr != nil {
		return connect.Connection{}, c.localErr
	}
	return connect.Local(c.daemon.Socket), nil
}

func (c *connector) Server(target link.Target) connect.Connection { return connect.Server(target) }

func (c *connector) Servers(context.Context) ([]connect.ListedServer, error) { return c.servers, nil }

func setup(t *testing.T) (*Model, *fakeservef.Queue, *fakeservef.Server) {
	t.Helper()
	daemon := fakeservef.NewDaemon(t)
	daemon.Load(project)
	queue := fakeservef.NewQueue()
	model := New(queue, &connector{daemon: daemon})
	t.Cleanup(model.Close)
	return model, queue, daemon
}

func TestStartConnectsToTheDaemonAndListsProjects(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{})
	if model.Connecting != "This computer" {
		t.Fatalf("connecting %q", model.Connecting)
	}
	queue.Drain()
	if model.Connection == nil || !model.Connection.Daemon || model.Error != "" {
		t.Fatalf("connection %+v, error %q", model.Connection, model.Error)
	}
	if len(model.Projects.Projects) != 1 || model.Projects.Projects[0].ID != project {
		t.Fatalf("projects %+v", model.Projects)
	}
	if model.Title() != "This computer - servef" {
		t.Fatalf("title %q", model.Title())
	}
}

func TestDaemonFailureIsShown(t *testing.T) {
	queue := fakeservef.NewQueue()
	model := New(queue, &connector{localErr: errors.New("servef is not on PATH")})
	model.Handle(link.Request{})
	queue.Drain()
	if model.Connection != nil || !strings.Contains(model.Error, "servef is not on PATH") {
		t.Fatalf("connection %+v, error %q", model.Connection, model.Error)
	}
}

func TestOpenProjectAndDocuments(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	if model.Tree.Len() != 5 || !slices.Equal(model.TreeOpen, []string{"docs"}) || model.TreeVersion == 0 {
		t.Fatalf("tree %d entries, open %v, version %d", model.Tree.Len(), model.TreeOpen, model.TreeVersion)
	}
	if model.View.Kind != docview.None {
		t.Fatalf("view %+v", model.View)
	}

	revealed := model.RevealVersion
	model.OpenDocument("src/main.go")
	queue.Drain()
	if model.View.Kind != docview.Code || model.View.Language != "go" {
		t.Fatalf("code view %+v", model.View)
	}
	if model.Lines == nil || model.LinesVersion != model.DocumentVersion || len(model.Lines) != 7 {
		t.Fatalf("highlighted lines %d (version %d of %d)", len(model.Lines), model.LinesVersion, model.DocumentVersion)
	}
	if model.Lines[0][0].Category != highlight.Keyword {
		t.Fatalf("package is a keyword: %v", model.Lines[0])
	}
	if model.RevealVersion == revealed {
		t.Fatal("an opened document is revealed")
	}
	if model.Title() != "src/main.go - demo - servef" {
		t.Fatalf("title %q", model.Title())
	}

	model.OpenDocument("README.md")
	queue.Drain()
	if model.View.Kind != docview.Markdown || model.Lines != nil {
		t.Fatalf("markdown view %+v, lines %d", model.View, len(model.Lines))
	}
	model.SetMode(docview.Source)
	queue.Drain()
	if model.View.Kind != docview.Code || model.Lines == nil {
		t.Fatalf("markdown source %+v", model.View)
	}

	model.OpenDocument("missing.md")
	queue.Drain()
	if model.View.Kind != docview.Failed || model.View.Message != "File not found" || model.Tree.Len() != 5 {
		t.Fatalf("missing document %+v", model.View)
	}
}

func TestMarkdownLinksAndImages(t *testing.T) {
	model, queue, daemon := setup(t)
	daemon.Set(func(s *fakeservef.Server) { s.Images["docs/logo.png"] = []byte("PNG") })
	model.Handle(link.Request{})
	queue.Drain()
	model.openProject(project, "README.md")
	queue.Drain()
	if model.Page.Selected != "README.md" {
		t.Fatalf("selected %q", model.Page.Selected)
	}

	read := model.ImageReader()
	data, err := read(context.Background(), "docs/logo.png")
	if err != nil || string(data) != "PNG" {
		t.Fatalf("image %q, %v", data, err)
	}
	for _, destination := range []string{"https://example.com/x.png", "../../x.png", "missing.png"} {
		if _, err := read(context.Background(), destination); err == nil {
			t.Errorf("image %q loaded", destination)
		}
	}
	if _, err := New(queue, &connector{}).ImageReader()(context.Background(), "a.png"); err == nil {
		t.Fatal("image read without a connection")
	}

	model.FollowLink("docs/guide.md")
	queue.Drain()
	if model.Page.Selected != "docs/guide.md" {
		t.Fatalf("followed to %q", model.Page.Selected)
	}
	model.FollowLink("../README.md#demo")
	queue.Drain()
	if model.Page.Selected != "README.md" || model.Anchor != "demo" {
		t.Fatalf("followed to %q at %q", model.Page.Selected, model.Anchor)
	}
	version := model.AnchorVersion
	model.FollowLink("#see-also")
	if model.Anchor != "see-also" || model.AnchorVersion != version+1 {
		t.Fatalf("anchor %q %d", model.Anchor, model.AnchorVersion)
	}
	opened := []string{model.FollowLink("https://example.com/"), model.FollowLink("javascript:alert(1)"), model.FollowLink("../../etc/passwd")}
	queue.Drain()
	if !slices.Equal(opened, []string{"https://example.com/", "", ""}) || model.Page.Selected != "README.md" {
		t.Fatalf("opened %v, selected %q", opened, model.Page.Selected)
	}
}

func TestOpenPathThroughTheDaemon(t *testing.T) {
	model, queue, daemon := setup(t)
	daemon.Set(func(s *fakeservef.Server) {
		s.OpenResult = contract.DaemonOpenResult{Root: project, Added: true, Project: project}
	})
	model.Handle(link.Request{Path: project + "/src"})
	queue.Drain()
	var opened []string
	daemon.Get(func(s *fakeservef.Server) { opened = s.Opened })
	if !slices.Equal(opened, []string{project + "/src"}) {
		t.Fatalf("daemon opened %v", opened)
	}
	if model.Project != project || model.Tree.Len() == 0 {
		t.Fatalf("project %q, tree %d", model.Project, model.Tree.Len())
	}
}

func TestLinksToLocalAndServers(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{Link: &link.Target{Kind: link.Local, Project: project, Path: "docs/guide.md"}})
	queue.Drain()
	if model.Page.Selected != "docs/guide.md" || model.RevealVersion == 0 {
		t.Fatalf("local link opened %q", model.Page.Selected)
	}

	server := fakeservef.New(t)
	server.Load("/srv/demo")
	target, err := link.ParseServer(server.URL + "/?project=%2Fsrv%2Fdemo&path=src%2Fmain.go")
	if err != nil {
		t.Fatal(err)
	}
	model.Handle(link.Request{Link: &target})
	queue.Drain()
	if model.Connection == nil || model.Connection.Daemon || model.Project != "/srv/demo" || model.Page.Selected != "src/main.go" {
		t.Fatalf("server link: connection %+v, project %q, selected %q", model.Connection, model.Project, model.Page.Selected)
	}
	if model.Confirm != nil {
		t.Fatal("loopback servers need no confirmation")
	}
}

func TestRemoteServerLinksAreConfirmedOnce(t *testing.T) {
	model, _, _ := setup(t)
	remote := link.Target{Kind: link.Server, Host: "10.255.255.1", Port: 7975}
	model.OpenLink(remote)
	if model.Confirm == nil || *model.Confirm != remote || model.Connection != nil {
		t.Fatalf("confirm %+v, connection %+v", model.Confirm, model.Connection)
	}
	model.RejectLink()
	if model.Confirm != nil || model.Connection != nil {
		t.Fatal("rejected link connected")
	}
	model.OpenLink(remote)
	model.AcceptLink()
	if model.Confirm != nil || model.Connection == nil || model.Connection.ID != "http-10.255.255.1-7975" {
		t.Fatalf("accepted: confirm %+v, connection %+v", model.Confirm, model.Connection)
	}
	model.Disconnect()
	model.OpenLink(remote)
	if model.Confirm != nil {
		t.Fatal("an accepted server is not confirmed again")
	}
	// The queue is never drained: its project reads would dial 10.255.255.1.
}

func TestConnectAddress(t *testing.T) {
	model, queue, _ := setup(t)
	model.ConnectAddress("not an address")
	if model.Error == "" || model.Connection != nil {
		t.Fatal("bad address accepted")
	}
	server := fakeservef.New(t)
	server.Load(project)
	model.ConnectAddress(strings.TrimPrefix(server.URL, "http://"))
	queue.Drain()
	if model.Connection == nil || len(model.Projects.Projects) != 1 || model.Error != "" {
		t.Fatalf("connection %+v, projects %+v, error %q", model.Connection, model.Projects, model.Error)
	}
	model.OpenPath("/x")
	if !strings.Contains(model.Error, "daemon") {
		t.Fatalf("open path on a CLI server: %q", model.Error)
	}
}

func TestLiveUpdates(t *testing.T) {
	model, queue, daemon := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.openProject(project, "src/main.go")
	queue.Drain()

	daemon.Send(contract.Event{Name: contract.EventStatus, Status: &contract.StatusEvent{Mode: "watching"}})
	daemon.Send(contract.Event{Name: contract.EventHead, Head: &contract.HeadEvent{Branch: "main", Commit: strings.Repeat("a", 40), Subject: "init"}})
	if !queue.Until(func() bool { return model.Live.Label == "Live" && model.Head != nil }, 5*time.Second) {
		t.Fatalf("status %+v, head %+v", model.Live, model.Head)
	}

	edited := daemon
	_, pages := fakeservef.Sample(project)
	changed := pages[3]
	changed.Source = "package main\n\nfunc main() {}\n"
	edited.Page(changed)
	var before int
	daemon.Get(func(s *fakeservef.Server) { before = s.PageReads })
	version := model.DocumentVersion

	daemon.Send(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: []string{"README.md"}}})
	daemon.Send(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: []string{"src/main.go"}}})
	if !queue.Until(func() bool { return model.Page.Source == changed.Source }, 5*time.Second) {
		t.Fatalf("document not reread: %q", model.Page.Source)
	}
	var after int
	daemon.Get(func(s *fakeservef.Server) { after = s.PageReads })
	if after != before+1 {
		t.Fatalf("page reads %d -> %d: only the open document's change rereads", before, after)
	}
	if model.DocumentVersion != version+1 || model.Lines == nil || len(model.Lines) != 4 {
		t.Fatalf("document version %d -> %d, lines %d", version, model.DocumentVersion, len(model.Lines))
	}
}

func TestLiveStreamReconnects(t *testing.T) {
	server := fakeservef.New(t)
	server.Load(project)
	queue := fakeservef.NewQueue()
	model := New(queue, &connector{})
	model.reconnect = 10 * time.Millisecond
	t.Cleanup(model.Close)
	model.ConnectAddress(server.URL)
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()

	var before int
	server.Get(func(s *fakeservef.Server) { before = s.PageReads })
	server.Close()
	if !queue.Until(func() bool { return model.Live.Label == "Off" }, 5*time.Second) {
		t.Fatalf("live %+v after the server went away", model.Live)
	}
	var after int
	server.Get(func(s *fakeservef.Server) { after = s.PageReads })
	if after != before {
		t.Fatal("no reread while the stream is down")
	}
}

func TestStaleResultsAreDropped(t *testing.T) {
	model, queue, _ := setup(t)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	model.OpenDocument("src/main.go")
	model.OpenDocument("README.md")
	queue.Drain()
	if model.Page.Selected != "README.md" {
		t.Fatalf("the last request wins: %q", model.Page.Selected)
	}
	model.OpenDocument("src/main.go")
	model.CloseProject()
	queue.Drain()
	if model.Project != "" || model.Page.Selected != "" {
		t.Fatalf("closed project shows %q %q", model.Project, model.Page.Selected)
	}
}

func openWithChanges(t *testing.T) (*Model, *fakeservef.Queue, *fakeservef.Server) {
	t.Helper()
	model, queue, daemon := setup(t)
	daemon.LoadChanges(t, project)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	return model, queue, daemon
}

func TestSourceControlListsChangesAndOpensDiffs(t *testing.T) {
	model, queue, daemon := openWithChanges(t)
	// The project's load reads its changes, as the web does, so the
	// Explorer can show their statuses.
	if !model.ChangesRead {
		t.Fatal("changes not read on project load")
	}
	model.ShowPanel(SourceControl)
	queue.Drain()
	if !model.ChangesRead || len(model.Changes.Files) != 8 || model.Changes.Branch != "main" {
		t.Fatalf("changes %+v", model.Changes)
	}
	model.ShowPanel(SourceControl)
	model.ShowPanel(Explorer)
	model.ShowPanel(SourceControl)
	queue.Drain()
	daemon.Get(func(s *fakeservef.Server) {
		if s.ChangesReads != 3 {
			t.Fatalf("changes read %d times; the load and each opening of the panel read them", s.ChangesReads)
		}
	})

	model.OpenDocument("src/main.go")
	queue.Drain()
	main := model.Changes.Files[2]
	model.OpenChange(main)
	queue.Drain()
	open := model.Review
	if open == nil || open.Diff == nil || open.Error != "" || open.Layout.Kind != review.Text || len(open.Layout.Diff.Hunks) != 1 {
		t.Fatalf("review %+v", open)
	}
	if open.Old == nil || open.New == nil || open.LinesVersion != open.Version || len(open.New) != 9 {
		t.Fatalf("highlighted sides %d %d (version %d of %d)", len(open.Old), len(open.New), open.LinesVersion, open.Version)
	}
	if model.Title() != "code/main.go (changes) - demo - servef" {
		t.Fatalf("title %q", model.Title())
	}

	renamed := model.Changes.Files[4]
	model.OpenChange(renamed)
	queue.Drain()
	if model.Review.Layout.Kind != review.Unchanged {
		t.Fatalf("renamed layout %+v", model.Review.Layout)
	}
	var query []string
	daemon.Get(func(s *fakeservef.Server) {
		last := s.DiffQueries[len(s.DiffQueries)-1]
		query = []string{last.Get("path"), last.Get("oldPath"), last.Get("ignoreWhitespace")}
	})
	if !slices.Equal(query, []string{"guides/getting-started.md", "guides/intro.md", ""}) {
		t.Fatalf("diff query %q", query)
	}

	model.CloseReview()
	if model.Review != nil || model.Page.Selected != "src/main.go" {
		t.Fatalf("closing the review shows the document: %+v %q", model.Review, model.Page.Selected)
	}
	model.OpenChange(main)
	model.OpenDocument("README.md")
	queue.Drain()
	if model.Review != nil {
		t.Fatal("opening a document closes the review")
	}
}

func TestReviewOptions(t *testing.T) {
	model, queue, daemon := openWithChanges(t)
	model.OpenChange(contract.ChangedFile{Path: "notes/hunks.txt", Status: contract.ChangeModified})
	queue.Drain()
	if got := len(model.Review.Layout.Diff.Hunks); got != 3 || len(model.Review.Layout.Diff.Gaps) != 2 {
		t.Fatalf("hunks %d, gaps %d", got, len(model.Review.Layout.Diff.Gaps))
	}
	// Full file is the viewer's: the layout and its highlighting stay.
	version := model.Review.Version
	model.SetFullFile(true)
	queue.Drain()
	if !model.FullFile || model.Review.Version != version || model.Review.New == nil {
		t.Fatalf("full file: version %d -> %d", version, model.Review.Version)
	}
	model.SetIgnoreWhitespace(true)
	queue.Drain()
	daemon.Get(func(s *fakeservef.Server) {
		if got := s.DiffQueries[len(s.DiffQueries)-1].Get("ignoreWhitespace"); got != "1" {
			t.Fatalf("ignoreWhitespace=%q", got)
		}
	})
}

func TestReviewFailures(t *testing.T) {
	model, queue, daemon := openWithChanges(t)
	model.OpenChange(contract.ChangedFile{Path: "gone.txt", Status: contract.ChangeModified})
	queue.Drain()
	if model.Review.Error != "No changes to gone.txt since the last commit." || model.Review.Diff != nil {
		t.Fatalf("unchanged: %+v", model.Review)
	}
	model.OpenChange(contract.ChangedFile{Path: "assets/sample.bin", Status: contract.ChangeModified})
	queue.Drain()
	if model.Review.Layout.Kind != review.Binary || model.Review.Old != nil {
		t.Fatalf("binary: %+v", model.Review.Layout)
	}
	daemon.Set(func(s *fakeservef.Server) { delete(s.Changes, project) })
	model.LoadChanges()
	queue.Drain()
	if model.ChangesError != "Could not read changes: Project unavailable" {
		t.Fatalf("changes error %q", model.ChangesError)
	}
}

func TestLiveUpdatesRereadChangesAndTheOpenDiff(t *testing.T) {
	model, queue, daemon := openWithChanges(t)
	model.ShowPanel(SourceControl)
	queue.Drain()
	main := model.Changes.Files[2]
	model.OpenChange(main)
	queue.Drain()
	daemon.Send(contract.Event{Name: contract.EventStatus, Status: &contract.StatusEvent{Mode: "watching"}})
	if !queue.Until(func() bool { return model.Live.Label == "Live" }, 5*time.Second) {
		t.Fatalf("status %+v", model.Live)
	}

	edited := fakeservef.Diff(t, "diff-hunks.json")
	edited.Path = main.Path
	daemon.Set(func(s *fakeservef.Server) {
		s.Diffs[main.Path] = edited
		changes := s.Changes[project]
		changes.Files = changes.Files[:3]
		s.Changes[project] = changes
	})
	reads := func() (changes, diffs int) {
		daemon.Get(func(s *fakeservef.Server) { changes, diffs = s.ChangesReads, len(s.DiffQueries) })
		return changes, diffs
	}
	changesBefore, diffsBefore := reads()

	// Another file's edit rereads the list, not the open diff.
	daemon.Send(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: []string{"notes.txt"}}})
	if !queue.Until(func() bool { return len(model.Changes.Files) == 3 }, 5*time.Second) {
		t.Fatalf("changes not reread: %d files", len(model.Changes.Files))
	}
	if _, diffs := reads(); diffs != diffsBefore {
		t.Fatal("an unrelated edit reread the open diff")
	}

	daemon.Send(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: []string{main.Path}}})
	if !queue.Until(func() bool { return model.Review.Layout.Diff != nil && len(model.Review.Layout.Diff.Hunks) == 3 }, 5*time.Second) {
		t.Fatalf("diff not reread: %+v", model.Review.Layout)
	}
	if changes, diffs := reads(); changes != changesBefore+2 || diffs != diffsBefore+1 {
		t.Fatalf("reads: changes %d -> %d, diffs %d -> %d", changesBefore, changes, diffsBefore, diffs)
	}
}

// Opening Source Control while the load's read is in flight reads once.
func TestSourceControlJoinsTheLoadsRead(t *testing.T) {
	model, queue, daemon := setup(t)
	daemon.LoadChanges(t, project)
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	model.ShowPanel(SourceControl)
	queue.Drain()
	daemon.Get(func(s *fakeservef.Server) {
		if s.ChangesReads != 1 {
			t.Fatalf("changes read %d times", s.ChangesReads)
		}
	})
	if !model.ChangesRead {
		t.Fatal("changes not read")
	}
}

// A failed first read is retried by a live change and by a return to the
// window, at most once per throttle, as the web's useRefreshOnReturn.
func TestChangesRetryOnLiveUpdatesAndReturns(t *testing.T) {
	model, queue, daemon := setup(t)
	clock := time.Unix(1000, 0)
	model.now = func() time.Time { return clock }
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject(project)
	queue.Drain()
	if model.ChangesRead || model.ChangesError == "" {
		t.Fatalf("first read should fail: read %v, error %q", model.ChangesRead, model.ChangesError)
	}
	daemon.LoadChanges(t, project)
	daemon.Send(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: []string{"README.md"}}})
	if !queue.Until(func() bool { return model.ChangesRead }, 5*time.Second) {
		t.Fatalf("a live change rereads after a failure: %q", model.ChangesError)
	}
	reads := func() (n int) {
		daemon.Get(func(s *fakeservef.Server) { n = s.ChangesReads })
		return n
	}
	before := reads()
	clock = clock.Add(time.Second)
	model.Returned()
	queue.Drain()
	if reads() != before {
		t.Fatal("a return within the throttle reads nothing")
	}
	clock = clock.Add(2 * time.Second)
	model.Returned()
	model.Returned()
	queue.Drain()
	if reads() != before+1 {
		t.Fatalf("a return after the throttle reads once; %d -> %d", before, reads())
	}
}

func openFixtures(t *testing.T) (*Model, *fakeservef.Queue, *fakeservef.Server) {
	t.Helper()
	model, queue, daemon := setup(t)
	if err := daemon.LoadFixtures(fakeservef.FixtureDir()); err != nil {
		t.Fatal(err)
	}
	model.Handle(link.Request{})
	queue.Drain()
	model.OpenProject("/home/user/markdown")
	queue.Drain()
	return model, queue, daemon
}

func TestSetModeRemembersAndPrefersPreview(t *testing.T) {
	model, queue, _ := openFixtures(t)
	model.OpenDocument("guides/getting-started.md")
	queue.Drain()
	if model.View.Mode != docview.Source {
		t.Fatalf("Markdown opens by its type: %+v", model.View)
	}
	model.SelectLines(&docview.LineRange{Start: 2, End: 2})
	model.SetMode(docview.Preview)
	queue.Drain()
	if model.View.Kind != docview.Markdown || model.Selection() != nil {
		t.Fatalf("preview %+v, lines %+v", model.View, model.Selection())
	}
	// The preference follows to the next Markdown document; other files
	// show what they can.
	model.OpenDocument("guides/diagrams.md")
	queue.Drain()
	if model.View.Kind != docview.Markdown {
		t.Fatalf("preference: %+v", model.View)
	}
	model.OpenDocument("config/settings.json")
	queue.Drain()
	if model.View.Kind != docview.Code {
		t.Fatalf("no preview: %+v", model.View)
	}
	// Each document's own choice outlives leaving it.
	model.OpenDocument("guides/getting-started.md")
	queue.Drain()
	model.SetMode(docview.Source)
	model.OpenDocument("guides/diagrams.md")
	queue.Drain()
	if model.View.Kind != docview.Code {
		t.Fatalf("Source clears the preference: %+v", model.View)
	}
	model.SetMode(docview.Preview)
	model.OpenDocument("guides/getting-started.md")
	queue.Drain()
	if model.View.Kind != docview.Code {
		t.Fatalf("its own choice wins: %+v", model.View)
	}
}

func TestFormatIsCachedPerSource(t *testing.T) {
	model, queue, daemon := openFixtures(t)
	reads := func() int {
		var count int
		daemon.Get(func(s *fakeservef.Server) { count = len(s.FormatQueries) })
		return count
	}
	model.OpenDocument("config/settings.json")
	queue.Drain()
	model.SetMode(docview.Format)
	if !model.View.FormatPending || model.View.Kind != docview.Code {
		t.Fatalf("source shows while Format reads: %+v", model.View)
	}
	queue.Drain()
	if model.View.Kind != docview.Formatted || reads() != 1 {
		t.Fatalf("format %+v, reads %d", model.View, reads())
	}
	if model.Lines == nil || model.LinesVersion != model.CodeVersion {
		t.Fatal("formatted text is highlighted")
	}

	// Coming back shows Format again without another read.
	model.OpenDocument("config/broken.json")
	queue.Drain()
	model.OpenDocument("config/settings.json")
	queue.Drain()
	if model.View.Kind != docview.Formatted || reads() != 1 {
		t.Fatalf("return %+v, reads %d", model.View, reads())
	}

	// New source is formatted anew.
	var page contract.PageData
	fakeservef.Fixture(t, "page-json.json", &page)
	page.Source += " "
	daemon.Page(page)
	daemon.Send(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: []string{"config/settings.json"}}})
	if !queue.Until(func() bool { return reads() == 2 && model.View.Kind == docview.Formatted }, 5*time.Second) {
		t.Fatalf("reread %+v, reads %d", model.View, reads())
	}
}

func TestHistoryRestoresDocumentsAndLines(t *testing.T) {
	model, queue, _ := openFixtures(t)
	if model.CanGoBack() || model.CanGoForward() {
		t.Fatal("no history yet")
	}
	model.OpenDocument("guides/getting-started.md")
	queue.Drain()
	model.SelectLines(&docview.LineRange{Start: 2, End: 3})
	model.OpenDocument("config/settings.json")
	queue.Drain()
	if model.Selection() != nil {
		t.Fatalf("lines belong to their document: %+v", model.Selection())
	}
	model.GoBack()
	queue.Drain()
	if got := model.Selection(); model.Page.Selected != "guides/getting-started.md" || got == nil || got.Start != 2 || got.End != 3 {
		t.Fatalf("back to %q, lines %+v", model.Page.Selected, got)
	}
	model.GoForward()
	queue.Drain()
	if model.Page.Selected != "config/settings.json" || model.CanGoForward() {
		t.Fatalf("forward to %q", model.Page.Selected)
	}
	model.CloseProject()
	if model.CanGoBack() {
		t.Fatal("history belongs to a project")
	}
}

func TestLineLinkToTheOpenDocumentReadsNothing(t *testing.T) {
	model, queue, daemon := openFixtures(t)
	model.OpenDocument("config/settings.json")
	queue.Drain()
	var before, after int
	daemon.Get(func(s *fakeservef.Server) { before = s.PageReads })
	reveal := model.LineRevealVersion
	model.FollowLink("#L1")
	queue.Drain()
	daemon.Get(func(s *fakeservef.Server) { after = s.PageReads })
	if after != before || model.LineRevealVersion == reveal {
		t.Fatalf("reads %d -> %d, reveal %d -> %d", before, after, reveal, model.LineRevealVersion)
	}
	if got := model.Selection(); got == nil || got.Start != 1 {
		t.Fatalf("lines %+v", got)
	}
}
