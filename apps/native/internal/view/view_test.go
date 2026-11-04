package view

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/flexdinesh/servef/apps/native/internal/connect"
	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/highlight"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
	"github.com/flexdinesh/servef/apps/native/internal/workspace"
)

const project = "/home/dee/demo"

// connector reaches the fake daemon, and every server at the fake remote
// when one is set, so tests never dial other computers.
type connector struct{ daemon, remote *fakeservef.Server }

func (c connector) Local(context.Context) (connect.Connection, error) {
	return connect.Local(c.daemon.Socket), nil
}
func (c connector) Server(target link.Target) connect.Connection {
	if c.remote != nil {
		local, _ := link.ParseServer(c.remote.URL)
		connection := connect.Server(local)
		connection.Label = target.Origin()[len("http://"):]
		return connection
	}
	return connect.Server(target)
}
func (c connector) Servers(context.Context) ([]connect.ListedServer, error) {
	return []connect.ListedServer{{Host: "localhost", Port: 7971, Origin: "http://localhost:7971"}}, nil
}

type harness struct {
	t      *testing.T
	app    *App
	queue  *fakeservef.Queue
	daemon *fakeservef.Server
	tt     *ui.Tester
}

func newHarness(t *testing.T) *harness {
	return newHarnessWith(t, nil)
}

func newHarnessWith(t *testing.T, remote *fakeservef.Server) *harness {
	daemon := fakeservef.NewDaemon(t)
	daemon.Load(project)
	daemon.Set(func(s *fakeservef.Server) { s.Images["docs/logo.png"] = pngBytes() })
	queue := fakeservef.NewQueue()
	model := workspace.New(queue, connector{daemon: daemon, remote: remote})
	t.Cleanup(model.Close)
	app := New(model, queue)
	h := &harness{t: t, app: app, queue: queue, daemon: daemon}
	h.tt = ui.NewTester(app.View, 1100, 700)
	return h
}

// settle runs queued work and draws until nothing is pending.
func (h *harness) settle() {
	for range 5 {
		h.queue.Drain()
		h.tt.Frame()
	}
}

func (h *harness) click(text string) {
	h.t.Helper()
	if err := h.tt.Click(text); err != nil {
		h.t.Fatalf("click %q: %v; texts %q", text, err, h.tt.Texts())
	}
	h.settle()
}

func (h *harness) wantText(texts ...string) {
	h.t.Helper()
	for _, text := range texts {
		if !h.tt.HasText(text) {
			h.t.Fatalf("missing %q; texts %q", text, h.tt.Texts())
		}
	}
}

func TestBrowseProjectTreeAndDocuments(t *testing.T) {
	h := newHarness(t)
	h.wantText("Open this computer")
	h.click("Open this computer")
	h.wantText("This computer", "demo  ·  main", project)

	h.click("demo  ·  main")
	h.wantText("demo · main", "docs", "guide.md", "src", "README.md", "3 files. Choose one in the tree.")
	if h.tt.HasText("main.go") {
		t.Fatal("src starts closed, as the server marked it")
	}

	h.click("README.md")
	h.wantText("Demo", "guide", "Preview", "Source")
	if title := h.app.Model.Title(); title != "README.md - demo - servef" {
		t.Fatalf("title %q", title)
	}

	// A relative link opens the linked document; its directory opens.
	h.click("guide")
	h.wantText("Guide", "Install")
	if h.app.Model.Page.Selected != "docs/guide.md" {
		t.Fatalf("selected %q", h.app.Model.Page.Selected)
	}

	// Source view of the Markdown file.
	h.click("Source")
	if h.app.code == nil || !strings.HasPrefix(h.app.code.Buffer.String(), "# Guide") || !h.app.code.ReadOnly {
		t.Fatalf("source editor %+v", h.app.code)
	}

	h.click("Projects")
	h.wantText("This computer", "demo  ·  main")
}

func TestSourceViewIsHighlightedReadOnly(t *testing.T) {
	h := newHarness(t)
	h.click("Open this computer")
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: project, Path: "src/main.go"})
	h.settle()
	h.wantText("src/main.go", "main.go", "go")
	state := h.app.code
	if state == nil || state.LineNumbers != editor.LineNumbersOn || !state.ReadOnly {
		t.Fatalf("editor %+v", state)
	}
	if got := state.Buffer.Line(3); got != "func main() {" {
		t.Fatalf("line 3 %q", got)
	}
	language := state.Buffer.Language()
	if language == nil || language.Highlighter == nil {
		t.Fatal("source has no highlighter")
	}
	tokens, final := language.Highlighter(state.Buffer.Snapshot(), func() {}).Tokens(3, "func main() {", nil)
	if !final || len(tokens) == 0 || tokens[0].Kind != editor.TokenKeyword || strings.TrimSpace("func main() {"[:tokens[0].End]) != "func" {
		t.Fatalf("tokens of line 3: %v %v", tokens, final)
	}
	// Typing changes nothing.
	h.tt.Type("x")
	if strings.Contains(state.Buffer.String(), "x") {
		t.Fatal("read-only source was edited")
	}
}

