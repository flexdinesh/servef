package web

import (
	"encoding/json"
	"html"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAppMarkdownVisibilityPreservesSourceAndSearch(t *testing.T) {
	root := t.TempDir()
	source := "---\r\ntitle: metadata-search-token\r\n---\r\n\r\n# Visible body\r\n\r\nBefore <!-- comment-search-token --> after.\r\n"
	writeMarkdown(t, filepath.Join(root, "visibility.md"), source)
	app := newTestApp(t, root)
	response, page := requestPageData(t, app, "/api/page?project=.&path=visibility.md")
	if response.Code != http.StatusOK {
		t.Fatalf("page status = %d", response.Code)
	}
	if page.Source != source {
		t.Fatalf("source changed: %q", page.Source)
	}
	assertContains(t, page.Content, "<h1>Visible body</h1>", "<p>Before  after.</p>")
	for _, hidden := range []string{"metadata-search-token", "comment-search-token"} {
		if strings.Contains(page.Content, hidden) {
			t.Fatalf("preview includes %q: %q", hidden, page.Content)
		}
	}
	markup := regexp.MustCompile(`<[^>]*>`)
	highlighted := html.UnescapeString(markup.ReplaceAllString(strings.Join(page.SourceLines, "\n"), ""))
	if highlighted != strings.TrimSuffix(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
		t.Fatalf("highlight changed source: %q", highlighted)
	}
	search := request(t, app, "/api/search-documents?project=.")
	if search.Code != http.StatusOK {
		t.Fatalf("search status = %d", search.Code)
	}
	var results searchDocumentsResponse
	if err := json.Unmarshal(search.Body.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if len(results.Documents) != 1 {
		t.Fatalf("search documents = %d", len(results.Documents))
	}
	assertContains(t, results.Documents[0].Content, "metadata-search-token", "comment-search-token")
}
