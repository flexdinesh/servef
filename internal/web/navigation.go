package web

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/lsp"
	"github.com/flexdinesh/servef/internal/navigation"
)

// navigationSource reads a project's navigable files through one scan, taken
// on first use: CLI scope applies, gitignored files are left out, and reads
// stop at the preview limit, past which files are not highlighted or parsed
// either. Create one per request or index build.
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

func (s *navigationSource) Files() ([]navigation.File, error) {
	index, err := s.scan()
	if err != nil {
		return nil, err
	}
	result := make([]navigation.File, 0, len(index.Files))
	for _, name := range index.Files {
		if index.Ignored(name) {
			continue
		}
		if _, ok := s.app.config.Types.Lookup(name).Navigator(); !ok {
			continue
		}
		resolved, err := index.Resolve(name)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil {
			continue
		}
		result = append(result, navigation.File{Path: name, Size: info.Size(), Modified: info.ModTime()})
	}
	return result, nil
}

func (s *navigationSource) Read(name string) ([]byte, bool, error) {
	index, err := s.scan()
	if err != nil {
		return nil, false, err
	}
	if index.Ignored(name) {
		return nil, false, nil
	}
	resolved, err := index.Resolve(name)
	if err != nil {
		return nil, false, nil
	}
	file, err := s.app.readIndexed(resolved, s.app.config.Types.Lookup(name).Type(), document.PreviewLimit)
	if err != nil || file.binary || file.tooLarge {
		return nil, false, nil
	}
	return file.source, true, nil
}

// languageServerHeader opts a request into language servers. It is a header
// rather than a query parameter so other sites cannot start servers through
// the browser: sending it cross-origin needs a CORS preflight servef never
// grants.
const languageServerHeader = "X-Servef-Language-Servers"

const (
	// serverStartWait is how long a request waits for a starting server
	// before answering by name; the server keeps starting for later ones.
	serverStartWait = 2 * time.Second
	// serverRequestTimeout bounds one query to a ready server.
	serverRequestTimeout = 10 * time.Second
	maxServerDefinitions = 50
	maxServerReferences  = 1000
)

const (
	providerServer = "language-server"
	providerSyntax = "syntax"
)

func languageServersEnabled(r *http.Request) bool {
	return r.Header.Get(languageServerHeader) == "on"
}

// navigationResponse is a navigation result and who answered it.
type navigationResponse struct {
	navigation.Result
	// Provider is providerServer or providerSyntax.
	Provider string `json:"provider"`
	// Server names the language server that answered.
	Server string `json:"server,omitempty"`
}

// serveNavigation answers /api/definition and /api/references. line is
// 1-based; column counts UTF-16 code units from 0, as the browser does.
// Requests that opt in ask the family's language server first and fall back
// to syntax navigation when it is missing, starting, failing, or points only
// outside the project.
func (a *App) serveNavigation(w http.ResponseWriter, r *http.Request, references bool, projectID, name, lineValue, columnValue string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	line, lineErr := strconv.Atoi(lineValue)
	column, columnErr := strconv.Atoi(columnValue)
	if lineErr != nil || columnErr != nil || line < 1 || column < 0 || name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "A file, line, and column are required."})
		return
	}
	source := a.navigationSource(project.root)
	at := document.Position{Line: line, Column: column}
	family := a.config.Types.Lookup(name).Type().ID
	useServers := languageServersEnabled(r)
	if useServers {
		if response, ok := a.serverNavigation(r.Context(), references, project.root, source, name, family, at); ok {
			writeJSON(w, http.StatusOK, response)
			return
		}
	}
	// While a server stands by, scan per request instead of building a
	// syntax index the server makes unnecessary.
	build := !useServers || !a.servers.Available(project.root, family)
	var result navigation.Result
	var err error
	if references {
		result, err = a.navigation.References(r.Context(), project.root, source, name, at, build)
	} else {
		result, err = a.navigation.Definition(project.root, source, name, at, build)
	}
	switch {
	case errors.Is(err, navigation.ErrNotNavigable):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: a.config.Types.Lookup(name).Type().Label + " files do not support code navigation."})
	case errors.Is(err, navigation.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "That file is unavailable."})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusOK, navigationResponse{Result: result, Provider: providerSyntax})
	}
}

