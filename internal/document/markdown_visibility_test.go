package document

import (
	"bytes"
	"html"
	"regexp"
	"strings"
	"testing"
)

func TestMarkdownPreviewHidesLeadingFrontmatter(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{"yaml", "---\ntitle: hidden metadata\n---\n# Visible body\n"},
		{"toml", "+++\ntitle = \"hidden metadata\"\n+++\n# Visible body\n"},
		{"bom-crlf", "\xef\xbb\xbf---\r\ntitle: hidden metadata\r\n---\r\n# Visible body\r\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			original := bytes.Clone(source)
			handler := newMarkdown()
			preview, err := handler.Preview(source, Context{})
			if err != nil || preview != "<h1>Visible body</h1>\n" {
				t.Fatalf("preview = %q, %v", preview, err)
			}
			if !bytes.Equal(source, original) {
				t.Fatal("preview changed source bytes")
			}
		})
	}
}

func TestMarkdownPreviewPreservesMarkdownDelimiters(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		want   string
	}{
		{"unclosed-yaml", "---\ntitle: visible metadata\n\nBody", "visible metadata"},
		{"unclosed-toml", "+++\ntitle = \"visible metadata\"\n\nBody", "visible metadata"},
		{"thematic-break", "Intro\n\n---\n\nBody", "<hr>"},
		{"setext-heading", "Heading\n---\n", "<h2>Heading</h2>"},
		{"later-delimiters", "Intro\n\n---\n\nBody\n\n---", "<p>Body</p>"},
	} {
		t.Run(test.name, func(t *testing.T) {
			preview, err := newMarkdown().Preview([]byte(test.source), Context{})
			if err != nil || !strings.Contains(preview, test.want) {
				t.Fatalf("preview = %q, %v; want %q", preview, err, test.want)
			}
		})
	}
}

func TestMarkdownPreviewHidesCommentsAndPreservesSurroundingText(t *testing.T) {
	source := []byte("<!-- hidden block\nmetadata -->\n\nBefore <!-- hidden inline --> after.\n\n<!-- hidden trailing -->Trailing text\n\n<div>\n<!-- hidden nested -->\nVisible HTML body\n</div>\n")
	original := bytes.Clone(source)
	handler := newMarkdown()
	preview, err := handler.Preview(source, Context{})
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"hidden", "metadata", "&lt;!--"} {
		if strings.Contains(preview, hidden) {
			t.Fatalf("comment %q visible in %q", hidden, preview)
		}
	}
	for _, visible := range []string{"<p>Before  after.</p>", "Trailing text", "Visible HTML body", "&lt;div&gt;", "&lt;/div&gt;"} {
		if !strings.Contains(preview, visible) {
			t.Fatalf("missing %q in %q", visible, preview)
		}
	}
	if !bytes.Equal(source, original) {
		t.Fatal("preview changed source bytes")
	}
}

func TestMarkdownPreviewPreservesCommentLiteralsAndEscapesHTML(t *testing.T) {
	source := []byte("`<!-- inline literal -->`\n\n```text\n<!-- fenced literal -->\n```\n\n    <!-- indented literal -->\n\n<div title=\"<!-- attribute literal -->\">\n<script>alert('unsafe')</script>\n</div>\n")
	preview, err := newMarkdown().Preview(source, Context{})
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{"&lt;!-- inline literal --&gt;", "&lt;!-- fenced literal --&gt;", "&lt;!-- indented literal --&gt;", "&lt;!-- attribute literal --&gt;", "&lt;script&gt;"} {
		if !strings.Contains(preview, visible) {
			t.Fatalf("missing %q in %q", visible, preview)
		}
	}
	if strings.Contains(preview, "<script>") || strings.Contains(preview, "<div title=") {
		t.Fatalf("unsafe HTML rendered: %q", preview)
	}
}

func TestMarkdownHighlightRetainsFrontmatterAndComments(t *testing.T) {
	source := []byte("---\ntitle: metadata\n---\n\nVisible <!-- hidden preview comment -->\n")
	_, err := newMarkdown().Preview(source, Context{})
	if err != nil {
		t.Fatal(err)
	}
	lines := HighlightLines(source, "markdown")
	markup := regexp.MustCompile(`<[^>]*>`)
	text := html.UnescapeString(markup.ReplaceAllString(strings.Join(lines, "\n"), ""))
	if text != strings.TrimSuffix(string(source), "\n") {
		t.Fatalf("highlight lost source: %q", text)
	}
}
