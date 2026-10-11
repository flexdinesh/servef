package web

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/internal/document"
)

func TestFileTypeScopeControlsTreePagesAndSearch(t *testing.T) {
	root := t.TempDir()
	for name, source := range map[string]string{
		"readme.md": "# Hello", "config.JSON": `{"token":"json-search"}`, "nested/app.yml": "token: yaml-search\n", "nested/other.yaml": "second: yes\n", "main.go": "package main", "broken.json": `{"token": broken-search`,
	} {
		writeMarkdown(t, filepath.Join(root, name), source)
	}
	for _, selectors := range [][]string{nil, {"MD"}, {"json", "yml"}} {
		types, err := document.Builtins().Select(selectors)
		if err != nil {
			t.Fatal(err)
		}
		app, err := New(Config{Root: root, Depth: 5, Types: types})
		if err != nil {
			t.Fatal(err)
		}
		_, index := requestPageData(t, app, "/api/page")
		if len(index.FileTypes) != len(types.Types()) {
			t.Fatal("scope metadata differs")
		}
		for _, name := range []string{"readme.md", "config.JSON", "nested/app.yml", "nested/other.yaml", "main.go", "broken.json"} {
			response, page := requestPageData(t, app, "/api/page?path="+name)
			allowed := types.Supports(name)
			if allowed != (response.Code == http.StatusOK) {
				t.Fatalf("%v %s status = %d", selectors, name, response.Code)
			}
			if allowed && (!page.HasFile || page.DocumentType == nil || page.Source == "" || len(page.SourceLines) == 0) {
				t.Fatalf("incomplete document contract: %+v", page)
			}
			if allowed && name == "broken.json" && (page.PreviewError == "" || page.Error != "") {
				t.Fatalf("preview failure blocked source: %+v", page)
			}
		}
		response := request(t, app, "/api/search-documents")
		var search searchDocumentsResponse
		if err := json.Unmarshal(response.Body.Bytes(), &search); err != nil {
			t.Fatal(err)
		}
		if len(search.Documents) != index.FileCount {
			t.Fatalf("search %d differs from discovery %d", len(search.Documents), index.FileCount)
		}
		for _, entry := range search.Documents {
			if !types.Supports(entry.Path) {
				t.Fatalf("disallowed search result: %s", entry.Path)
			}
			if entry.Path == "broken.json" && !strings.Contains(entry.Content, "broken-search") {
				t.Fatal("malformed file disappeared from search")
			}
		}
	}
}

func TestSourceOnlyHandlerUsesSameAPIContracts(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "main.go"), "package main\n")
	types, err := document.NewRegistry(document.CodeHandler{Definition: document.Type{ID: "go", Label: "Go", Extensions: []string{".go"}, Language: "go", DefaultMode: "source"}})
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{Root: root, Depth: 0, Types: types})
	if err != nil {
		t.Fatal(err)
	}
	response, page := requestPageData(t, app, "/api/page?path=main.go")
	if response.Code != http.StatusOK || page.DocumentType == nil || page.DocumentType.DefaultMode != "source" || page.DocumentType.PreviewKind != "" || page.Content != "" || len(page.SourceLines) != 1 {
		t.Fatalf("source-only page = %+v", page)
	}
	if page.Tree[0].FileType != "go" {
		t.Fatalf("tree family = %s", page.Tree[0].FileType)
	}
}
