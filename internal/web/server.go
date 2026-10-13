// Package web serves an indexed local file workspace and document views.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/features"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/projects"
)

//go:embed dist vendor/MINISEARCH-LICENSE.txt vendor/PX0-LICENSE.txt
var assets embed.FS

var noticeAssets = map[string]string{
	"/assets/vendor/MINISEARCH-LICENSE.txt": "vendor/MINISEARCH-LICENSE.txt",
	"/assets/vendor/PX0-LICENSE.txt":        "vendor/PX0-LICENSE.txt",
}

// Config controls project discovery and the live file scan performed for each
// request. Root confines every project; ScanDepth bounds repository discovery;
// Depth bounds the file scan within a project.
type Config struct {
	Root       string
	ScanDepth  int
	Depth      int
	Exclusions []string
	Features   features.Set
	// Types decides presentation; Filenames decides which files are in scope.
	Types     *document.Registry
	Filenames files.NameFilter
}

// App composes discovery and document handlers into the HTTP API.
type App struct {
	config Config
	// servedRoot is the absolute root; the browser keys stored state by it.
	servedRoot string
	metrics    metricsCollector
	openFile   func(string) (fs.File, error)
	shell      string
}

type pageData struct {
	ServedRoot string `json:"servedRoot"`
	Project    string `json:"project"`
	// ProjectUnavailable reports that the project itself could not be
	// resolved, as opposed to a file within it.
	ProjectUnavailable bool           `json:"projectUnavailable"`
	RootName           string         `json:"rootName"`
	Tree               []treeNode     `json:"tree"`
	Selected           string         `json:"selected"`
	Content            string         `json:"content"`
	Source             string         `json:"source"`
	FileSize           int64          `json:"fileSize"`
	FileCount          int            `json:"fileCount"`
	HasFile            bool           `json:"hasFile"`
	Empty              bool           `json:"empty"`
	Error              string         `json:"error"`
	Warnings           []string       `json:"warnings"`
	FilenamePatterns   []string       `json:"filenamePatterns"`
	DocumentType       *document.Type `json:"documentType"`
	Binary             bool           `json:"binary"`
	TooLarge           bool           `json:"tooLarge"`
	PreviewDisabled    bool           `json:"previewDisabled"`
	SearchExcluded     bool           `json:"searchExcluded"`
	SourceLines        []string       `json:"sourceLines"`
	PreviewError       string         `json:"previewError"`
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

type formatResponse struct {
	FormattedLines []string `json:"formattedLines"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type projectsResponse struct {
	ServedRoot string             `json:"servedRoot"`
	RootName   string             `json:"rootName"`
	Projects   []projects.Project `json:"projects"`
	Warnings   []string           `json:"warnings"`
}

type directoriesResponse struct {
	Path        string               `json:"path"`
	Directories []projects.Directory `json:"directories"`
}

// workspace is the project directory a request is scoped to.
type workspace struct {
	id   string
	root string
}

const (
	noProjectMessage          = "No project was selected."
	projectUnavailableMessage = "Project unavailable."
)

// indexedFile is a read of an indexed file. Binary files keep only the sniffed
// prefix; files over the read limit are not read.
type indexedFile struct {
	source   []byte
	size     int64
	binary   bool
	tooLarge bool
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
	if len(config.Filenames.Patterns()) == 0 {
		config.Filenames = files.AllNames()
	}
	servedRoot, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(servedRoot); err == nil {
		servedRoot = resolved
	}
	return &App{config: config, servedRoot: servedRoot, openFile: openFile, shell: string(shell)}, nil
}

func openFile(name string) (fs.File, error) { return os.Open(name) }

// readIndexed reads a resolved file unless it is larger than limit (when
// limit >= 0). Binary files stop after the sniffed prefix; types without a text
// source are binary without reading content.
func (a *App) readIndexed(resolved string, fileType document.Type, limit int64) (indexedFile, error) {
	file, err := a.openFile(resolved)
	if err != nil {
		return indexedFile{}, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return indexedFile{}, err
	}
	result := indexedFile{size: info.Size()}
	if limit >= 0 && result.size > limit {
		result.tooLarge = true
		return result, nil
	}
	if !fileType.HasTextSource() {
		result.binary = true
		return result, nil
	}
	prefix := make([]byte, document.SniffLength)
	read, err := io.ReadFull(file, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return indexedFile{}, err
	}
	prefix = prefix[:read]
	if document.IsBinary(prefix) {
		result.binary = true
		return result, nil
	}
	rest, err := io.ReadAll(file)
	if err != nil {
		return indexedFile{}, err
	}
	result.source = append(prefix, rest...)
	return result, nil
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
	query := r.URL.Query()
	projectID := query.Get("project")
	if r.URL.Path == "/api/projects" {
		a.serveProjects(w)
		return
	}
	if r.URL.Path == "/api/directories" {
		a.serveDirectories(w, query.Get("path"))
		return
	}
	if r.URL.Path == "/api/search-documents" {
		a.serveSearchDocuments(w, projectID)
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
		a.serveImage(w, projectID, query.Get("path"))
		return
	}
	if r.URL.Path == "/api/raw" {
		a.serveRaw(w, projectID, query.Get("path"))
		return
	}
	if r.URL.Path == "/api/format" {
		a.serveFormat(w, projectID, query.Get("path"))
		return
	}
	if r.URL.Path == "/api/page" {
		selected, required := query["path"]
		pathValue := ""
		if len(selected) > 0 {
			pathValue = selected[0]
		}
		a.servePageData(w, projectID, pathValue, required)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/view" {
		http.NotFound(w, r)
		return
	}

	// Without a project the browser chooses one, so no page data is embedded.
	if projectID == "" {
		a.serveShell(w, http.StatusOK, nil)
		return
	}
	selected := ""
	required := r.URL.Path == "/view"
	if required {
		selected = query.Get("path")
	}
	data, status := a.page(projectID, selected, required)
	a.serveShell(w, status, &data)
}

// project resolves a request's project ID within the root.
func (a *App) project(id string) (workspace, int, string) {
	if id == "" {
		return workspace{}, http.StatusBadRequest, noProjectMessage
	}
	root, err := projects.Resolve(a.config.Root, id, a.config.Exclusions)
	if errors.Is(err, files.ErrPathEscape) {
		return workspace{}, http.StatusBadRequest, projectUnavailableMessage
	}
	if err != nil {
		return workspace{}, http.StatusNotFound, projectUnavailableMessage
	}
	return workspace{id: id, root: root}, http.StatusOK, ""
}

func (a *App) serveProjects(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	discovery, err := projects.Discover(a.config.Root, a.config.ScanDepth, a.config.Exclusions)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, projectsResponse{
		ServedRoot: a.servedRoot,
		RootName:   filepath.Base(a.servedRoot),
		Projects:   discovery.Projects,
		Warnings:   warningStrings(discovery.Warnings),
	})
}

func (a *App) serveDirectories(w http.ResponseWriter, id string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if id == "" {
		id = projects.RootID
	}
	directories, err := projects.Directories(a.config.Root, id, a.config.Exclusions)
	switch {
	case errors.Is(err, files.ErrPathEscape):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "That folder is outside the served directory."})
	case errors.Is(err, projects.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "That folder is unavailable."})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Could not read that folder."})
	default:
		writeJSON(w, http.StatusOK, directoriesResponse{Path: id, Directories: directories})
	}
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

func (a *App) serveShell(w http.ResponseWriter, status int, data *pageData) {
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

func (a *App) serveImage(w http.ResponseWriter, projectID, name string) {
	contentType, ok := imageContentType(name)
	if !ok {
		http.NotFound(w, nil)
		return
	}
	project, status, _ := a.project(projectID)
	if status != http.StatusOK {
		http.NotFound(w, nil)
		return
	}
	resolved, err := resolveImage(project.root, name)
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
	case ".ico":
		return "image/x-icon", true
	case ".bmp":
		return "image/bmp", true
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

func (a *App) serveSearchDocuments(w http.ResponseWriter, projectID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	index, err := a.scan(project.root)
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
		family := a.config.Types.Lookup(indexedPath)
		file, err := a.readIndexed(resolved, family.Type(), document.SearchLimit)
		if err != nil {
			response.Warnings = append(response.Warnings, fmt.Sprintf("%s: %v", indexedPath, err))
			continue
		}
		if file.tooLarge || file.binary {
			continue
		}
		content, err := family.SearchText(file.source)
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

// serveFormat returns the on-demand formatted view of a formattable file.
func (a *App) serveFormat(w http.ResponseWriter, projectID, name string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	index, err := a.scan(project.root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	resolved, err := index.Resolve(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "That file is unavailable."})
		return
	}
	family := a.config.Types.Lookup(name)
	fileType := family.Type()
	if !fileType.Formattable {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: fileType.Label + " files cannot be formatted."})
		return
	}
	file, err := a.readIndexed(resolved, fileType, document.PreviewLimit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	if file.tooLarge {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "File is too large to format."})
		return
	}
	if file.binary {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "binary files cannot be formatted"})
		return
	}
	lines, err := family.FormatLines(file.source)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, formatResponse{FormattedLines: lines})
}

// serveRaw returns the bytes of an indexed image-preview file in a sandbox.
func (a *App) serveRaw(w http.ResponseWriter, projectID, name string) {
	project, status, _ := a.project(projectID)
	if status != http.StatusOK {
		http.NotFound(w, nil)
		return
	}
	index, err := a.scan(project.root)
	if err != nil {
		http.Error(w, "could not scan files", http.StatusInternalServerError)
		return
	}
	resolved, err := index.Resolve(name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	contentType, ok := imageContentType(name)
	if !ok || a.config.Types.Lookup(name).Type().PreviewKind != document.PreviewImage {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	file, err := a.openFile(resolved)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, file)
}

func (a *App) scan(root string) (*files.Index, error) {
	return files.Scan(root, a.config.Depth, a.config.Exclusions, a.config.Filenames.Match)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (a *App) servePageData(w http.ResponseWriter, projectID, selected string, required bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	data, status := a.page(projectID, selected, required)
	writeJSON(w, status, data)
}

func (a *App) page(projectID, selected string, required bool) (pageData, int) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		return pageData{
			ServedRoot:         a.servedRoot,
			Project:            projectID,
			ProjectUnavailable: true,
			Selected:           selected,
			Tree:               []treeNode{},
			FilenamePatterns:   a.config.Filenames.Patterns(),
			SourceLines:        []string{},
			Error:              message,
			Warnings:           []string{},
		}, status
	}
	index, err := a.scan(project.root)
	if err != nil {
		return pageData{
			ServedRoot:       a.servedRoot,
			Project:          project.id,
			RootName:         filepath.Base(project.root),
			Tree:             []treeNode{},
			FilenamePatterns: a.config.Filenames.Patterns(),
			SourceLines:      []string{},
			Error:            err.Error(),
			Warnings:         []string{},
		}, http.StatusInternalServerError
	}
	data := pageData{
		ServedRoot:       a.servedRoot,
		Project:          project.id,
		RootName:         filepath.Base(index.RootPath()),
		Selected:         selected,
		FileCount:        len(index.Files),
		Empty:            len(index.Files) == 0,
		Warnings:         warningStrings(index.Warnings),
		FilenamePatterns: a.config.Filenames.Patterns(),
		SourceLines:      []string{},
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
		family := a.config.Types.Lookup(selected)
		fileType := family.Type()
		// Images are served from /api/raw, so only text display is capped.
		limit := int64(document.DisplayLimit)
		if fileType.PreviewKind == document.PreviewImage {
			limit = -1
		}
		file, err := a.readIndexed(resolved, fileType, limit)
		if err != nil {
			data.Error = fmt.Sprintf("Could not read %s: %v", selected, err)
			return data, http.StatusInternalServerError
		}
		data.DocumentType = &fileType
		data.FileSize = file.size
		data.HasFile = true
		data.Binary = file.binary
		if file.tooLarge {
			data.TooLarge = true
			data.PreviewDisabled = true
			data.SearchExcluded = true
			return data, http.StatusOK
		}
		// Image previews use raw bytes and no server work, so stay enabled at any
		// size; large SVG source is still shown as plain lines.
		large := file.size > document.PreviewLimit
		data.PreviewDisabled = large && fileType.PreviewKind != document.PreviewImage
		data.SearchExcluded = file.binary || file.size > document.SearchLimit
		if file.binary {
			return data, http.StatusOK
		}
		data.Source = string(file.source)
		if large {
			data.SourceLines = document.PlainLines(file.source)
			return data, http.StatusOK
		}
		data.SourceLines = document.HighlightLines(file.source, fileType.Language)
		data.Content, err = family.Preview(file.source, document.Context{Path: selected, Project: project.id, Supports: a.config.Filenames.Match})
		if err != nil {
			data.PreviewError = err.Error()
		}
	}
	return data, http.StatusOK
}

func (a *App) makeTree(entries []files.Entry, selected string) []treeNode {
	nodes := make([]treeNode, 0, len(entries))
	for _, entry := range entries {
		node := treeNode{Name: entry.Name, Path: entry.Path, IsDir: entry.IsDir, Selected: entry.Path == selected}
		node.Children = a.makeTree(entry.Children, selected)
		if !entry.IsDir {
			node.FileType = a.config.Types.Lookup(entry.Path).Type().ID
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
