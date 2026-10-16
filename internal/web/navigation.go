package web

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/lsp"
)

// navigationSource shares one request's CLI-scoped scan. Every read and
// returned location passes indexed-path resolution and eligibility checks.
type navigationSource struct {
	app   *App
	root  string
	once  sync.Once
	index *files.Index
	err   error
}

func (a *App) navigationSource(root string) *navigationSource {
	return &navigationSource{app: a, root: root}
}

func (s *navigationSource) scan() (*files.Index, error) {
	s.once.Do(func() { s.index, s.err = s.app.scan(s.root) })
	return s.index, s.err
}

// Passive status and capped results inspect only metadata and the binary prefix.
func (s *navigationSource) readFile(name string, source bool) ([]byte, string) {
	if source {
		text, _, reason := s.snapshot(name)
		return text, reason
	}
	index, err := s.scan()
	if err != nil {
		return nil, "Could not scan the project."
	}
	resolved, err := index.Resolve(name)
	if err != nil {
		return nil, "That file is unavailable."
	}
	if index.Ignored(name) {
		return nil, "Gitignored files do not support code navigation."
	}
	family := s.app.config.Types.Lookup(name).Type()
	file, err := s.app.openFile(resolved)
	if err != nil {
		return nil, "That file is unavailable."
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, "That file is unavailable."
	}
	if info.Size() > document.PreviewLimit {
		return nil, "Files over 4 MiB do not support code navigation."
	}
	if !family.HasTextSource() {
		return nil, "Binary files do not support code navigation."
	}
	prefix, err := io.ReadAll(io.LimitReader(file, document.SniffLength))
	if err != nil {
		return nil, "That file is unavailable."
	}
	if document.IsBinary(prefix) {
		return nil, "Binary files do not support code navigation."
	}
	return nil, ""
}

// snapshot pairs text with the descriptor's change stamp. A file changed or
// replaced while being read is retried, so its overlay never receives newer
// metadata alongside older bytes.
func (s *navigationSource) snapshot(name string) ([]byte, lsp.File, string) {
	index, err := s.scan()
	if err != nil {
		return nil, lsp.File{}, "Could not scan the project."
	}
	if index.Ignored(name) {
		return nil, lsp.File{}, "Gitignored files do not support code navigation."
	}
	for attempt := 0; attempt < 2; attempt++ {
		resolved, err := index.Resolve(name)
		if err != nil {
			return nil, lsp.File{}, "That file is unavailable."
		}
		text, stamp, reason, changed := s.readSnapshot(resolved, s.app.config.Types.Lookup(name).Type())
		if !changed {
			return text, stamp, reason
		}
	}
	return nil, lsp.File{}, "That file changed while being read. Try again."
}

func (s *navigationSource) readSnapshot(resolved string, family document.Type) ([]byte, lsp.File, string, bool) {
	file, err := s.app.openFile(resolved)
	if err != nil {
		return nil, lsp.File{}, "That file is unavailable.", false
	}
	defer func() { _ = file.Close() }()
	before, err := file.Stat()
	if err != nil {
		return nil, lsp.File{}, "That file is unavailable.", false
	}
	if before.Size() > document.PreviewLimit {
		return nil, lsp.File{}, "Files over 4 MiB do not support code navigation.", false
	}
	if !family.HasTextSource() {
		return nil, lsp.File{}, "Binary files do not support code navigation.", false
	}
	text, err := io.ReadAll(io.LimitReader(file, document.PreviewLimit+1))
	if err != nil {
		return nil, lsp.File{}, "That file is unavailable.", false
	}
	after, err := file.Stat()
	if err != nil {
		return nil, lsp.File{}, "That file is unavailable.", false
	}
	current, err := os.Stat(resolved)
	if err != nil {
		return nil, lsp.File{}, "That file is unavailable.", false
	}
	if !os.SameFile(before, current) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || after.Size() != current.Size() || !after.ModTime().Equal(current.ModTime()) || int64(len(text)) != after.Size() {
		return nil, lsp.File{}, "", true
	}
	if document.IsBinary(text[:min(len(text), document.SniffLength)]) {
		return nil, lsp.File{}, "Binary files do not support code navigation.", false
	}
	return text, lsp.File{Path: resolved, Size: after.Size(), Modified: after.ModTime()}, "", false
}

