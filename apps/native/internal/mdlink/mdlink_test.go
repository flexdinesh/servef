package mdlink

import (
	"strings"
	"testing"

	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/pkg/contract"
)

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		doc, dest string
		want      Link
	}{
		{"docs/guide.md", "install.md", Link{Kind: Document, Path: "docs/install.md"}},
		{"docs/guide.md", "../README.md#usage", Link{Kind: Document, Path: "README.md", Fragment: "usage"}},
		{"docs/guide.md", "/src/main.go?plain=1", Link{Kind: Document, Path: "src/main.go"}},
		{"README.md", "./a/./b.md", Link{Kind: Document, Path: "a/b.md"}},
		{"README.md", "#install", Link{Kind: Anchor, Fragment: "install"}},
		{"README.md", "https://example.com/x?y=1", Link{Kind: External, URL: "https://example.com/x?y=1"}},
		{"README.md", "MAILTO:dev@example.com", Link{Kind: External, URL: "mailto:dev@example.com"}},
		{"README.md", "javascript:alert(1)", Link{}},
		{"README.md", "file:///etc/passwd", Link{}},
		{"README.md", "//evil.example/x", Link{}},
		{"README.md", "docs/", Link{}},
		{"README.md", "../outside.md", Link{}},
		{"docs/a.md", "../../outside.md", Link{}},
		{"README.md", "", Link{}},
		{"README.md", "%zz", Link{}},
	} {
		if got := Resolve(tc.doc, tc.dest); got != tc.want {
			t.Errorf("Resolve(%q, %q) = %+v, want %+v", tc.doc, tc.dest, got, tc.want)
		}
	}
}

func TestImage(t *testing.T) {
	for _, tc := range []struct {
		doc, dest, want string
		ok              bool
	}{
		{"docs/guide.md", "img/a.png", "docs/img/a.png", true},
		{"docs/guide.md", "/assets/logo.svg#dark", "assets/logo.svg", true},
		{"docs/guide.md", "../../x.png", "", false},
		{"README.md", "https://example.com/a.png", "", false},
		{"README.md", "data:image/png;base64,AAAA", "", false},
		{"README.md", "", "", false},
	} {
		got, ok := Image(tc.doc, tc.dest)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Image(%q, %q) = %q, %v; want %q, %v", tc.doc, tc.dest, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPreviewSource(t *testing.T) {
	for source, want := range map[string]string{
		"---\ntitle: x\n---\n# Hi\n":        "# Hi\n",
		"\xef\xbb\xbf+++\na = 1\n+++\nbody": "body",
		"---\nunclosed\n# Hi":               "---\nunclosed\n# Hi",
		"# Hi\n---\n":                       "# Hi\n---\n",
		"---\r\nk: v\r\n---\r\nbody":        "body",
	} {
		if got := PreviewSource(source); got != want {
			t.Errorf("PreviewSource(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestPreview(t *testing.T) {
	// The server declares Mermaid files as their own family, previewed as
	// one fence (page-mermaid.json matches the server's registry).
	var page contract.PageData
	fakeservef.Fixture(t, "page-mermaid.json", &page)
	if page.DocumentType == nil || page.DocumentType.ID != MermaidFamily || page.DocumentType.PreviewKind != contract.PreviewMarkdown {
		t.Fatalf("page-mermaid.json declares %+v", page.DocumentType)
	}
	if want := "<pre><code class=\"language-mermaid\">"; !strings.HasPrefix(page.Content, want) {
		t.Fatalf("the server's Mermaid preview is no longer one mermaid fence: %q", page.Content)
	}
	got := Preview(page.Source, page.DocumentType)
	if want := "```mermaid\n" + page.Source + "```\n"; got != want {
		t.Fatalf("mermaid page preview:\n%q\nwant\n%q", got, want)
	}
	mermaid := &contract.FileType{ID: MermaidFamily}
	if got := Preview("a ``` b", mermaid); got != "````mermaid\na ``` b\n````\n" {
		t.Fatalf("fence around backticks: %q", got)
	}
	// Every other family shows its Markdown source, whatever its preview
	// looks like.
	markdown := &contract.FileType{ID: "markdown"}
	if got := Preview("---\nx: 1\n---\n# Hi\n", markdown); got != "# Hi\n" {
		t.Fatalf("markdown preview %q", got)
	}
	if got := Preview("```mermaid\nA-->B\n```\n", markdown); got != "```mermaid\nA-->B\n```\n" {
		t.Fatalf("a Markdown file of one mermaid fence %q", got)
	}
	if got := Preview("x", nil); got != "x" {
		t.Fatalf("no type %q", got)
	}
}
