package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
)

func newScopedApp(t *testing.T, root string, patterns ...string) *App {
	t.Helper()
	filter, err := files.NewNameFilter(patterns)
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{Roots: []string{root}, Depth: 5, Filenames: filter})
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func searchDocuments(t *testing.T, app *App) searchDocumentsResponse {
	t.Helper()
	response := request(t, app, "/api/search-documents?project=.")
	if response.Code != http.StatusOK {
		t.Fatalf("search status = %d", response.Code)
	}
	var search searchDocumentsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &search); err != nil {
		t.Fatal(err)
	}
	return search
}

func TestFilenameScopeControlsTreePagesAndSearch(t *testing.T) {
	root := t.TempDir()
	names := []string{"readme.md", "config.JSON", "nested/app.yml", "main.go", "Dockerfile", "broken.json", "notes"}
	for _, name := range names {
		writeMarkdown(t, filepath.Join(root, name), "token: search-"+strings.ReplaceAll(name, "/", "-")+"\n")
	}
	for _, patterns := range [][]string{{"*"}, {"*.MD"}, {"*.json", "*.yml"}, {"dockerfile", "*.go"}} {
		filter, err := files.NewNameFilter(patterns)
		if err != nil {
			t.Fatal(err)
		}
		app := newScopedApp(t, root, patterns...)
		_, index := requestPageData(t, app, "/api/page?project=.")
		if !reflect.DeepEqual(index.FilenamePatterns, patterns) {
			t.Fatalf("filenamePatterns = %v, want %v", index.FilenamePatterns, patterns)
		}
		for _, name := range names {
			response, page := requestPageData(t, app, "/api/page?project=.&path="+name)
			allowed := filter.Match(name)
			if allowed != (response.Code == http.StatusOK) {
				t.Fatalf("%v %s status = %d", patterns, name, response.Code)
			}
			if allowed && (!page.HasFile || page.DocumentType == nil || page.Source == "" || len(page.SourceLines) == 0) {
				t.Fatalf("incomplete document contract: %+v", page)
			}
		}
		search := searchDocuments(t, app)
		if len(search.Documents) != index.FileCount {
			t.Fatalf("search %d differs from discovery %d", len(search.Documents), index.FileCount)
		}
		for _, entry := range search.Documents {
			if !filter.Match(entry.Path) {
				t.Fatalf("out-of-scope search result: %s", entry.Path)
			}
		}
	}
}

func TestPageContractShape(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "config.json"), `{"a":1}`)
	response := request(t, newTestApp(t, root), "/api/page?project=.&path=config.json")
	var contract map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"filenamePatterns", "binary", "previewDisabled", "searchExcluded", "documentType", "navigationSupported"} {
		if _, ok := contract[key]; !ok {
			t.Errorf("missing %q", key)
		}
	}
	if _, ok := contract["fileTypes"]; ok {
		t.Error("legacy fileTypes present")
	}
	var fileType map[string]json.RawMessage
	if err := json.Unmarshal(contract["documentType"], &fileType); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"id": `"json"`, "label": `"JSON"`, "extensions": `[".json"]`, "filenames": `[]`, "language": `"json"`, "previewKind": `""`, "previewLabel": `""`, "defaultMode": `"source"`, "formattable": `true`}
	if len(fileType) != len(want) {
		t.Fatalf("documentType = %v", fileType)
	}
	for key, value := range want {
		if string(fileType[key]) != value {
			t.Errorf("documentType.%s = %s, want %s", key, fileType[key], value)
		}
	}
	if string(contract["filenamePatterns"]) != `["*"]` {
		t.Errorf("filenamePatterns = %s", contract["filenamePatterns"])
	}
}

