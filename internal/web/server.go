// Package web serves a live Markdown index and rendered documents.
package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/servef/internal/features"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

//go:embed dist vendor/MINISEARCH-LICENSE.txt vendor/PX0-LICENSE.txt
var assets embed.FS

var noticeAssets = map[string]string{
	"/assets/vendor/MINISEARCH-LICENSE.txt": "vendor/MINISEARCH-LICENSE.txt",
	"/assets/vendor/PX0-LICENSE.txt":        "vendor/PX0-LICENSE.txt",
}

// Config controls the live file scan performed for each request.
type Config struct {
	Root       string
	Depth      int
	Exclusions []string
	Features   features.Set
}

// App is an HTTP handler for the Markdown browser.
type App struct {
	config   Config
	markdown goldmark.Markdown
	metrics  metricsCollector
	readFile func(string) ([]byte, error)
	shell    string
}

type pageData struct {
	RootName  string     `json:"rootName"`
	Tree      []treeNode `json:"tree"`
	Selected  string     `json:"selected"`
	Content   string     `json:"content"`
	Source    string     `json:"source"`
	FileSize  int64      `json:"fileSize"`
	FileCount int        `json:"fileCount"`
	HasFile   bool       `json:"hasFile"`
	Empty     bool       `json:"empty"`
	Error     string     `json:"error"`
	Warnings  []string   `json:"warnings"`
}

type treeNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"isDir"`
	Open     bool       `json:"open"`
	Selected bool       `json:"selected"`
	Children []treeNode `json:"children"`
}

type searchDocument struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

type searchDocumentsResponse struct {
	Documents []searchDocument `json:"documents"`
	Warnings  []string         `json:"warnings"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// New constructs a request-time scanning web interface.
func New(config Config) (*App, error) {
	shell, err := fs.ReadFile(assets, "dist/index.html")
	if err != nil {
		return nil, err
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(
			util.Prioritized(&escapedHTMLRenderer{}, 500),
			util.Prioritized(newFencedCodeRenderer(), 500),
		)),
	)
	return &App{config: config, markdown: md, readFile: os.ReadFile, shell: string(shell)}, nil
}

// ServeHTTP rescans the configured directory before rendering every response.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data: blob: http: https:; font-src 'self' https://cdn.tldraw.com https://esm.sh; style-src 'self' 'unsafe-inline'; script-src 'self'; worker-src 'self'; connect-src 'self' https://cdn.tldraw.com; base-uri 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if asset, ok := noticeAssets[r.URL.Path]; ok {
		a.serveAsset(w, asset)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		a.serveFrontendAsset(w, strings.TrimPrefix(r.URL.Path, "/assets/"))
		return
	}
	if r.URL.Path == "/api/search-documents" {
		a.serveSearchDocuments(w)
		return
	}
	if r.URL.Path == "/api/features" {
		a.serveFeatures(w)
		return
	}
	if r.URL.Path == "/api/metrics" {
		a.serveMetrics(w)
		return
	}
	if r.URL.Path == "/api/image" {
		a.serveImage(w, r.URL.Query().Get("path"))
		return
	}
	if r.URL.Path == "/api/page" {
		selected, required := r.URL.Query()["path"]
		pathValue := ""
		if len(selected) > 0 {
			pathValue = selected[0]
		}
		a.servePageData(w, pathValue, required)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/view" {
		http.NotFound(w, r)
		return
	}

	selected := ""
	required := r.URL.Path == "/view"
	if required {
		selected = r.URL.Query().Get("path")
	}
	data, status := a.page(selected, required)
	a.serveShell(w, status, data)
}

func (a *App) serveAsset(w http.ResponseWriter, name string) {
	content, err := fs.ReadFile(assets, name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(content)
}

func (a *App) serveFrontendAsset(w http.ResponseWriter, name string) {
	if !fs.ValidPath(name) {
		http.NotFound(w, nil)
		return
	}
	content, err := fs.ReadFile(assets, "dist/"+name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	switch path.Ext(name) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(content)
}

func (a *App) serveShell(w http.ResponseWriter, status int, data pageData) {
	encoded, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "could not render page", http.StatusInternalServerError)
		return
	}
	content := strings.Replace(a.shell, "{{APP_DATA}}", string(encoded), 1)
	encodedFeatures, err := json.Marshal(a.config.Features.BrowserData())
	if err != nil {
		http.Error(w, "could not render page", http.StatusInternalServerError)
		return
	}
	content = strings.Replace(content, "{{FEATURE_DATA}}", string(encodedFeatures), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(content))
}

