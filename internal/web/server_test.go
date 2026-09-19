package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/features"
)

func TestAppServesSharedMarkdownFixtures(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "markdown")
	app := newTestApp(t, root)

	landing, landingData := requestPageData(t, app, "/api/page")
	if landing.Code != http.StatusOK {
		t.Fatalf("landing status = %d, want %d", landing.Code, http.StatusOK)
	}
	if got, want := landingData.RootName, "markdown"; got != want {
		t.Fatalf("root name = %q, want %q", got, want)
	}
	if got, want := landingData.FileCount, 6; got != want {
		t.Fatalf("file count = %d, want %d", got, want)
	}
	assertContains(t, landing.Body.String(), "README.md", "code-blocks.md", "getting-started.md", "api.markdown", "search.md")
	for _, excluded := range []string{"notes.txt", "vendor", "ignored.md"} {
		if strings.Contains(landing.Body.String(), excluded) {
			t.Errorf("landing includes excluded fixture %q", excluded)
		}
	}

	guide, guideData := requestPageData(t, app, "/api/page?path=guides%2Fgetting-started.md")
	if guide.Code != http.StatusOK {
		t.Fatalf("guide status = %d, want %d", guide.Code, http.StatusOK)
	}
	assertContains(t, guideData.Content,
		"<h1>Getting started</h1>",
		"<table>",
		`<pre><code class="language-sh syntax-highlight">servef testdata/markdown`,
		`href="/view?path=reference%2Ftopics%2Fsearch.md"`,
	)
	if guideData.FileSize == 0 {
		t.Fatal("guide file size is zero")
	}

	codeBlocks, codeBlocksData := requestPageData(t, app, "/api/page?path=guides%2Fcode-blocks.md")
	if codeBlocks.Code != http.StatusOK {
		t.Fatalf("code blocks status = %d, want %d", codeBlocks.Code, http.StatusOK)
	}
	assertContains(t, codeBlocksData.Content,
		`<pre><code class="language-json syntax-highlight">`,
		`<pre><code class="language-bash syntax-highlight">`,
		`<pre><code class="language-shell syntax-highlight">`,
		`<pre><code class="language-custom-format">unknown &lt;syntax&gt; remains safely escaped`,
		`<pre><code>plain text still supports copying`,
	)

	diagrams, diagramsData := requestPageData(t, app, "/api/page?path=guides%2Fdiagrams.md")
	if diagrams.Code != http.StatusOK {
		t.Fatalf("diagrams status = %d, want %d", diagrams.Code, http.StatusOK)
	}
	assertContains(t, diagramsData.Content,
		`<pre><code class="language-mermaid">flowchart LR`,
		`GoAPI[&quot;Go API &amp; renderer&quot;]`,
		`<pre><code class="language-mermaid">sequenceDiagram`,
		`User-&gt;&gt;App: Open nested document`,
		`<pre><code class="language-mermaid">stateDiagram-v2`,
		`Excalidraw --&gt; Tldraw: feature enabled`,
		`<pre><code class="language-mermaid">gitGraph`,
		`commit id: &quot;feature&quot;`,
	)
	if strings.Contains(diagramsData.Content, `GoAPI["Go API & renderer"]`) {
		t.Fatal("Mermaid source was not HTML-escaped")
	}

	search := request(t, app, "/api/search-documents")
	if search.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d", search.Code, http.StatusOK)
	}
	var searchData searchDocumentsResponse
	if err := json.Unmarshal(search.Body.Bytes(), &searchData); err != nil {
		t.Fatal(err)
	}
	if got, want := len(searchData.Documents), 6; got != want {
		t.Fatalf("search documents = %d, want %d", got, want)
	}
	foundSearchFixture := false
	for _, document := range searchData.Documents {
		if document.Path == "reference/topics/search.md" {
			foundSearchFixture = true
			assertContains(t, document.Content, "luminous-orchid")
		}
	}
	if !foundSearchFixture {
		t.Fatal("nested search fixture missing from search documents")
	}
}

