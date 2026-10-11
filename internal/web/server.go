// Package web serves an indexed local file workspace and document views.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/features"
	"github.com/flexdinesh/servef/internal/files"
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
	Types      *document.Registry
}

// App composes discovery and document handlers into the HTTP API.
type App struct {
	config   Config
	metrics  metricsCollector
	readFile func(string) ([]byte, error)
	shell    string
}

type pageData struct {
	RootName     string          `json:"rootName"`
	Tree         []treeNode      `json:"tree"`
	Selected     string          `json:"selected"`
	Content      string          `json:"content"`
	Source       string          `json:"source"`
	FileSize     int64           `json:"fileSize"`
	FileCount    int             `json:"fileCount"`
	HasFile      bool            `json:"hasFile"`
	Empty        bool            `json:"empty"`
	Error        string          `json:"error"`
	Warnings     []string        `json:"warnings"`
	FileTypes    []document.Type `json:"fileTypes"`
	DocumentType *document.Type  `json:"documentType"`
	SourceLines  []string        `json:"sourceLines"`
	PreviewError string          `json:"previewError"`
}

type treeNode struct {
	FileType string     `json:"fileType"`
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

	if config.Types == nil {
		config.Types = document.Builtins()
	}
	return &App{config: config, readFile: os.ReadFile, shell: string(shell)}, nil
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

	index, err := files.Scan(a.config.Root, a.config.Depth, a.config.Exclusions, a.config.Types.Supports)
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
		handler, _ := a.config.Types.Lookup(indexedPath)
		content, err := handler.SearchText(source)
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

func (a *App) servePageData(w http.ResponseWriter, selected string, required bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	data, status := a.page(selected, required)
	writeJSON(w, status, data)
}

func (a *App) page(selected string, required bool) (pageData, int) {
	index, err := files.Scan(a.config.Root, a.config.Depth, a.config.Exclusions, a.config.Types.Supports)
	if err != nil {
		return pageData{
			RootName:    filepath.Base(a.config.Root),
			Tree:        []treeNode{},
			FileTypes:   a.config.Types.Types(),
			SourceLines: []string{},
			Error:       err.Error(),
			Warnings:    []string{},
		}, http.StatusInternalServerError
	}
	data := pageData{
		RootName:    filepath.Base(index.RootPath()),
		Selected:    selected,
		FileCount:   len(index.Files),
		Empty:       len(index.Files) == 0,
		Warnings:    warningStrings(index.Warnings),
		FileTypes:   a.config.Types.Types(),
		SourceLines: []string{},
	}
	data.Tree = a.makeTree(index.Tree.Children, selected)
	if required && selected == "" {
		data.Error = "No file was selected."
		return data, http.StatusBadRequest
	}

	if selected != "" {
		resolved, err := index.Resolve(selected)
		if err != nil {
			status := http.StatusNotFound
			if errors.Is(err, files.ErrPathEscape) {
				status = http.StatusBadRequest
			}
			data.Error = "That file is unavailable. Refresh the index and choose another file."
			return data, status
		}
		source, err := a.readFile(resolved)
		if err != nil {
			data.Error = fmt.Sprintf("Could not read %s: %v", selected, err)
			return data, http.StatusInternalServerError
		}
		handler, _ := a.config.Types.Lookup(selected)
		fileType := handler.Type()
		data.DocumentType = &fileType
		data.Source = string(source)
		data.SourceLines = document.HighlightLines(source, fileType.Language)
		data.Content, err = handler.Preview(source, document.Context{Path: selected, Supports: a.config.Types.Supports})
		if err != nil {
			data.PreviewError = err.Error()
		}
		data.FileSize = int64(len(source))
		data.HasFile = true
	}
	return data, http.StatusOK
}

func (a *App) makeTree(entries []files.Entry, selected string) []treeNode {
	nodes := make([]treeNode, 0, len(entries))
	for _, entry := range entries {
		node := treeNode{Name: entry.Name, Path: entry.Path, IsDir: entry.IsDir, Selected: entry.Path == selected}
		node.Children = a.makeTree(entry.Children, selected)
		if handler, ok := a.config.Types.Lookup(entry.Path); !entry.IsDir && ok {
			node.FileType = handler.Type().ID
		}
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