func (a *App) serveFeatures(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, a.config.Features.BrowserData())
}

func (a *App) serveMetrics(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, a.metrics.sample())
}

func (a *App) serveImage(w http.ResponseWriter, name string) {
	contentType, ok := imageContentType(name)
	if !ok {
		http.NotFound(w, nil)
		return
	}
	resolved, err := resolveImage(a.config.Root, name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(content)
}

func imageContentType(name string) (string, bool) {
	switch strings.ToLower(path.Ext(name)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	case ".avif":
		return "image/avif", true
	case ".svg":
		return "image/svg+xml", true
	default:
		return "", false
	}
}

func resolveImage(root, name string) (string, error) {
	if name == "" || filepath.IsAbs(name) {
		return "", errors.New("invalid image path")
	}
	rel := path.Clean(filepath.ToSlash(name))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", errors.New("invalid image path")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	canonicalRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(canonicalRoot, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	relToRoot, err := filepath.Rel(canonicalRoot, resolved)
	if err != nil || filepath.IsAbs(relToRoot) || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", errors.New("image path escapes root")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("image is not a regular file")
	}
	return resolved, nil
}

func (a *App) serveSearchDocuments(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	index, err := files.Scan(a.config.Root, a.config.Depth, a.config.Exclusions)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	response := searchDocumentsResponse{
		Documents: make([]searchDocument, 0, len(index.Files)),
		Warnings:  warningStrings(index.Warnings),
	}
	for _, indexedPath := range index.Files {
		resolved, err := index.Resolve(indexedPath)
		if err != nil {
			response.Warnings = append(response.Warnings, fmt.Sprintf("%s: %v", indexedPath, err))
			continue
		}
		source, err := a.readFile(resolved)
		if err != nil {
			response.Warnings = append(response.Warnings, fmt.Sprintf("%s: %v", indexedPath, err))
			continue
		}
		content, err := a.searchText(source)
		if err != nil {
			response.Warnings = append(response.Warnings, fmt.Sprintf("%s: %v", indexedPath, err))
			continue
		}
		response.Documents = append(response.Documents, searchDocument{
			Path:    indexedPath,
			Name:    path.Base(indexedPath),
			Content: content,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (a *App) searchText(source []byte) (string, error) {
	document := a.markdown.Parser().Parse(text.NewReader(source))
	parts := make([]string, 0)
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch value := node.(type) {
		case *ast.Text:
			parts = append(parts, string(value.Value(source)))
		case *ast.String:
			parts = append(parts, string(value.Value))
		case *ast.AutoLink:
			parts = append(parts, string(value.Text(source)))
		case *ast.CodeBlock:
			parts = append(parts, string(value.Lines().Value(source)))
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock:
			parts = append(parts, string(value.Lines().Value(source)))
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			parts = append(parts, string(value.Text(source)))
			return ast.WalkSkipChildren, nil
		case *ast.HTMLBlock:
			parts = append(parts, string(value.Text(source)))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " "), nil
}

func (a *App) servePageData(w http.ResponseWriter, selected string, required bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	data, status := a.page(selected, required)
	writeJSON(w, status, data)
}

func (a *App) page(selected string, required bool) (pageData, int) {
	index, err := files.Scan(a.config.Root, a.config.Depth, a.config.Exclusions)
	if err != nil {
		return pageData{
			RootName: filepath.Base(a.config.Root),
			Tree:     []treeNode{},
			Error:    err.Error(),
			Warnings: []string{},
		}, http.StatusInternalServerError
	}
	data := pageData{
		RootName:  filepath.Base(index.RootPath()),
		Selected:  selected,
		FileCount: len(index.Files),
		Empty:     len(index.Files) == 0,
		Warnings:  warningStrings(index.Warnings),
	}
	data.Tree = makeTree(index.Tree.Children, selected)
	if required && selected == "" {
		data.Error = "No Markdown file was selected."
		return data, http.StatusBadRequest
	}

	if selected != "" {
		resolved, err := index.Resolve(selected)
		if err != nil {
			status := http.StatusNotFound
			if errors.Is(err, files.ErrPathEscape) {
				status = http.StatusBadRequest
			}
			data.Error = "That Markdown file is unavailable. Refresh the index and choose another file."
			return data, status
		}
		source, err := a.readFile(resolved)
		if err != nil {
			data.Error = fmt.Sprintf("Could not read %s: %v", selected, err)
			return data, http.StatusInternalServerError
		}
		data.Source = string(source)
		var rendered bytes.Buffer
		if err := a.renderMarkdown(source, selected, &rendered); err != nil {
			data.Error = fmt.Sprintf("Could not render %s: %v", selected, err)
			return data, http.StatusInternalServerError
		}
		data.Content = rendered.String() // Goldmark escapes raw HTML via escapedHTMLRenderer.
		data.FileSize = int64(len(source))
		data.HasFile = true
	}
	return data, http.StatusOK
}

func (a *App) renderMarkdown(source []byte, selected string, output *bytes.Buffer) error {
	document := a.markdown.Parser().Parse(text.NewReader(source))
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch value := node.(type) {
			case *ast.Link:
				value.Destination = rewriteMarkdownLink(value.Destination, selected)
			case *ast.Image:
				value.Destination = rewriteMarkdownImage(value.Destination, selected)
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return err
	}
	return a.markdown.Renderer().Render(output, source, document)
}

func rewriteMarkdownImage(destination []byte, selected string) []byte {
	target, err := url.Parse(string(destination))
	if err != nil || target.Scheme != "" || target.Host != "" || target.Path == "" {
		return destination
	}

	imagePath := target.Path
	if strings.HasPrefix(imagePath, "/") {
		imagePath = strings.TrimPrefix(imagePath, "/")
	} else {
		imagePath = path.Join(path.Dir(selected), imagePath)
	}
	imagePath = path.Clean(imagePath)

	query := target.Query()
	query.Set("path", imagePath)
	return []byte((&url.URL{
		Path:     "/api/image",
		RawQuery: query.Encode(),
		Fragment: target.Fragment,
	}).String())
}

func rewriteMarkdownLink(destination []byte, selected string) []byte {
	target, err := url.Parse(string(destination))
	if err != nil || target.Scheme != "" || target.Host != "" || target.Path == "" {
		return destination
	}
	extension := strings.ToLower(path.Ext(target.Path))
	if extension != ".md" && extension != ".markdown" {
		return destination
	}

	markdownPath := target.Path
	if strings.HasPrefix(markdownPath, "/") {
		markdownPath = strings.TrimPrefix(markdownPath, "/")
	} else {
		markdownPath = path.Join(path.Dir(selected), markdownPath)
	}
	markdownPath = path.Clean(markdownPath)

	query := target.Query()
	query.Set("path", markdownPath)
	return []byte((&url.URL{
		Path:     "/view",
		RawQuery: query.Encode(),
		Fragment: target.Fragment,
	}).String())
}

func makeTree(entries []files.Entry, selected string) []treeNode {
	nodes := make([]treeNode, 0, len(entries))
	for _, entry := range entries {
		node := treeNode{Name: entry.Name, Path: entry.Path, IsDir: entry.IsDir, Selected: entry.Path == selected}
		node.Children = makeTree(entry.Children, selected)
		node.Open = entry.IsDir && (selected == entry.Path || strings.HasPrefix(selected, entry.Path+"/"))
		nodes = append(nodes, node)
	}
	return nodes
}

func warningStrings(warnings []files.Warning) []string {
	result := make([]string, len(warnings))
	for i, warning := range warnings {
		result[i] = warning.Error()
	}
	return result
}

type escapedHTMLRenderer struct{}

func (*escapedHTMLRenderer) RegisterFuncs(register renderer.NodeRendererFuncRegisterer) {
	register.Register(ast.KindHTMLBlock, renderEscapedHTMLBlock)
	register.Register(ast.KindRawHTML, renderEscapedRawHTML)
}

func renderEscapedHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	block := node.(*ast.HTMLBlock)
	if entering {
		for i := 0; i < block.Lines().Len(); i++ {
			line := block.Lines().At(i)
			_, _ = w.Write(util.EscapeHTML(line.Value(source)))
		}
	} else if block.HasClosure() {
		_, _ = w.Write(util.EscapeHTML(block.ClosureLine.Value(source)))
	}
	return ast.WalkContinue, nil
}

func renderEscapedRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	raw := node.(*ast.RawHTML)
	for i := 0; i < raw.Segments.Len(); i++ {
		segment := raw.Segments.At(i)
		_, _ = w.Write(util.EscapeHTML(segment.Value(source)))
	}
	return ast.WalkSkipChildren, nil
}