func TestAppServesProcessMetrics(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	app.metrics = metricsCollector{
		readRSS: func() (uint64, error) {
			return 0, errors.New("RSS unavailable")
		},
		readCPUTime: func() (time.Duration, error) {
			return 0, errors.New("CPU unavailable")
		},
		readGoMemory: func() uint64 {
			return 456
		},
	}
	response := request(t, app, "/api/metrics")
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d", response.Code, http.StatusOK)
	}
	var contract map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	cpuUsage, ok := contract["cpuUsage"]
	if !ok || string(cpuUsage) != "null" {
		t.Fatalf("cpuUsage = %s, want null", cpuUsage)
	}
	for _, legacy := range []string{"rssBytes", "supported"} {
		if _, ok := contract[legacy]; ok {
			t.Fatalf("legacy field %q present", legacy)
		}
	}
	var metrics struct {
		MemoryBytes  uint64   `json:"memoryBytes"`
		MemorySource string   `json:"memorySource"`
		CPUUsage     *float64 `json:"cpuUsage"`
		Goroutines   int      `json:"goroutines"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.MemoryBytes != 456 || metrics.MemorySource != "go" {
		t.Fatalf("memory = %d (%q), want 456 (go)", metrics.MemoryBytes, metrics.MemorySource)
	}
	if metrics.CPUUsage != nil {
		t.Fatalf("CPU usage = %v, want null", *metrics.CPUUsage)
	}
	if metrics.Goroutines < 1 {
		t.Fatalf("goroutines = %d, want positive", metrics.Goroutines)
	}
}

func TestAppListsAndRendersMarkdown(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "docs", "Guide.MD"), "# Guide\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n<script>alert('no')</script>\n")
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	app := newTestApp(t, root)
	landing, landingData := requestPageData(t, app, "/api/page")
	if landing.Code != http.StatusOK {
		t.Fatalf("landing status = %d, want %d", landing.Code, http.StatusOK)
	}
	assertContains(t, landing.Body.String(), "docs", "Guide.MD")
	if strings.Contains(landing.Body.String(), ">empty<") {
		t.Fatal("landing includes a directory with no Markdown descendants")
	}
	if landingData.Empty || landingData.HasFile || len(landingData.Tree) != 1 {
		t.Fatalf("landing data = %#v", landingData)
	}

	document, documentData := requestPageData(t, app, "/api/page?path=docs%2FGuide.MD")
	if document.Code != http.StatusOK {
		t.Fatalf("document status = %d, want %d", document.Code, http.StatusOK)
	}
	body := documentData.Content
	assertContains(t, body, "<h1>Guide</h1>", "<table>", "&lt;script&gt;alert('no')&lt;/script&gt;")
	if got, want := documentData.Source, "# Guide\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n<script>alert('no')</script>\n"; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
	if strings.Contains(body, "<script>alert") {
		t.Fatal("raw Markdown HTML was rendered unsafely")
	}
	if got := document.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	shell := request(t, app, "/view?path=docs%2FGuide.MD")
	if got := shell.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("Content-Security-Policy is empty")
	}
	assertContains(t, shell.Body.String(), `id="root"`, `id="app-data"`, `/assets/`)
	if strings.Contains(shell.Body.String(), "{{APP_DATA}}") || strings.Contains(shell.Body.String(), "<script>alert") {
		t.Fatal("shell contains unexpanded or unsafe app data")
	}
}

func TestAppServesAndBootstrapsFeatures(t *testing.T) {
	set, err := features.Parse([]string{features.MermaidTldraw})
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{Root: t.TempDir(), Depth: 5, Features: set})
	if err != nil {
		t.Fatal(err)
	}

	response := request(t, app, "/api/features")
	if response.Code != http.StatusOK {
		t.Fatalf("features status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var endpoint features.Data
	if err := json.Unmarshal(response.Body.Bytes(), &endpoint); err != nil {
		t.Fatal(err)
	}
	if !endpoint.MermaidTldraw {
		t.Fatalf("features response = %#v", endpoint)
	}

	shell := request(t, app, "/")
	match := regexp.MustCompile(`<template id="feature-data">([^<]+)</template>`).FindStringSubmatch(shell.Body.String())
	if len(match) != 2 {
		t.Fatalf("feature bootstrap missing: %s", shell.Body.String())
	}
	var bootstrap features.Data
	if err := json.Unmarshal([]byte(match[1]), &bootstrap); err != nil {
		t.Fatal(err)
	}
	if bootstrap != endpoint {
		t.Fatalf("bootstrap = %#v, endpoint = %#v", bootstrap, endpoint)
	}
}

func TestAppDefaultsFeaturesDisabled(t *testing.T) {
	response := request(t, newTestApp(t, t.TempDir()), "/api/features")
	var payload features.Data
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.MermaidTldraw {
		t.Fatal("mermaid-tldraw enabled by default")
	}
}

func TestAppHighlightsKnownCodeAndPreservesOtherFences(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "diagrams.md"), strings.Join([]string{
		"```mermaid",
		`flowchart LR`,
		`    A["<script>alert('no')</script> & text"] --> B`,
		"```",
		"",
		"```go",
		`fmt.Println("<ordinary> & code")`,
		"```",
		"",
		"```unknown-language",
		`<unknown> & code`,
		"```",
		"",
		"```",
		`<plain> & code`,
		"```",
	}, "\n"))
	app := newTestApp(t, root)
	response, data := requestPageData(t, app, "/api/page?path=diagrams.md")
	if response.Code != http.StatusOK {
		t.Fatalf("document status = %d, want %d", response.Code, http.StatusOK)
	}
	body := data.Content
	assertContains(t, body,
		"<pre><code class=\"language-mermaid\">flowchart LR\n",
		`A[&quot;&lt;script&gt;alert('no')&lt;/script&gt; &amp; text&quot;] --&gt; B`,
		`<pre><code class="language-go syntax-highlight">`,
		`<span class="syntax-nf">Println</span>`,
		`<span class="syntax-s">&#34;&lt;ordinary&gt; &amp; code&#34;</span>`,
		`<pre><code class="language-unknown-language">&lt;unknown&gt; &amp; code`,
		`<pre><code>&lt;plain&gt; &amp; code`,
	)
	if strings.Contains(body, "<script>alert") {
		t.Fatal("Mermaid source was rendered as raw HTML")
	}
	for _, target := range []string{"/", "/view?path=missing.md"} {
		if strings.Contains(request(t, app, target).Body.String(), "MermaidCanvas") {
			t.Errorf("GET %s eagerly includes Mermaid chunk", target)
		}
	}
}

