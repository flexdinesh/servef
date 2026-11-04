package view

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/apps/native/internal/workspace"
	"github.com/flexdinesh/servef/pkg/contract"
)

// fixtureProject is the project of testdata/api/default, which the web's
// mock mode serves too.
const fixtureProject = "/home/user/markdown"

func newFixtureHarness(t *testing.T) *harness {
	t.Helper()
	daemon := fakeservef.NewDaemon(t)
	if err := daemon.LoadFixtures(fakeservef.FixtureDir()); err != nil {
		t.Fatal(err)
	}
	queue := fakeservef.NewQueue()
	model := workspace.New(queue, connector{daemon: daemon})
	t.Cleanup(model.Close)
	h := &harness{t: t, app: New(model, queue), queue: queue, daemon: daemon}
	h.tt = ui.NewTester(h.app.View, 1100, 700)
	h.click("Open this computer")
	return h
}

func (h *harness) open(path string) {
	h.t.Helper()
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: fixtureProject, Path: path})
	h.settle()
}

func (h *harness) wantNoText(text string) {
	h.t.Helper()
	if h.tt.HasText(text) {
		h.t.Fatalf("unexpected %q; texts %q", text, h.tt.Texts())
	}
}

func (h *harness) formatQueries() int {
	var count int
	h.daemon.Set(func(s *fakeservef.Server) { count = len(s.FormatQueries) })
	return count
}

func TestFormatIsReadOnDemandAndShown(t *testing.T) {
	h := newFixtureHarness(t)
	h.open("config/settings.json")
	h.wantText("Source", "Format")
	if h.app.Model.View.Kind != docview.Code || h.formatQueries() != 0 {
		t.Fatalf("opening asks for no format: view %+v, queries %d", h.app.Model.View, h.formatQueries())
	}
	source := h.app.code.Buffer.String()

	h.click("Format")
	if h.app.Model.View.Kind != docview.Formatted || h.formatQueries() != 1 {
		t.Fatalf("format: view %+v, queries %d", h.app.Model.View, h.formatQueries())
	}
	if got := h.app.code.Buffer.String(); got == source || got != h.app.Model.View.Text {
		t.Fatalf("format text %q", got)
	}
	if !h.app.code.ReadOnly || !strings.HasSuffix(h.app.code.Label, " formatted") {
		t.Fatalf("formatted editor %+v", h.app.code)
	}

	h.click("Source")
	if got := h.app.code.Buffer.String(); got != source {
		t.Fatalf("source restored as %q", got)
	}
	h.click("Format")
	if h.formatQueries() != 1 {
		t.Fatalf("format read again: %d", h.formatQueries())
	}
}

func TestFormatFailureKeepsSourceAndRetries(t *testing.T) {
	h := newFixtureHarness(t)
	h.open("config/broken.json")
	source := h.app.code.Buffer.String()
	h.click("Format")
	h.wantText("Couldn't format: unexpected end of JSON input.")
	view := h.app.Model.View
	if view.Kind != docview.Code || view.Mode != docview.Source || h.app.code.Buffer.String() != source {
		t.Fatalf("failure keeps Source: %+v", view)
	}
	h.click("Format")
	if h.formatQueries() != 2 {
		t.Fatalf("Format retries: %d", h.formatQueries())
	}
	h.wantText("Couldn't format: unexpected end of JSON input.")
}

func TestFileNotesFollowTheServer(t *testing.T) {
	h := newFixtureHarness(t)
	h.open("assets/sample.bin")
	h.wantText("Binary file", "servef shows text and images only.")
	h.wantNoText("Source")

	h.open("logs/enormous.log")
	h.wantText("Too large to display", "servef shows text files up to 32 MB.")
	h.wantNoText("Source")

	h.open("logs/huge.log")
	var notice string
	for _, text := range h.tt.Texts() {
		if strings.HasPrefix(text, "Large file (") {
			notice = text
		}
	}
	if !strings.HasSuffix(notice, ": preview and formatting are off. Not included in search.") {
		t.Fatalf("notice %q; texts %q", notice, h.tt.Texts())
	}
	if h.app.Model.View.Highlight {
		t.Fatal("the server disabled highlighting")
	}
}