// serverNavigation asks the family's language server. It reports false
// when syntax navigation should answer instead.
func (a *App) serverNavigation(ctx context.Context, references bool, root string, source *navigationSource, name, family string, at document.Position) (navigationResponse, bool) {
	server, ok := a.servers.ServerFor(family)
	if !ok || !a.servers.Available(root, family) {
		return navigationResponse{}, false
	}
	listed, err := source.Files()
	if err != nil {
		return navigationResponse{}, false
	}
	text, readable, err := source.Read(name)
	if err != nil || !readable {
		return navigationResponse{}, false
	}
	request := lsp.Request{Root: root, Family: family, Text: text, At: lsp.Position{Line: at.Line, Column: at.Column}}
	found := false
	for _, file := range listed {
		if _, served := server.Families[a.config.Types.Lookup(file.Path).Type().ID]; !served {
			continue
		}
		absolute, ok := source.absolute(file.Path)
		if !ok {
			continue
		}
		stamped := lsp.File{Path: absolute, Size: file.Size, Modified: file.Modified}
		request.Files = append(request.Files, stamped)
		if file.Path == name {
			request.Document, found = stamped, true
		}
	}
	if !found {
		return navigationResponse{}, false
	}

	starting, cancel := context.WithTimeout(ctx, serverStartWait)
	err = a.servers.Start(starting, root, family)
	cancel()
	if err != nil {
		return navigationResponse{}, false
	}
	querying, cancel := context.WithTimeout(ctx, serverRequestTimeout)
	defer cancel()
	var locations []lsp.Location
	kind, limit := "definition", maxServerDefinitions
	if references {
		locations, err = a.servers.References(querying, request)
		kind, limit = "reference", maxServerReferences
	} else {
		locations, err = a.servers.Definition(querying, request)
	}
	if err != nil {
		return navigationResponse{}, false
	}
	// Servers do not mark the declaration among references; ask for it so
	// results label it as syntax navigation does.
	declared := map[lsp.Location]bool{}
	if references {
		definitions, _ := a.servers.Definition(querying, request)
		for _, definition := range definitions {
			declared[definition] = true
		}
	}
	places := make([]navigation.Place, 0, len(locations))
	seen := map[navigation.Place]bool{}
	for _, location := range locations {
		relative, ok := source.relative(location.Path)
		if !ok {
			continue // Outside the project, ignored, or out of scope.
		}
		place := navigation.Place{Path: relative, Position: document.Position{Line: location.Line, Column: location.Column}, Kind: kind}
		if declared[location] {
			place.Kind = "definition"
		}
		if !seen[place] {
			seen[place] = true
			places = append(places, place)
		}
	}
	if !references && len(locations) > 0 && len(places) == 0 {
		// Defined outside the project; syntax navigation lands on the import.
		return navigationResponse{}, false
	}
	// The requested file first, then path and position order, as syntax
	// navigation answers.
	sort.SliceStable(places, func(i, j int) bool {
		left, right := places[i], places[j]
		if (left.Path == name) != (right.Path == name) {
			return left.Path == name
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.Position.Before(right.Position)
	})
	places = places[:min(len(places), limit)]
	return navigationResponse{
		Result:   navigation.Result{Symbol: navigation.WordAt(text, at), Locations: navigation.Locate(source, places)},
		Provider: providerServer,
		Server:   server.Name,
	}, true
}

// absolute resolves an indexed project path to its file on disk.
func (s *navigationSource) absolute(name string) (string, bool) {
	index, err := s.scan()
	if err != nil {
		return "", false
	}
	resolved, err := index.Resolve(name)
	return resolved, err == nil
}

// relative maps a path from a language server to an indexed, in-scope,
// non-ignored project path.
func (s *navigationSource) relative(absolute string) (string, bool) {
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

// navigationStatus tells the UI who answers navigation for a file.
type navigationStatus struct {
	Navigable bool `json:"navigable"`
	// LanguageServers reports the request's opt-in.
	LanguageServers bool `json:"languageServers"`
	// Provider is providerServer, providerSyntax, or empty when the file
	// is not navigable.
	Provider string        `json:"provider"`
	Server   *serverStatus `json:"server"`
}

type serverStatus struct {
	Name     string        `json:"name"`
	Language string        `json:"language"`
	State    lsp.State     `json:"state"`
	Message  string        `json:"message"`
	Install  []lsp.Install `json:"install"`
	Docs     string        `json:"docs"`
	// Memory is the running server's resident memory in bytes; 0 when
	// stopped or unknown.
	Memory      uint64 `json:"memory"`
	IdleMinutes int    `json:"idleMinutes"`
}

// serveNavigationStatus answers /api/navigation-status without starting
// servers. refresh looks for newly installed servers and retries failed
// ones.
func (a *App) serveNavigationStatus(w http.ResponseWriter, r *http.Request, projectID, name string, refresh bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "A file is required."})
		return
	}
	if refresh {
		a.servers.Refresh()
	}
	family := a.config.Types.Lookup(name)
	enabled := languageServersEnabled(r)
	response := navigationStatus{LanguageServers: enabled}
	if _, ok := family.Navigator(); !ok {
		writeJSON(w, http.StatusOK, response)
		return
	}
	response.Navigable, response.Provider = true, providerSyntax
	if current, ok := a.servers.Status(project.root, family.Type().ID); ok {
		response.Server = &serverStatus{
			Name:        current.Server.Name,
			Language:    current.Server.Language,
			State:       current.State,
			Message:     current.Message,
			Install:     current.Server.Install,
			Docs:        current.Server.Docs,
			Memory:      current.Memory,
			IdleMinutes: int(a.servers.Idle() / time.Minute),
		}
		if enabled && current.State != lsp.StateMissing && current.State != lsp.StateFailed {
			response.Provider = providerServer
		}
	}
	writeJSON(w, http.StatusOK, response)
}