func TestSearchDocumentsReturnsPathsNamesAndVisibleText(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "docs", "Guide.md"), strings.Join([]string{
		"# Install Guide",
		"Read the **setup instructions** and [reference](https://example.com).",
		"Use `servef`.",
		"```sh",
		"servef docs",
		"```",
		"<script>hiddenMarkup()</script>",
	}, "\n\n"))
	writeMarkdown(t, filepath.Join(root, "node_modules", "ignored.md"), "# Ignored")

	response := request(t, newTestApp(t, root), "/api/search-documents")
	if response.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}

	var payload searchDocumentsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Documents) != 1 {
		t.Fatalf("documents = %#v, want one", payload.Documents)
	}
	document := payload.Documents[0]
	if document.Path != "docs/Guide.md" || document.Name != "Guide.md" {
		t.Fatalf("document identity = %#v", document)
	}
	assertContains(t, document.Content, "Install Guide", "setup instructions", "reference", "servef", "servef docs", "hiddenMarkup")
	for _, unwanted := range []string{"https://example.com", "**", "```", root} {
		if strings.Contains(document.Content, unwanted) {
			t.Errorf("search content contains %q: %s", unwanted, document.Content)
		}
	}
}

func TestSearchDocumentsRescansAndReportsUnreadableFiles(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "first.md"), "# First")
	app := newTestApp(t, root)

	first := request(t, app, "/api/search-documents")
	assertContains(t, first.Body.String(), "first.md")
	writeMarkdown(t, filepath.Join(root, "second.md"), "# Second")
	second := request(t, app, "/api/search-documents")
	assertContains(t, second.Body.String(), "first.md", "second.md")

	originalReadFile := app.readFile
	app.readFile = func(name string) ([]byte, error) {
		if filepath.Base(name) == "second.md" {
			return nil, errors.New("test read failure")
		}
		return originalReadFile(name)
	}
	partial := request(t, app, "/api/search-documents")
	if partial.Code != http.StatusOK {
		t.Fatalf("partial status = %d, want %d", partial.Code, http.StatusOK)
	}
	var payload searchDocumentsResponse
	if err := json.Unmarshal(partial.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Documents) != 1 || payload.Documents[0].Path != "first.md" {
		t.Fatalf("partial documents = %#v", payload.Documents)
	}
	if len(payload.Warnings) != 1 || !strings.Contains(payload.Warnings[0], "second.md: test read failure") {
		t.Fatalf("warnings = %#v", payload.Warnings)
	}
}

func TestSearchDocumentsEncodesHostileMarkdownAsJSON(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "hostile.md"), "# Safe\n\nText </script><script>alert(1)</script> tail")
	response := request(t, newTestApp(t, root), "/api/search-documents")
	if strings.Contains(response.Body.String(), "</script>") {
		t.Fatalf("JSON response contains an unescaped script terminator: %s", response.Body.String())
	}
	var payload searchDocumentsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Documents) != 1 {
		t.Fatalf("documents = %#v", payload.Documents)
	}
}