func TestAnyFileOpensWithAFamily(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "Dockerfile"), "FROM scratch\n")
	writeMarkdown(t, filepath.Join(root, "LICENSE"), "<b>plain</b>\n")
	writeMarkdown(t, filepath.Join(root, "broken.json"), `{"token": broken-search`)
	app := newTestApp(t, root)
	for name, want := range map[string]string{"Dockerfile": "dockerfile", "LICENSE": "text", "broken.json": "json"} {
		response, page := requestPageData(t, app, "/api/page?project=.&path="+name)
		if response.Code != http.StatusOK || page.DocumentType == nil || page.DocumentType.ID != want {
			t.Fatalf("%s = %d %+v", name, response.Code, page.DocumentType)
		}
		if page.Content != "" || page.PreviewError != "" || page.Binary || page.PreviewDisabled || page.SearchExcluded {
			t.Fatalf("%s eager preview or flags: %+v", name, page)
		}
	}
	_, page := requestPageData(t, app, "/api/page?project=.&path=LICENSE")
	if page.DocumentType.Label != "Plain text" || !reflect.DeepEqual(page.SourceLines, []string{"&lt;b&gt;plain&lt;/b&gt;"}) {
		t.Fatalf("fallback page = %+v", page)
	}
	types := map[string]string{}
	for _, node := range page.Tree {
		types[node.Name] = node.FileType
	}
	if !reflect.DeepEqual(types, map[string]string{"Dockerfile": "dockerfile", "LICENSE": "text", "broken.json": "json"}) {
		t.Fatalf("tree families = %v", types)
	}
	found := false
	for _, entry := range searchDocuments(t, app).Documents {
		found = found || (entry.Path == "broken.json" && strings.Contains(entry.Content, "broken-search"))
	}
	if !found {
		t.Fatal("malformed file disappeared from search")
	}
}