func TestMarkdownImagesLoadThroughTheServer(t *testing.T) {
	h := newHarness(t)
	h.click("Open this computer")
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: project, Path: "README.md"})
	h.settle()
	read := h.app.Model.ImageReader()
	data, err := read(context.Background(), "docs/logo.png")
	if err != nil || len(data) == 0 {
		t.Fatalf("image %d bytes, %v", len(data), err)
	}
	if _, err := h.app.markdown.LoadImage("docs/logo.png"); err != nil {
		t.Fatalf("preview loads project images: %v", err)
	}
	if _, err := h.app.markdown.LoadImage("https://example.com/a.png"); err == nil {
		t.Fatal("preview loaded a remote image")
	}
}

func TestExternalLinksOpenInTheSystem(t *testing.T) {
	h := newHarness(t)
	h.daemon.Set(func(s *fakeservef.Server) {
		page := s.Pages[project]["README.md"]
		page.Source = "[site](https://example.com/docs) and [bad](javascript:alert(1))\n"
		s.Pages[project]["README.md"] = page
	})
	h.click("Open this computer")
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: project, Path: "README.md"})
	h.settle()
	h.click("site")
	h.click("bad")
	if got := h.tt.OpenedURLs(); !slices.Equal(got, []string{"https://example.com/docs"}) {
		t.Fatalf("opened %v", got)
	}
}

func TestRemoteLinksAskFirst(t *testing.T) {
	remote := fakeservef.New(t)
	remote.Load("/srv/demo")
	h := newHarnessWith(t, remote)
	h.app.Model.OpenLink(link.Target{Kind: link.Server, Host: "10.255.255.1", Port: 7975})
	h.tt.Frame()
	h.wantText("Open 10.255.255.1:7975?")
	h.click("Cancel")
	if h.app.Model.Confirm != nil || h.app.Model.Connection != nil {
		t.Fatal("cancel connected")
	}
	h.app.Model.OpenLink(link.Target{Kind: link.Server, Host: "10.255.255.1", Port: 7975})
	h.tt.Frame()
	h.click("Open")
	if h.app.Model.Connection == nil || h.app.Model.Connection.Label != "10.255.255.1:7975" {
		t.Fatalf("connection %+v", h.app.Model.Connection)
	}
	h.wantText("10.255.255.1:7975", "/srv/demo")
}

func TestConnectPageListsServersAndTakesAddresses(t *testing.T) {
	h := newHarness(t)
	h.app.Model.RefreshServers()
	h.settle()
	h.wantText("Servers on this computer", "http://localhost:7971")

	server := fakeservef.New(t)
	server.Load("/srv/demo")
	if err := h.tt.Click("Server address"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type(server.URL)
	h.click("Connect")
	h.wantText(strings.TrimPrefix(server.URL, "http://"), "/srv/demo")
}

func TestLiveStatusShowsInTheStatusBar(t *testing.T) {
	h := newHarness(t)
	h.click("Open this computer")
	h.click("demo  ·  main")
	h.daemon.Send(eventStatus("watching"))
	if !h.queue.Until(func() bool { return h.app.Model.Live.Label == "Live" }, 5*time.Second) {
		t.Fatal("no status")
	}
	h.tt.Frame()
	h.wantText("Live", "This computer")
}

func TestThemesFollowTheAppearance(t *testing.T) {
	for _, dark := range []bool{false, true} {
		system := ui.LightTheme()
		if dark {
			system = ui.DarkTheme()
		}
		window, palette := windowTheme(system)
		if palette.Bg != theme.For(dark).Bg || window.Background != ui.Hex(palette.Bg) || window.Text != ui.Hex(palette.TextPrimary) {
			t.Fatalf("dark %v: window theme %+v", dark, window)
		}
		code := codeTheme(window, palette)
		for _, category := range highlight.Categories {
			style := code.Tokens[tokenKinds[category]]
			if style.Color != ui.Hex(palette.Syntax[category]) || style.Italic != (category == highlight.Comment) || style.Bold {
				t.Errorf("dark %v %s: %+v", dark, category.Name(), style)
			}
		}
	}
	h := newHarness(t)
	h.tt.SetDark(true)
	h.tt.Frame()
	h.wantText("Open this computer")
}

func TestHighlighterClampsSpans(t *testing.T) {
	h := &lineHighlighter{lines: [][]highlight.Span{{{End: 4, Category: highlight.Keyword}, {End: 99, Category: highlight.Plain}}}}
	tokens, _ := h.Tokens(0, "func x", nil)
	if len(tokens) != 2 || tokens[1].End != 6 {
		t.Fatalf("tokens %v", tokens)
	}
	if tokens, _ := h.Tokens(5, "beyond", nil); len(tokens) != 1 || tokens[0].Kind != editor.TokenText {
		t.Fatalf("missing line %v", tokens)
	}
}