func TestSearchDocumentsReturnsJSONForFatalScan(t *testing.T) {
	root := t.TempDir()
	app := newTestApp(t, root)
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	response := request(t, app, "/api/search-documents")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("search status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	var payload errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error == "" {
		t.Fatal("JSON error is empty")
	}
}

func TestBrowserAssetsAndCSP(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	shell := request(t, app, "/")
	assetPattern := regexp.MustCompile(`/assets/[^"']+\.(?:css|js)`)
	targets := assetPattern.FindAllString(shell.Body.String(), -1)
	if len(targets) < 2 {
		t.Fatalf("shell assets = %v", targets)
	}
	for _, target := range targets {
		response := request(t, app, target)
		if response.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want %d", target, response.Code, http.StatusOK)
		}
		wantType := "text/javascript; charset=utf-8"
		if strings.HasSuffix(target, ".css") {
			wantType = "text/css; charset=utf-8"
		}
		if got := response.Header().Get("Content-Type"); got != wantType {
			t.Errorf("GET %s Content-Type = %q", target, got)
		}
	}
	entries, err := fs.ReadDir(assets, "dist")
	if err != nil {
		t.Fatal(err)
	}
	foundMermaidChunk := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "MermaidCanvas-") && strings.HasSuffix(entry.Name(), ".js") {
			foundMermaidChunk = true
			if response := request(t, app, "/assets/"+entry.Name()); response.Code != http.StatusOK {
				t.Errorf("GET Mermaid chunk status = %d", response.Code)
			}
		}
	}
	if !foundMermaidChunk {
		t.Fatal("Mermaid chunk not found")
	}
	px0License := request(t, app, "/assets/vendor/PX0-LICENSE.txt")
	if px0License.Code != http.StatusOK {
		t.Fatalf("GET px0 license status = %d, want %d", px0License.Code, http.StatusOK)
	}
	assertContains(t, px0License.Body.String(), "MIT License", "Arpit Bhayani")
	csp := shell.Header().Get("Content-Security-Policy")
	wantCSP := "default-src 'none'; img-src data: blob: http: https:; font-src 'self' https://cdn.tldraw.com https://esm.sh; style-src 'self' 'unsafe-inline'; script-src 'self'; worker-src 'self'; connect-src 'self' https://cdn.tldraw.com; base-uri 'none'; form-action 'none'"
	if csp != wantCSP {
		t.Errorf("Content-Security-Policy = %q, want %q", csp, wantCSP)
	}
}

func TestAppRescansOnEveryRequest(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "first.md"), "# First\n")
	app := newTestApp(t, root)

	first := request(t, app, "/api/page")
	if strings.Contains(first.Body.String(), "second.md") {
		t.Fatal("second.md appeared before it existed")
	}
	writeMarkdown(t, filepath.Join(root, "second.md"), "# Second\n")
	second := request(t, app, "/api/page")
	assertContains(t, second.Body.String(), "first.md", "second.md")
}

func TestAppRewritesLocalMarkdownLinksToViewRoutes(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "README.md"), "# Home\n")
	writeMarkdown(t, filepath.Join(root, "docs", "Guide.MD"), strings.Join([]string{
		"[root](../README.md#top)",
		"[sibling](Other.markdown?plain=1#details)",
		"[root relative](/README.md)",
		"[external](https://example.com/README.md)",
		"[anchor](#section)",
		"[other file](notes.txt)",
	}, "\n\n"))
	writeMarkdown(t, filepath.Join(root, "docs", "Other.markdown"), "# Other\n")

	document, data := requestPageData(t, newTestApp(t, root), "/api/page?path=docs%2FGuide.MD")
	if document.Code != http.StatusOK {
		t.Fatalf("document status = %d, want %d", document.Code, http.StatusOK)
	}
	body := data.Content
	assertContains(t, body,
		`href="/view?path=README.md#top"`,
		`href="/view?path=docs%2FOther.markdown&amp;plain=1#details"`,
		`href="/view?path=README.md"`,
		`href="https://example.com/README.md"`,
		`href="#section"`,
		`href="notes.txt"`,
	)
}