// The header authorizes process startup. Cross-origin browser requests need
// a CORS preflight, which servef never grants.
const languageServerHeader = "X-Servef-Language-Servers"

const (
	serverStartWait          = 20 * time.Second
	serverRequestTimeout     = 10 * time.Second
	navigationRequestTimeout = 25 * time.Second
	maxServerDefinitions     = 50
	maxServerReferences      = 1000
)

func languageServersEnabled(r *http.Request) bool {
	return r.Header.Get(languageServerHeader) == "on"
}

type navigationLocation struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"endLine"`
	EndColumn int    `json:"endColumn"`
	Kind      string `json:"kind"`
	Snippet   string `json:"snippet"`
}

type navigationResponse struct {
	Symbol    string               `json:"symbol"`
	Locations []navigationLocation `json:"locations"`
	Server    string               `json:"server"`
	Filtered  int                  `json:"filtered"`
	Truncated bool                 `json:"truncated"`
}

type navigationError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func navigationFailure(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, navigationError{Error: message, Code: code})
}

// serveNavigation answers only from LSP. Empty success means the server
// returned no eligible results, never a hidden startup or query failure.
func (a *App) serveNavigation(w http.ResponseWriter, r *http.Request, references bool, projectID, name, lineValue, columnValue string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		navigationFailure(w, status, "unavailable", message)
		return
	}
	line, lineErr := strconv.Atoi(lineValue)
	column, columnErr := strconv.Atoi(columnValue)
	if lineErr != nil || columnErr != nil || line < 1 || column < 0 || name == "" {
		navigationFailure(w, http.StatusBadRequest, "invalid", "A file, line, and column are required.")
		return
	}
	source := a.navigationSource(project.root)
	_, reason := source.readFile(name, false)
	if reason != "" {
		navigationFailure(w, http.StatusNotFound, "unavailable", reason)
		return
	}
	family := a.config.Types.Lookup(name).Type().ID
	server, supported := a.servers.ServerFor(family)
	if !supported {
		navigationFailure(w, http.StatusBadRequest, "unsupported", "No language server is configured for this language.")
		return
	}
	if !languageServersEnabled(r) {
		navigationFailure(w, http.StatusBadRequest, "disabled", "Language servers are disabled in Settings.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), navigationRequestTimeout)
	defer cancel()
	starting, cancelStart := context.WithTimeout(ctx, serverStartWait)
	err := a.servers.Start(starting, project.root, family)
	cancelStart()
	if err != nil {
		a.writeNavigationFailure(w, err, true)
		return
	}
	// Startup may take seconds. Re-scan and re-read after readiness, including
	// changes to gitignore, deletion, replacement, and the document itself.
	source = a.navigationSource(project.root)
	text, documentFile, reason := source.snapshot(name)
	if reason != "" {
		navigationFailure(w, http.StatusNotFound, "unavailable", reason)
		return
	}
	request := lsp.Request{Root: project.root, Family: family, Text: text, Document: documentFile, At: lsp.Position{Line: line, Column: column}}
	index, err := source.scan()
	if err != nil {
		navigationFailure(w, http.StatusInternalServerError, "unavailable", "Could not scan the project.")
		return
	}
	for _, nameOnDisk := range index.Files {
		if index.Ignored(nameOnDisk) {
			continue
		}
		if _, served := server.Families[a.config.Types.Lookup(nameOnDisk).Type().ID]; !served {
			continue
		}
		resolved, err := index.Resolve(nameOnDisk)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || info.Size() > document.PreviewLimit {
			continue
		}
		file := lsp.File{Path: resolved, Size: info.Size(), Modified: info.ModTime()}
		if nameOnDisk == name {
			file = documentFile
		}
		request.Files = append(request.Files, file)
	}
	if len(server.WorkspaceFiles) > 0 {
		contextFilter, err := files.NewNameFilter(server.WorkspaceFiles)
		if err != nil {
			navigationFailure(w, http.StatusInternalServerError, "unavailable", "Invalid language server workspace configuration.")
			return
		}
		contextFiles, err := files.Scan(project.root, a.config.Depth, a.config.Exclusions, contextFilter.Match)
		if err != nil {
			navigationFailure(w, http.StatusInternalServerError, "unavailable", "Could not scan workspace configuration.")
			return
		}
		seen := map[string]bool{}
		for _, file := range request.Files {
			seen[file.Path] = true
		}
		for _, name := range contextFiles.Files {
			if contextFiles.Ignored(name) {
				continue
			}
			resolved, err := contextFiles.Resolve(name)
			if err != nil || seen[resolved] {
				continue
			}
			info, err := os.Stat(resolved)
			if err != nil {
				continue
			}
			request.Files = append(request.Files, lsp.File{Path: resolved, Size: info.Size(), Modified: info.ModTime()})
			seen[resolved] = true
		}
	}
	if request.Document.Path == "" {
		navigationFailure(w, http.StatusNotFound, "unavailable", "That file is unavailable.")
		return
	}
	querying, cancelQuery := context.WithTimeout(ctx, serverRequestTimeout)
	defer cancelQuery()
	var locations []lsp.Location
	kind, limit := "definition", maxServerDefinitions
	if references {
		locations, err = a.servers.References(querying, request)
		kind, limit = "reference", maxServerReferences
	} else {
		locations, err = a.servers.Definition(querying, request)
	}
	if err != nil {
		a.writeNavigationFailure(w, err, false)
		return
	}
	response := source.locations(locations, kind, limit)
	response.Symbol, response.Server = navigationWord(text, request.At), server.Name
	if current, ok := a.servers.Status(project.root, family); ok {
		response.Server = current.Server.Name
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) writeNavigationFailure(w http.ResponseWriter, err error, starting bool) {
	switch {
	case errors.Is(err, lsp.ErrMissing):
		navigationFailure(w, http.StatusServiceUnavailable, "missing", "The language server is not installed. Open language status for installation instructions.")
	case errors.Is(err, lsp.ErrNotReady) && starting:
		navigationFailure(w, http.StatusServiceUnavailable, "starting", "The language server is still starting. Try again shortly.")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		navigationFailure(w, http.StatusServiceUnavailable, "timeout", "The language server request timed out. Try again.")
	case errors.Is(err, lsp.ErrUnsupported):
		navigationFailure(w, http.StatusBadRequest, "unsupported-method", "The language server does not support this navigation action.")
	default:
		navigationFailure(w, http.StatusServiceUnavailable, "failed", "Language server failed: "+err.Error())
	}
}

// relative rejects outside, ignored, out-of-scope and escaping symlink paths.
func (s *navigationSource) relative(absolute string) (string, bool) {
	if !filepath.IsAbs(absolute) {
		return "", false
	}
	index, err := s.scan()
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(s.root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", false
	}
	relative = filepath.ToSlash(relative)
	if _, err := index.Resolve(relative); err != nil || index.Ignored(relative) {
		return "", false
	}
	return relative, true
}

func (s *navigationSource) locations(found []lsp.Location, kind string, limit int) navigationResponse {
	response := navigationResponse{Locations: []navigationLocation{}}
	seen := map[lsp.Location]bool{}
	type target struct {
		text   []byte
		reason string
	}
	// At most one target's source stays resident. Result counts are bounded,
	// and multiple hits in a target avoid repeated filesystem reads.
	lastName := ""
	last := target{}
	for _, location := range found {
		relative, ok := s.relative(location.Path)
		if !ok {
			response.Filtered++
			continue
		}
		if relative != lastName {
			last.text, last.reason = s.readFile(relative, len(response.Locations) < limit)
			lastName = relative
		}
		if last.reason != "" || location.Line < 1 || location.Column < 0 {
			response.Filtered++
			continue
		}
		location.Path = relative
		if seen[location] {
			continue
		}
		seen[location] = true
		if len(response.Locations) == limit {
			response.Truncated = true
			continue
		}
		response.Locations = append(response.Locations, navigationLocation{Path: relative, Line: location.Line, Column: location.Column, EndLine: location.End.Line, EndColumn: location.End.Column, Kind: kind, Snippet: navigationSnippet(last.text, location.Line)})
	}
	if kind == "reference" {
		sort.SliceStable(response.Locations, func(i, j int) bool {
			left, right := response.Locations[i], response.Locations[j]
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			if left.Line != right.Line {
				return left.Line < right.Line
			}
			return left.Column < right.Column
		})
	}
	return response
}

func navigationSnippet(text []byte, line int) string {
	for line > 1 {
		index := bytes.IndexByte(text, '\n')
		if index < 0 {
			return ""
		}
		text, line = text[index+1:], line-1
	}
	if end := bytes.IndexByte(text, '\n'); end >= 0 {
		text = text[:end]
	}
	// Snippets are display hints. Keep unusually long generated lines bounded.
	text = bytes.TrimSpace(text)
	end := 0
	for count := 0; count < 240 && end < len(text); count++ {
		_, size := utf8.DecodeRune(text[end:])
		end += size
	}
	return string(text[:end])
}

// navigationWord supplies a display label only. It never resolves or ranks symbols.
func navigationWord(text []byte, at lsp.Position) string {
	for line := 1; line < at.Line; line++ {
		index := bytes.IndexByte(text, '\n')
		if index < 0 {
			return ""
		}
		text = text[index+1:]
	}
	if end := bytes.IndexByte(text, '\n'); end >= 0 {
		text = text[:end]
	}
	runes := []rune(string(text))
	index, units := 0, 0
	for index < len(runes) && units < at.Column {
		units += utf16.RuneLen(runes[index])
		index++
	}
	if units != at.Column {
		return ""
	}
	name := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' }
	start, end := index, index
	for start > 0 && name(runes[start-1]) {
		start--
	}
	for end < len(runes) && name(runes[end]) {
		end++
	}
	return string(runes[start:end])
}

type navigationStatus struct {
	Supported bool          `json:"supported"`
	Enabled   bool          `json:"enabled"`
	Eligible  bool          `json:"eligible"`
	Reason    string        `json:"reason"`
	Server    *serverStatus `json:"server"`
}

type serverStatus struct {
	Name         string            `json:"name"`
	Language     string            `json:"language"`
	State        lsp.State         `json:"state"`
	Message      string            `json:"message"`
	Install      []lsp.Install     `json:"install"`
	Docs         string            `json:"docs"`
	Memory       uint64            `json:"memory"`
	IdleMinutes  int               `json:"idleMinutes"`
	Capabilities *lsp.Capabilities `json:"capabilities"`
}

// Status and Check again discover binaries, never start processes.
func (a *App) serveNavigationStatus(w http.ResponseWriter, r *http.Request, projectID, name string, refresh bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		navigationFailure(w, status, "unavailable", message)
		return
	}
	if name == "" {
		navigationFailure(w, http.StatusBadRequest, "invalid", "A file is required.")
		return
	}
	if refresh {
		a.servers.Refresh()
	}
	_, reason := a.navigationSource(project.root).readFile(name, false)
	response := navigationStatus{Enabled: languageServersEnabled(r), Eligible: reason == "", Reason: reason}
	family := a.config.Types.Lookup(name)
	if current, ok := a.servers.Status(project.root, family.Type().ID); ok {
		response.Supported = true
		response.Server = &serverStatus{Name: current.Server.Name, Language: current.Server.Language, State: current.State, Message: current.Message, Install: current.Server.Install, Docs: current.Server.Docs, Memory: current.Memory, IdleMinutes: int(a.servers.Idle() / time.Minute), Capabilities: current.Capabilities}
	} else if response.Reason == "" {
		response.Reason = "No language server is configured for this language."
	}
	writeJSON(w, http.StatusOK, response)
}