func TestBinaryFilesOpenWithoutSourceAndSkipSearch(t *testing.T) {
	root := t.TempDir()
	png := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), bytes.Repeat([]byte{1}, 32)...)
	if err := os.WriteFile(filepath.Join(root, "image.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.bin"), []byte("head\x00tail"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeMarkdown(t, filepath.Join(root, "text.md"), "# text")
	app := newTestApp(t, root)
	for name, want := range map[string]string{"image.png": "image", "config.bin": "text"} {
		response, page := requestPageData(t, app, "/api/page?project=.&path="+name)
		if response.Code != http.StatusOK || !page.HasFile || !page.Binary || !page.SearchExcluded || page.PreviewDisabled {
			t.Fatalf("%s = %d %+v", name, response.Code, page)
		}
		if page.Source != "" || len(page.SourceLines) != 0 || page.Content != "" || page.DocumentType == nil || page.DocumentType.ID != want {
			t.Fatalf("%s binary contract = %+v", name, page)
		}
		if size, _ := os.Stat(filepath.Join(root, name)); page.FileSize != size.Size() {
			t.Fatalf("%s size = %d", name, page.FileSize)
		}
	}
	search := searchDocuments(t, app)
	if len(search.Documents) != 1 || search.Documents[0].Path != "text.md" {
		t.Fatalf("search = %+v", search.Documents)
	}
}

func TestRasterImagesAreBinaryWithoutNULBytes(t *testing.T) {
	root := t.TempDir()
	gif := []byte("GIF89a\x01\x01\x01\x01\x80\x01\x01gif-search-token;")
	if err := os.WriteFile(filepath.Join(root, "pixel.gif"), gif, 0o644); err != nil {
		t.Fatal(err)
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><text>svg-search-token</text></svg>`
	writeMarkdown(t, filepath.Join(root, "icon.svg"), svg)
	app := newTestApp(t, root)

	_, page := requestPageData(t, app, "/api/page?project=.&path=pixel.gif")
	if !page.HasFile || !page.Binary || !page.SearchExcluded || page.Source != "" || len(page.SourceLines) != 0 || page.FileSize != int64(len(gif)) {
		t.Fatalf("gif page = %+v", page)
	}
	_, page = requestPageData(t, app, "/api/page?project=.&path=icon.svg")
	if page.Binary || page.SearchExcluded || page.Source != svg {
		t.Fatalf("svg page = %+v", page)
	}
	paths := []string{}
	for _, entry := range searchDocuments(t, app).Documents {
		paths = append(paths, entry.Path)
	}
	if !reflect.DeepEqual(paths, []string{"icon.svg"}) {
		t.Fatalf("search paths = %v", paths)
	}
}

func TestLargeFilesDisablePreviewThenSearch(t *testing.T) {
	root := t.TempDir()
	large := "# <b>large</b> large-search-token\n" + strings.Repeat("x", document.PreviewLimit)
	huge := "# huge huge-search-token\n" + strings.Repeat("x", document.SearchLimit)
	writeMarkdown(t, filepath.Join(root, "large.md"), large)
	writeMarkdown(t, filepath.Join(root, "huge.md"), huge)
	writeMarkdown(t, filepath.Join(root, "large.json"), `{"a":"`+strings.Repeat("x", document.PreviewLimit)+`"}`)
	app := newTestApp(t, root)

	_, page := requestPageData(t, app, "/api/page?project=.&path=large.md")
	if !page.PreviewDisabled || page.SearchExcluded || page.Content != "" || page.Source != large {
		t.Fatalf("large page flags = %v %v content %d", page.PreviewDisabled, page.SearchExcluded, len(page.Content))
	}
	if len(page.SourceLines) != 2 || page.SourceLines[0] != "# &lt;b&gt;large&lt;/b&gt; large-search-token" {
		t.Fatalf("large source not escaped plain text: %q", page.SourceLines[0])
	}
	_, page = requestPageData(t, app, "/api/page?project=.&path=huge.md")
	if !page.PreviewDisabled || !page.SearchExcluded || page.Source != huge || page.Content != "" {
		t.Fatalf("huge page flags = %v %v", page.PreviewDisabled, page.SearchExcluded)
	}
	paths := []string{}
	for _, entry := range searchDocuments(t, app).Documents {
		paths = append(paths, entry.Path)
	}
	if !reflect.DeepEqual(paths, []string{"large.json", "large.md"}) {
		t.Fatalf("search paths = %v", paths)
	}
}

func TestFilesOverDisplayLimitAreNotRead(t *testing.T) {
	root := t.TempDir()
	// Sparse files: the NUL tail would read as binary if content were sniffed.
	sparse := func(name string, size int64) {
		t.Helper()
		writeMarkdown(t, filepath.Join(root, name), "text-token\n")
		if err := os.Truncate(filepath.Join(root, name), size); err != nil {
			t.Fatal(err)
		}
	}
	sparse("huge.log", document.DisplayLimit+1)
	sparse("huge.json", document.DisplayLimit+1)
	sparse("limit.log", document.DisplayLimit)
	sparse("huge.png", document.DisplayLimit+1)
	sparse("huge.svg", document.DisplayLimit+1)
	app := newTestApp(t, root)

	for _, name := range []string{"huge.log", "huge.json"} {
		response, page := requestPageData(t, app, "/api/page?project=.&path="+name)
		if response.Code != http.StatusOK || !page.HasFile || !page.TooLarge || page.Binary || !page.PreviewDisabled || !page.SearchExcluded {
			t.Fatalf("%s = %d %+v", name, response.Code, page)
		}
		if page.Source != "" || len(page.SourceLines) != 0 || page.Content != "" || page.FileSize != document.DisplayLimit+1 {
			t.Fatalf("%s sent content: source %d lines %d content %d", name, len(page.Source), len(page.SourceLines), len(page.Content))
		}
		if page.DocumentType == nil || page.DocumentType.ID != app.config.Types.Lookup(name).Type().ID {
			t.Fatalf("%s documentType = %+v", name, page.DocumentType)
		}
	}
	for _, name := range []string{"limit.log", "huge.png", "huge.svg"} {
		if _, page := requestPageData(t, app, "/api/page?project=.&path="+name); page.TooLarge {
			t.Fatalf("%s marked too large", name)
		}
	}
	if response := request(t, app, "/api/format?project=.&path=huge.json"); response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("huge format = %d", response.Code)
	}
	for _, entry := range searchDocuments(t, app).Documents {
		if strings.HasPrefix(entry.Path, "huge.") {
			t.Fatalf("search includes %s", entry.Path)
		}
	}
}

func TestLargeImagesKeepPreview(t *testing.T) {
	root := t.TempDir()
	png := append([]byte("\x89PNG\r\n\x1a\n\x00"), bytes.Repeat([]byte{1}, document.PreviewLimit)...)
	if err := os.WriteFile(filepath.Join(root, "photo.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><desc>` + strings.Repeat("x", document.PreviewLimit) + "</desc></svg>\n"
	writeMarkdown(t, filepath.Join(root, "large.svg"), svg)
	app := newTestApp(t, root)

	_, page := requestPageData(t, app, "/api/page?project=.&path=photo.png")
	if page.PreviewDisabled || !page.Binary || page.DocumentType == nil || page.DocumentType.PreviewKind != document.PreviewImage {
		t.Fatalf("large raster page = %+v", page)
	}
	_, page = requestPageData(t, app, "/api/page?project=.&path=large.svg")
	if page.PreviewDisabled || page.Binary || page.Source != svg || page.Content != "" {
		t.Fatalf("large SVG flags = previewDisabled %v binary %v", page.PreviewDisabled, page.Binary)
	}
	if len(page.SourceLines) != 1 || strings.Contains(page.SourceLines[0], "syntax-") || !strings.HasPrefix(page.SourceLines[0], "&lt;svg") {
		t.Fatal("large SVG source not escaped plain text")
	}
	for _, name := range []string{"photo.png", "large.svg"} {
		if response := request(t, app, "/api/raw?project=.&path="+name); response.Code != http.StatusOK || response.Body.Len() < document.PreviewLimit {
			t.Fatalf("%s raw = %d (%d bytes)", name, response.Code, response.Body.Len())
		}
	}
}

func TestFormatEndpoint(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "config.json"), `{"b":900719925474099312345,"html":"<script>"}`)
	writeMarkdown(t, filepath.Join(root, "config.yml"), "a:   1\n")
	writeMarkdown(t, filepath.Join(root, "broken.json"), `{"token": broken`)
	writeMarkdown(t, filepath.Join(root, "main.go"), "package main\n")
	writeMarkdown(t, filepath.Join(root, "page.html"), "<div><p>web-format</p></div>")
	writeMarkdown(t, filepath.Join(root, "site.css"), "a{color:red}")
	writeMarkdown(t, filepath.Join(root, "app.js"), "if(a){b()}")
	writeMarkdown(t, filepath.Join(root, "view.js"), "const el = <div>jsx</div>")
	writeMarkdown(t, filepath.Join(root, "large.json"), `{"a":"`+strings.Repeat("x", document.PreviewLimit)+`"}`)
	if err := os.WriteFile(filepath.Join(root, "binary.json"), []byte("{\x00}"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := newScopedApp(t, root, "*.json", "*.yml", "*.go", "*.html", "*.css", "*.js")
	writeMarkdown(t, filepath.Join(root, "outside.md"), "# outside")

	response := request(t, app, "/api/format?project=.&path=config.json")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("format status = %d", response.Code)
	}
	var formatted formatResponse
	if err := json.Unmarshal(response.Body.Bytes(), &formatted); err != nil {
		t.Fatal(err)
	}
	if len(formatted.FormattedLines) != 4 || !strings.Contains(formatted.FormattedLines[1], "syntax-") || !strings.Contains(formatted.FormattedLines[1], "900719925474099312345") {
		t.Fatalf("formatted lines = %#v", formatted.FormattedLines)
	}
	if strings.Contains(response.Body.String(), "<script>") {
		t.Fatal("formatted output not escaped")
	}
	if response := request(t, app, "/api/format?project=.&path=config.yml"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "formattedLines") {
		t.Fatalf("YAML format = %d %s", response.Code, response.Body.String())
	}
	for path, lines := range map[string]int{"page.html": 3, "site.css": 3, "app.js": 3} {
		response := request(t, app, "/api/format?project=.&path="+path)
		var formatted formatResponse
		if err := json.Unmarshal(response.Body.Bytes(), &formatted); err != nil || response.Code != http.StatusOK || len(formatted.FormattedLines) != lines {
			t.Errorf("%s format = %d %s", path, response.Code, response.Body.String())
		}
	}
	for target, status := range map[string]int{
		"/api/format?project=.&path=broken.json":   http.StatusUnprocessableEntity,
		"/api/format?project=.&path=view.js":       http.StatusUnprocessableEntity,
		"/api/format?project=.&path=binary.json":   http.StatusUnprocessableEntity,
		"/api/format?project=.&path=main.go":       http.StatusBadRequest,
		"/api/format?project=.&path=large.json":    http.StatusRequestEntityTooLarge,
		"/api/format?project=.&path=missing.json":  http.StatusNotFound,
		"/api/format?project=.&path=outside.md":    http.StatusNotFound,
		"/api/format?project=.&path=..%2Fetc.json": http.StatusNotFound,
		"/api/format?project=.":                    http.StatusNotFound,
	} {
		response := request(t, app, target)
		var payload errorResponse
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if response.Code != status || payload.Error == "" {
			t.Errorf("%s = %d %q, want %d", target, response.Code, payload.Error, status)
		}
	}
}

func TestRawEndpointServesOnlyImagePreviews(t *testing.T) {
	root := t.TempDir()
	png := []byte("\x89PNG\r\n\x1a\n\x00exact")
	if err := os.WriteFile(filepath.Join(root, "image.PNG"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`
	writeMarkdown(t, filepath.Join(root, "icon.svg"), svg)
	writeMarkdown(t, filepath.Join(root, "main.go"), "package main\n")
	writeMarkdown(t, filepath.Join(root, "README.md"), "# readme\n")
	writeMarkdown(t, filepath.Join(root, "hidden", "secret.png"), "secret")
	app := newScopedApp(t, root, "*.png", "*.svg", "*.go", "README.md")

	for name, want := range map[string]struct {
		contentType string
		body        []byte
	}{
		"image.PNG": {contentType: "image/png", body: png},
		"icon.svg":  {contentType: "image/svg+xml", body: []byte(svg)},
	} {
		response := request(t, app, "/api/raw?project=.&path="+name)
		if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), want.body) {
			t.Fatalf("%s = %d %q", name, response.Code, response.Body.Bytes())
		}
		for header, value := range map[string]string{
			"Content-Type":            want.contentType,
			"X-Content-Type-Options":  "nosniff",
			"Content-Security-Policy": "sandbox; default-src 'none'; style-src 'unsafe-inline'",
		} {
			if got := response.Header().Get(header); got != value {
				t.Errorf("%s %s = %q, want %q", name, header, got, value)
			}
		}
	}
	for target, status := range map[string]int{
		"/api/raw?project=.&path=main.go":          http.StatusUnsupportedMediaType,
		"/api/raw?project=.&path=README.md":        http.StatusUnsupportedMediaType,
		"/api/raw?project=.&path=missing.png":      http.StatusNotFound,
		"/api/raw?project=.&path=..%2Fsecret.png":  http.StatusNotFound,
		"/api/raw?project=.":                       http.StatusNotFound,
		"/api/raw?project=.&path=hidden%2Fmissing": http.StatusNotFound,
	} {
		if response := request(t, app, target); response.Code != status {
			t.Errorf("%s = %d, want %d", target, response.Code, status)
		}
	}
	scoped := newScopedApp(t, root, "*.svg")
	if response := request(t, scoped, "/api/raw?project=.&path=image.PNG"); response.Code != http.StatusNotFound {
		t.Fatalf("out-of-scope raw = %d", response.Code)
	}
}

func TestMarkdownLinksTargetInScopeFiles(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "README.md"), "[code](src/main.go) [conf](Dockerfile)")
	writeMarkdown(t, filepath.Join(root, "src", "main.go"), "package main\n")
	for patterns, want := range map[string]bool{"*": true, "*.md": false} {
		_, page := requestPageData(t, newScopedApp(t, root, strings.Split(patterns, ",")...), "/api/page?project=.&path=README.md")
		if strings.Contains(page.Content, `href="/view?project=.&amp;path=src%2Fmain.go"`) != want || strings.Contains(page.Content, `href="/view?project=.&amp;path=Dockerfile"`) != want {
			t.Fatalf("%s links = %s", patterns, page.Content)
		}
	}
}

func TestSourceOnlyHandlerUsesSameAPIContracts(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "main.zig"), "const std = @import(\"std\");\n")
	writeMarkdown(t, filepath.Join(root, "other.go"), "package main\n")
	types, err := document.NewRegistry(
		document.CodeHandler{Definition: document.Type{ID: "text", Label: "Plain text", Language: "plaintext", DefaultMode: "source"}},
		document.CodeHandler{Definition: document.Type{ID: "zig", Label: "Zig", Extensions: []string{".zig"}, Language: "zig", DefaultMode: "source"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{Roots: []string{root}, Depth: 0, Types: types})
	if err != nil {
		t.Fatal(err)
	}
	response, page := requestPageData(t, app, "/api/page?project=.&path=main.zig")
	if response.Code != http.StatusOK || page.DocumentType == nil || page.DocumentType.ID != "zig" || page.DocumentType.DefaultMode != "source" || page.DocumentType.PreviewKind != "" || page.DocumentType.Formattable || page.Content != "" || len(page.SourceLines) != 1 || !strings.Contains(page.SourceLines[0], "syntax-") {
		t.Fatalf("source-only page = %+v", page)
	}
	if page.Tree[0].FileType != "zig" || page.Tree[1].FileType != "text" {
		t.Fatalf("tree families = %+v", page.Tree)
	}
	if response := request(t, app, "/api/format?project=.&path=main.zig"); response.Code != http.StatusBadRequest {
		t.Fatalf("source-only format = %d", response.Code)
	}
	found := false
	for _, entry := range searchDocuments(t, app).Documents {
		found = found || entry.Path == "main.zig"
	}
	if !found {
		t.Fatal("source-only file missing from search")
	}
}

// Frontend API fixtures must describe documents the way the real server does.
func TestAPIFixturesMatchServerDocumentContract(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "markdown")
	app := newTestApp(t, root)
	names, err := filepath.Glob(filepath.Join("..", "..", "testdata", "api", "default", "page-*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("fixtures = %v, %v", names, err)
	}
	for _, name := range names {
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var fixture pageData
		if err := json.Unmarshal(content, &fixture); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if fixture.DocumentType == nil {
			continue
		}
		if want := app.config.Types.Lookup(fixture.Selected).Type(); !reflect.DeepEqual(*fixture.DocumentType, want) {
			t.Errorf("%s documentType = %+v, want %+v", name, *fixture.DocumentType, want)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(fixture.Selected))); err != nil {
			continue
		}
		_, served := requestPageData(t, app, "/api/page?project=.&path="+fixture.Selected)
		if fixture.Binary != served.Binary || fixture.TooLarge != served.TooLarge || fixture.SearchExcluded != served.SearchExcluded || fixture.PreviewDisabled != served.PreviewDisabled {
			t.Errorf("%s flags = binary %v tooLarge %v searchExcluded %v previewDisabled %v, server = %v %v %v %v", name,
				fixture.Binary, fixture.TooLarge, fixture.SearchExcluded, fixture.PreviewDisabled, served.Binary, served.TooLarge, served.SearchExcluded, served.PreviewDisabled)
		}
	}
}