func TestImageDocumentZooms(t *testing.T) {
	h := newFixtureHarness(t)
	h.open("assets/preview.png")
	h.settle()
	viewer := &h.app.doc.viewer
	if h.app.Model.View.Kind != docview.Image || viewer.Status != docview.ImageReady {
		t.Fatalf("image %+v, viewer %+v", h.app.Model.View, viewer)
	}
	if !viewer.Fit() {
		t.Fatal("an image opens fitted")
	}
	h.wantText("Fit", percentLabel(viewer.Percent()))

	// The image has the focus when it opens, as on the web.
	h.tt.Key(0, ui.Key1)
	h.settle()
	if viewer.Scale() != 1 || viewer.Percent() != 100 {
		t.Fatalf("1 is actual size: %+v", viewer)
	}
	h.tt.Key(0, ui.KeyMinus)
	h.settle()
	if viewer.Percent() >= 100 {
		t.Fatalf("minus zooms out: %+v", viewer)
	}
	h.tt.Key(0, ui.KeyEqual)
	h.settle()
	if viewer.Percent() != 100 {
		t.Fatalf("plus zooms in: %+v", viewer)
	}
	h.tt.Key(0, ui.Key0)
	h.settle()
	if !viewer.Fit() {
		t.Fatalf("0 fits: %+v", viewer)
	}

	if err := h.tt.Click("Zoom in"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if viewer.Fit() {
		t.Fatal("zoom in leaves fit")
	}
	h.click("Fit")
	if !viewer.Fit() {
		t.Fatal("Fit fits")
	}
}

func TestImageThatCannotDecodeSaysSo(t *testing.T) {
	h := newFixtureHarness(t)
	h.daemon.Set(func(s *fakeservef.Server) { s.Images["assets/preview.png"] = []byte("not an image") })
	h.open("assets/preview.png")
	h.settle()
	h.wantText("Can't display this image.")
}

func clickRect(tt *ui.Tester, r ui.Rect, mods ui.Modifiers) {
	tt.ClickAtWith(mods, r.X+r.W/2, r.Y+r.H/2)
}

// lineRect finds a line number in the gutter.
func (h *harness) lineRect(line int) ui.Rect {
	h.t.Helper()
	r, ok := h.tt.Find(strconv.Itoa(line))
	if !ok {
		h.t.Fatalf("line %d not shown; texts %q", line, h.tt.Texts())
	}
	return r
}

func wantLines(t *testing.T, h *harness, want *docview.LineRange) {
	t.Helper()
	got := h.app.Model.Selection()
	if (got == nil) != (want == nil) || (got != nil && *got != *want) {
		t.Fatalf("selection %+v, want %+v", got, want)
	}
}

func TestGutterSelectsLines(t *testing.T) {
	h := newFixtureHarness(t)
	longFile(t, h, "logs/long.log", 50)
	h.open("logs/long.log")
	h.settle()

	clickRect(h.tt, h.lineRect(3), 0)
	h.settle()
	wantLines(t, h, &docview.LineRange{Start: 3, End: 3})
	h.wantText("Ln 3")

	clickRect(h.tt, h.lineRect(5), ui.Shift)
	h.settle()
	wantLines(t, h, &docview.LineRange{Start: 3, End: 5})
	h.wantText("Ln 3–5")

	if err := h.tt.Click("Copy logs/long.log:3-5"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if got := h.tt.Clipboard(); got != "logs/long.log:3-5" {
		t.Fatalf("clipboard %q", got)
	}

	if err := h.tt.Click("Copy path"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if got := h.tt.Clipboard(); got != "logs/long.log" {
		t.Fatalf("clipboard %q", got)
	}

	clickRect(h.tt, h.lineRect(2), 0)
	h.settle()
	wantLines(t, h, &docview.LineRange{Start: 2, End: 2})
	h.tt.Key(0, ui.KeyEscape)
	h.settle()
	wantLines(t, h, nil)
}

func TestGutterDragSelectsOnRelease(t *testing.T) {
	h := newFixtureHarness(t)
	longFile(t, h, "logs/long.log", 50)
	h.open("logs/long.log")
	h.settle()
	from, to := h.lineRect(2), h.lineRect(4)
	h.tt.Press(from.X+from.W/2, from.Y+from.H/2)
	h.tt.Frame()
	h.tt.Move(to.X+to.W/2, to.Y+to.H/2)
	h.tt.Frame()
	wantLines(t, h, nil)
	if got := h.app.doc.lineDrag; got == nil || *got != (docview.LineRange{Start: 2, End: 4}) {
		t.Fatalf("drag %+v", got)
	}
	h.tt.Release(to.X+to.W/2, to.Y+to.H/2)
	h.settle()
	wantLines(t, h, &docview.LineRange{Start: 2, End: 4})
}

func TestOtherViewsClearTheSelection(t *testing.T) {
	h := newFixtureHarness(t)
	h.open("guides/getting-started.md")
	h.settle()
	h.app.Model.SetMode(docview.Source)
	h.settle()
	clickRect(h.tt, h.lineRect(2), 0)
	h.settle()
	wantLines(t, h, &docview.LineRange{Start: 2, End: 2})
	h.click("Preview")
	wantLines(t, h, nil)

	h.open("config/settings.json")
	h.settle()
	h.click("Format")
	if h.app.Model.View.Kind != docview.Formatted {
		t.Fatalf("view %+v", h.app.Model.View)
	}
	// Formatted lines do not address the file.
	clickRect(h.tt, h.lineRect(2), 0)
	h.settle()
	wantLines(t, h, nil)
}

func TestLineLinkOpensSourceAtItsLines(t *testing.T) {
	h := newFixtureHarness(t)
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: fixtureProject, Path: "guides/getting-started.md", Fragment: "L2-L3"})
	h.settle()
	if h.app.Model.View.Kind != docview.Code {
		t.Fatalf("a line link opens Source: %+v", h.app.Model.View)
	}
	wantLines(t, h, &docview.LineRange{Start: 2, End: 3})
	h.wantText("Ln 2–3")
	got, err := link.Parse(h.app.Model.DocumentLink())
	if err != nil || got.Project != fixtureProject || got.Path != "guides/getting-started.md" || got.Fragment != "L2-L3" {
		t.Fatalf("link %+v, %v", got, err)
	}
}

func TestSourceMenuCopies(t *testing.T) {
	h := newFixtureHarness(t)
	longFile(t, h, "logs/long.log", 50)
	h.open("logs/long.log")
	h.settle()
	clickRect(h.tt, h.lineRect(3), 0)
	h.settle()
	r, ok := h.tt.Find("Plain text source")
	if !ok {
		t.Fatalf("no editor; texts %q", h.tt.Texts())
	}
	h.tt.RightClickAt(r.X+r.W/2, r.Y+r.H/2)
	h.settle()
	if err := h.tt.ChooseMenuItem("Copy logs/long.log:3"); err != nil {
		t.Fatalf("%v; menu %q", err, h.tt.Menu())
	}
	h.settle()
	if got := h.tt.Clipboard(); got != "logs/long.log:3" {
		t.Fatalf("clipboard %q", got)
	}
	h.tt.RightClickAt(r.X+r.W/2, r.Y+r.H/2)
	h.settle()
	if err := h.tt.ChooseMenuItem("Copy Link"); err != nil {
		t.Fatalf("%v; menu %q", err, h.tt.Menu())
	}
	h.settle()
	if got := h.tt.Clipboard(); !strings.HasSuffix(got, "#L3") || !strings.HasPrefix(got, "servef-native://local/view?") {
		t.Fatalf("clipboard %q", got)
	}
}

// longFile is a page in the sample project long enough to scroll.
func longFile(t *testing.T, h *harness, path string, lines int) {
	t.Helper()
	var text strings.Builder
	for i := range lines {
		fmt.Fprintf(&text, "line %d\n", i+1)
	}
	page := contract.PageData{
		Project: fixtureProject, Selected: path, HasFile: true, FileSize: int64(text.Len()), Source: text.String(),
		DocumentType: &contract.FileType{ID: "text", Label: "Plain text", Language: "text", DefaultMode: contract.ModeSource},
	}
	h.daemon.Page(page)
}

func TestReturningRestoresScrollAndSelection(t *testing.T) {
	h := newFixtureHarness(t)
	longFile(t, h, "logs/long.log", 500)
	h.open("logs/long.log")
	h.settle()
	h.app.code.SetScrollY(1200)
	h.settle()
	clickRect(h.tt, h.lineRect(70), 0)
	h.settle()
	selected := docview.LineRange{Start: 70, End: 70}
	wantLines(t, h, &selected)
	scroll := h.app.code.ScrollY()
	if scroll == 0 {
		t.Fatal("scrolled")
	}

	h.open("config/settings.json")
	h.settle()
	wantLines(t, h, nil)
	if got := h.app.code.ScrollY(); got != 0 {
		t.Fatalf("another document starts at the top: %v", got)
	}

	h.app.Model.GoBack()
	h.settle()
	if h.app.Model.Page.Selected != "logs/long.log" {
		t.Fatalf("back to %q", h.app.Model.Page.Selected)
	}
	wantLines(t, h, &selected)
	if got := h.app.code.ScrollY(); got != scroll {
		t.Fatalf("scroll %v, want %v", got, scroll)
	}
	h.app.Model.GoForward()
	h.settle()
	if h.app.Model.Page.Selected != "config/settings.json" {
		t.Fatalf("forward to %q", h.app.Model.Page.Selected)
	}
}

func TestBackAndForwardKeys(t *testing.T) {
	h := newFixtureHarness(t)
	h.open("config/settings.json")
	h.open("config/broken.json")
	h.tt.Key(ui.Alt, ui.KeyLeft)
	h.settle()
	if got := h.app.Model.Page.Selected; got != "config/settings.json" {
		t.Fatalf("Alt+Left to %q", got)
	}
	h.tt.Key(ui.Alt, ui.KeyRight)
	h.settle()
	if got := h.app.Model.Page.Selected; got != "config/broken.json" {
		t.Fatalf("Alt+Right to %q", got)
	}
}

func TestRereadKeepsScroll(t *testing.T) {
	h := newFixtureHarness(t)
	longFile(t, h, "logs/long.log", 500)
	h.open("logs/long.log")
	h.settle()
	h.app.code.SetScrollY(1200)
	h.settle()
	scroll := h.app.code.ScrollY()
	state := h.app.code

	longFile(t, h, "logs/long.log", 501)
	h.daemon.Send(contract.Event{Name: contract.EventChange, Change: &contract.ChangeEvent{Paths: []string{"logs/long.log"}}})
	if !h.queue.Until(func() bool { return strings.Contains(h.app.Model.Page.Source, "line 501") }, 5*time.Second) {
		t.Fatal("not reread")
	}
	h.settle()
	if h.app.code != state {
		t.Fatal("a reread keeps the editor")
	}
	if got := state.Buffer.LineCount(); got != 502 {
		t.Fatalf("reread %d lines", got)
	}
	if got := state.ScrollY(); got != scroll {
		t.Fatalf("scroll %v, want %v", got, scroll)
	}
}

func TestPreviewKeepsScrollOnReturn(t *testing.T) {
	h := newFixtureHarness(t)
	h.open("guides/diagrams.md")
	h.click("Preview")
	h.settle()
	h.app.doc.preview.Y = 400
	h.settle()
	h.open("config/settings.json")
	h.settle()
	h.app.Model.GoBack()
	h.settle()
	if h.app.Model.View.Kind != docview.Markdown || h.app.doc.preview.Y != 400 {
		t.Fatalf("view %+v, scroll %v", h.app.Model.View, h.app.doc.preview.Y)
	}
}

func (h *harness) expandDiagram() *expandedDiagram {
	h.t.Helper()
	h.open("guides/request-flow.mmd")
	if h.app.Model.View.Kind != docview.Markdown {
		h.click("Preview")
	}
	for range 40 {
		h.settle()
		if h.tt.HasText("Expand diagram") {
			break
		}
	}
	if err := h.tt.Click("Expand diagram"); err != nil {
		h.t.Fatalf("%v; texts %q", err, h.tt.Texts())
	}
	for range 40 {
		h.settle()
		if e := h.app.doc.diagram; e != nil && e.view != nil {
			return e
		}
	}
	h.t.Fatalf("diagram did not open: %+v", h.app.doc.diagram)
	return nil
}

func TestExpandedDiagramZoomsAndPans(t *testing.T) {
	h := newFixtureHarness(t)
	e := h.expandDiagram()
	h.wantText("Reset zoom", "Close diagram", percentLabel(docview.ZoomPercent(e.view.Zoom)))
	fit := *e.view

	if err := h.tt.Click("Zoom in"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if e.view.Zoom <= fit.Zoom {
		t.Fatalf("zoom in %v from %v", e.view.Zoom, fit.Zoom)
	}
	if err := h.tt.Click("Reset zoom"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if *e.view != fit {
		t.Fatalf("reset %+v, want %+v", *e.view, fit)
	}

	r, ok := h.tt.Find("Mermaid diagram canvas")
	if !ok {
		t.Fatalf("no canvas; texts %q", h.tt.Texts())
	}
	x, y := r.X+r.W/2, r.Y+r.H/2

	// A headless wheel carries no modifiers: the handler takes the event a
	// window delivers for Ctrl and Cmd with the wheel.
	e.input(ui.InputEvent{Kind: ui.InputScroll, DY: -1, Mods: ui.Ctrl, X: 40, Y: 40})
	h.settle()
	if e.view.Zoom <= fit.Zoom {
		t.Fatalf("Ctrl+wheel zoom %v from %v", e.view.Zoom, fit.Zoom)
	}
	e.input(ui.InputEvent{Kind: ui.InputScroll, DY: 1, Mods: ui.Super, X: 40, Y: 40})
	h.settle()
	if e.view.Zoom != fit.Zoom {
		t.Fatalf("Cmd+wheel zoom %v from %v", e.view.Zoom, fit.Zoom)
	}
	h.tt.Pinch(x, y, 2)
	h.settle()
	if e.view.Zoom <= fit.Zoom {
		t.Fatalf("pinch zoom %v from %v", e.view.Zoom, fit.Zoom)
	}
	zoomed := *e.view
	h.tt.Wheel(x, y, 0, 10)
	h.settle()
	if e.view.Y != zoomed.Y-10 || e.view.Zoom != zoomed.Zoom {
		t.Fatalf("wheel pans: %+v from %+v", *e.view, zoomed)
	}
	zoomed = *e.view

	h.tt.Press(x, y)
	h.tt.Frame()
	h.tt.Move(x+30, y+20)
	h.tt.Frame()
	h.tt.Release(x+30, y+20)
	h.settle()
	if e.view.X != zoomed.X+30 || e.view.Y != zoomed.Y+20 || e.view.Zoom != zoomed.Zoom {
		t.Fatalf("pan %+v from %+v", *e.view, zoomed)
	}

	if err := h.tt.Click("Close diagram"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if h.app.doc.diagram != nil {
		t.Fatal("the dialog keeps its render after closing")
	}
}

func TestInlineDiagramDoesNotZoom(t *testing.T) {
	h := newFixtureHarness(t)
	h.open("guides/request-flow.mmd")
	if h.app.Model.View.Kind != docview.Markdown {
		h.click("Preview")
	}
	for range 40 {
		h.settle()
		if h.tt.HasText("Expand diagram") {
			break
		}
	}
	h.wantNoText("Zoom in")
	h.wantNoText("Reset zoom")
}

// TestCheckerboardShowsWhereverTheImageScrolls draws a fully transparent
// image: the frame's tiles must show in the pane at any scroll.
func TestCheckerboardShowsWhereverTheImageScrolls(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 900, 900))); err != nil {
		t.Fatal(err)
	}
	h := newFixtureHarness(t)
	h.daemon.Set(func(s *fakeservef.Server) { s.Images["assets/preview.png"] = data.Bytes() })
	h.open("assets/preview.png")
	h.settle()
	viewer := &h.app.doc.viewer
	for range 6 {
		viewer.ZoomIn()
	}
	h.settle()
	if viewer.Percent() <= 100 {
		t.Fatalf("zoom %d", viewer.Percent())
	}
	tones := func() int {
		shot := h.tt.Image()
		seen := map[color.RGBA]bool{}
		for y := 300; y < 332; y++ {
			for x := 650; x < 682; x++ {
				seen[shot.RGBAAt(x*shot.Bounds().Dx()/1100, y*shot.Bounds().Dy()/700)] = true
			}
		}
		return len(seen)
	}
	if got := tones(); got < 2 {
		t.Fatalf("no tiles at the start: %d tones", got)
	}
	h.app.doc.imageScroll.X, h.app.doc.imageScroll.Y = 2500, 2500
	h.settle()
	if h.app.doc.imageScroll.X == 0 || h.app.doc.imageScroll.Y == 0 {
		t.Fatalf("not scrolled: %+v", h.app.doc.imageScroll)
	}
	if got := tones(); got < 2 {
		t.Fatalf("no tiles after scrolling to %v, %v: %d tones", h.app.doc.imageScroll.X, h.app.doc.imageScroll.Y, got)
	}
}

func TestLineLinkRevealsItsLine(t *testing.T) {
	h := newFixtureHarness(t)
	longFile(t, h, "logs/long.log", 500)
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: fixtureProject, Path: "logs/long.log", Fragment: "L300"})
	for range 5 {
		h.settle()
	}
	wantLines(t, h, &docview.LineRange{Start: 300, End: 300})
	h.wantText("300", "Copy logs/long.log:300")
}