func TestAppRewritesLocalImageURLs(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "docs", "Guide.md"), strings.Join([]string{
		"![nested](../images/example.png#crop)",
		"![root](/images/example.svg?theme=dark)",
		"![query](local.webp?path=ignored&size=2)",
		"![external](https://example.com/image.png)",
		"![data](data:image/png;base64,AAAA)",
		"![blob](blob:https://example.com/id)",
		"![protocol relative](//example.com/image.png)",
	}, "\n\n"))

	response, data := requestPageData(t, newTestApp(t, root), "/api/page?path=docs%2FGuide.md")
	if response.Code != http.StatusOK {
		t.Fatalf("document status = %d, want %d", response.Code, http.StatusOK)
	}
	assertContains(t, data.Content,
		`src="/api/image?path=images%2Fexample.png#crop"`,
		`src="/api/image?path=images%2Fexample.svg&amp;theme=dark"`,
		`src="/api/image?path=docs%2Flocal.webp&amp;size=2"`,
		`src="https://example.com/image.png"`,
		`src="data:image/png;base64,AAAA"`,
		`src="blob:https://example.com/id"`,
		`src="//example.com/image.png"`,
	)
}

func TestAppServesSupportedImages(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name        string
		contentType string
	}{
		{name: "image.png", contentType: "image/png"},
		{name: "image.jpg", contentType: "image/jpeg"},
		{name: "image.jpeg", contentType: "image/jpeg"},
		{name: "image.gif", contentType: "image/gif"},
		{name: "image.webp", contentType: "image/webp"},
		{name: "image.avif", contentType: "image/avif"},
		{name: "image.svg", contentType: "image/svg+xml"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := []byte("exact bytes for " + test.name)
			name := filepath.Join(root, "images", test.name)
			if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, content, 0o644); err != nil {
				t.Fatal(err)
			}

			response := request(t, newTestApp(t, root), "/api/image?path=images%2F"+test.name)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if got := response.Header().Get("Content-Type"); got != test.contentType {
				t.Errorf("Content-Type = %q, want %q", got, test.contentType)
			}
			if got := response.Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("Cache-Control = %q, want no-cache", got)
			}
			if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
			if got := response.Body.Bytes(); !bytes.Equal(got, content) {
				t.Errorf("body = %q, want %q", got, content)
			}
		})
	}
}

func TestAppRejectsUnsafeAndUnavailableImages(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "directory.png", "nested.md"), "# nested")
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.png")); err != nil {
		t.Fatal(err)
	}

	app := newTestApp(t, root)
	for _, target := range []string{
		"/api/image",
		"/api/image?path=missing.png",
		"/api/image?path=notes.txt",
		"/api/image?path=directory.png",
		"/api/image?path=..%2Foutside.png",
		"/api/image?path=%2Foutside.png",
		"/api/image?path=escape.png",
	} {
		t.Run(target, func(t *testing.T) {
			response := request(t, app, target)
			if response.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
		})
	}
}

func TestAppHandlesEmptyMissingAndUnsafeSelections(t *testing.T) {
	root := t.TempDir()
	app := newTestApp(t, root)

	_, empty := requestPageData(t, app, "/api/page")
	if !empty.Empty {
		t.Fatal("empty root was not reported empty")
	}

	missing := request(t, app, "/view?path=missing.md")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want %d", missing.Code, http.StatusNotFound)
	}
	assertContains(t, missing.Body.String(), "unavailable")

	unsafe := request(t, app, "/view?path=..%2Foutside.md")
	if unsafe.Code != http.StatusBadRequest {
		t.Fatalf("unsafe status = %d, want %d", unsafe.Code, http.StatusBadRequest)
	}

	noSelection := request(t, app, "/view")
	if noSelection.Code != http.StatusBadRequest {
		t.Fatalf("empty selection status = %d, want %d", noSelection.Code, http.StatusBadRequest)
	}
}

func TestAppRejectsUnknownRoutesAndMethods(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	unknown := request(t, app, "/unknown")
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown status = %d, want %d", unknown.Code, http.StatusNotFound)
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, req)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/search-documents", nil)
	response = httptest.NewRecorder()
	app.ServeHTTP(response, req)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST search status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func newTestApp(t *testing.T, root string) *App {
	t.Helper()
	app, err := New(Config{Root: root, Depth: 5, Exclusions: []string{".git", "node_modules", "vendor"}})
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func request(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func requestPageData(t *testing.T, handler http.Handler, target string) (*httptest.ResponseRecorder, pageData) {
	t.Helper()
	response := request(t, handler, target)
	var data pageData
	if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	return response, data
}

func writeMarkdown(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, value string, substrings ...string) {
	t.Helper()
	for _, substring := range substrings {
		if !strings.Contains(value, substring) {
			t.Errorf("response does not contain %q\n%s", substring, value)
		}
	}
}
