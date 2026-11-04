package view

import (
	"fmt"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/apps/native/internal/workspace"
)

// openMarkdown serves source as docs/guide.md and opens its preview.
func openMarkdown(t *testing.T, source string) *harness {
	t.Helper()
	h := newHarness(t)
	h.daemon.Set(func(s *fakeservef.Server) {
		page := s.Pages[project]["docs/guide.md"]
		page.Source = source
		s.Pages[project]["docs/guide.md"] = page
	})
	h.click("Open this computer")
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: project, Path: "docs/guide.md"})
	h.settle()
	return h
}

func TestMermaidFencesDrawAsDiagrams(t *testing.T) {
	h := openMarkdown(t, "# Flow\n\n```mermaid\nflowchart LR\n  A[Start] --> B[Done]\n```\n")
	if _, ok := h.tt.Find("Mermaid diagram"); !ok {
		t.Fatalf("no diagram; texts %q", h.tt.Texts())
	}
	if h.tt.HasText("flowchart LR") {
		t.Fatal("the source shows beside the diagram")
	}
}

func TestMermaidSyntaxErrorsShowTheSource(t *testing.T) {
	h := openMarkdown(t, "```mermaid\nflowchart LR\n  A --> B\n  B -->\n```\n")
	if _, ok := h.tt.Find("Mermaid diagram"); ok {
		t.Fatal("a diagram drew for a broken source")
	}
	// Ondine's diagnostic, as the web app shows it.
	h.wantText("flowchart LR", "Parse error on line 5:", "Expecting 'NODE_STRING'")
}

func TestFencesHighlightWithTheirLanguage(t *testing.T) {
	h := openMarkdown(t, "```go\nfunc main() {}\n```\n\n```klingon\nqapla'\n```\n")
	h.wantText("func", "qapla'")
	if _, ok := h.tt.Find("Code, go"); !ok {
		t.Fatalf("go fence is not labeled; texts %q", h.tt.Texts())
	}
	if _, ok := h.tt.Find("Code, klingon"); ok {
		t.Fatal("an unknown language is labeled")
	}
	if !h.tt.HasText("go") {
		t.Fatal("no language label")
	}
}

// countingRunner counts the view's background jobs.
type countingRunner struct {
	workspace.Runner
	jobs int
}

func (r *countingRunner) Go(work func() func()) {
	r.jobs++
	r.Runner.Go(work)
}

func countJobs(h *harness) *countingRunner {
	r := &countingRunner{Runner: h.queue}
	h.app.run = r
	return r
}

func countText(h *harness, text string) int {
	n := 0
	for _, t := range h.tt.Texts() {
		if t == text {
			n++
		}
	}
	return n
}

// Every fence of a long document highlights once: one job per fence, all
// labeled, and the view goes quiet (more fences than any cache bound once
// restarted jobs every frame and never labeled one).
func TestManyFencesHighlightOnce(t *testing.T) {
	var doc strings.Builder
	for i := range 200 {
		fmt.Fprintf(&doc, "```go\nvar x%d = %d\n```\n\n", i, i)
	}
	h := openMarkdown(t, "# Fences\n")
	jobs := countJobs(h)
	h.daemon.Set(func(s *fakeservef.Server) {
		page := s.Pages[project]["docs/guide.md"]
		page.Source = doc.String()
		s.Pages[project]["docs/guide.md"] = page
	})
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: project, Path: "README.md"})
	h.settle()
	h.app.Model.OpenLink(link.Target{Kind: link.Local, Project: project, Path: "docs/guide.md"})
	for range 4 {
		h.settle()
	}
	if got := countText(h, "Code, go"); got != 200 {
		t.Fatalf("%d of 200 fences labeled", got)
	}
	if jobs.jobs != 200 {
		t.Fatalf("%d highlight jobs for 200 fences", jobs.jobs)
	}
	if h.tt.FramePending() {
		t.Fatal("frames still pending after settling")
	}
}

// A style value the drawing cannot use (here an rgb() with no arguments)
// draws the diagram rather than crashing the app.
func TestHostileMermaidStyleDraws(t *testing.T) {
	h := openMarkdown(t, "```mermaid\nflowchart LR\n  A --> B\n  style A fill:rgb,stroke:rgba\n```\n")
	if _, ok := h.tt.Find("Mermaid diagram"); !ok {
		t.Fatalf("no diagram; texts %q", h.tt.Texts())
	}
}

// Resizing renders once the width holds, not once per width; the last
// render shows meanwhile.
func TestMermaidRendersOncePerSettledWidth(t *testing.T) {
	h := openMarkdown(t, "```mermaid\nflowchart LR\n  A --> B\n```\n")
	jobs := countJobs(h)
	for _, width := range []int{1000, 980, 960, 940} {
		h.tt.SetSize(width, 700)
		h.settle()
		if _, ok := h.tt.Find("Mermaid diagram"); !ok {
			t.Fatalf("width %d: the diagram left while resizing", width)
		}
	}
	if jobs.jobs != 0 {
		t.Fatalf("%d renders while resizing", jobs.jobs)
	}
	h.tt.Advance(resizeDelay)
	h.settle()
	h.tt.Advance(resizeDelay)
	h.settle()
	if jobs.jobs != 1 {
		t.Fatalf("%d renders after the width held, want 1", jobs.jobs)
	}
	if _, ok := h.tt.Find("Mermaid diagram"); !ok {
		t.Fatal("no diagram after resizing")
	}
}
