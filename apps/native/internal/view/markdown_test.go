package view

import (
	"testing"

	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/apps/native/internal/link"
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
