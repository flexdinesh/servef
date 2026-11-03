// Package web serves an indexed local file workspace and document views.
package web

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/lsp"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/walkthrough"
	"github.com/flexdinesh/servef/internal/watch"
)

// Config controls project discovery and the live file scan performed for each
// request. Roots confine every project; ScanDepth bounds repository discovery;
// Depth bounds the file scan within a project.
type Config struct {
	// Roots is required; requests read it once each, so it may grow while
	// serving.
	Roots     *projects.RootSet
	ScanDepth int
	Depth     int
	// Types decides presentation; every file within Depth is in scope.
	Types *document.Registry
	// LanguageServers runs language servers for requests that opt in; nil
	// uses the built-in servers for Types' text families. The App
	// closes it.
	LanguageServers *lsp.Manager
	// LiveUpdates follows the projects browsers show (decision 0050); nil
	// uses watch.DefaultOptions. The App closes it.
	LiveUpdates *watch.Hub
	// WalkthroughDirectory stores published walkthroughs; empty uses
	// walkthrough.DefaultDirectory.
	WalkthroughDirectory string
	// OpenBrowser opens a published walkthrough's URL when it asks to be
	// opened; nil never opens one.
	OpenBrowser func(url string) error
	// SessionKey names the session whose browser memory a page reads
	// (/api/projects sessionKey); empty uses the roots joined by newlines.
	SessionKey string
}

// App composes discovery and document handlers into the HTTP API.
type App struct {
	config Config
	// roots are the canonical served roots; the browser keys stored state by
	// them. Each request takes one List snapshot.
	roots    *projects.RootSet
	metrics  metricsCollector
	openFile func(string) (fs.File, error)
	// statsBudget bounds the bytes one compare-stats request reads.
	statsBudget  int64
	shell        string
	servers      *lsp.Manager
	watches      *watch.Hub
	walkthroughs *walkthrough.Service
	// notices fans walkthrough announcements out (decision 0066).
	notices noticeHub
	routes  *http.ServeMux
}

// errorResponse is every JSON error body. Code, when set, names a failure
// the browser handles specially.
type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// workspace is the project directory a request is scoped to.
type workspace struct {
	id   string
	root string
}

const (
	noProjectMessage          = "No project was selected."
	projectUnavailableMessage = "Project unavailable."
	insideRepositoryMessage   = "Project unavailable: folders inside a repository open with the repository."
)

// New constructs a request-time scanning web interface.
func New(config Config) (*App, error) {
	shell, err := fs.ReadFile(assets, "dist/index.html")
	if err != nil {
		return nil, err
	}

	if config.Types == nil {
		config.Types = document.Builtins()
	}
	if config.Roots == nil {
		return nil, errors.New("web: no roots")
	}
	if config.LanguageServers == nil {
		config.LanguageServers = lsp.NewManager(builtinServers(config.Types), lsp.Options{})
	}
	if err := checkLanguageServers(config.LanguageServers.Servers(), config.Types); err != nil {
		return nil, err
	}
	if config.WalkthroughDirectory == "" {
		config.WalkthroughDirectory, err = walkthrough.DefaultDirectory()
		if err != nil {
			return nil, err
		}
	}
	if config.LiveUpdates == nil {
		config.LiveUpdates = watch.NewHub(watch.DefaultOptions())
	}
	app := &App{config: config, roots: config.Roots, openFile: openFile, statsBudget: compareStatsBudget, shell: string(shell), servers: config.LanguageServers, watches: config.LiveUpdates}
	app.walkthroughs = walkthrough.NewService(walkthrough.NewStore(config.WalkthroughDirectory), walkthroughTargets{roots: config.Roots, depth: config.Depth, types: config.Types, openFile: func(name string) (fs.File, error) { return app.openFile(name) }})
	app.routes = app.newRoutes()
	return app, nil
}

// builtinServers intersects each built-in server with the registered text families.
func builtinServers(types *document.Registry) []lsp.Server {
	text := map[string]bool{}
	for _, family := range types.Types() {
		text[family.ID] = family.HasTextSource()
	}
	var servers []lsp.Server
	for _, server := range lsp.Builtins() {
		families := map[string]string{}
		for family, language := range server.Families {
			if text[family] {
				families[family] = language
			}
		}
		if len(families) > 0 {
			server.Families = families
			servers = append(servers, server)
		}
	}
	return servers
}

// checkLanguageServers rejects unknown, non-text, and ambiguously served families.
func checkLanguageServers(servers []lsp.Server, types *document.Registry) error {
	text := map[string]bool{}
	for _, family := range types.Types() {
		text[family.ID] = family.HasTextSource()
	}
	covered := map[string]string{}
	for _, server := range servers {
		if len(server.WorkspaceFiles) > 0 {
			if _, err := files.NewNameFilter(server.WorkspaceFiles); err != nil {
				return fmt.Errorf("language server %s workspace files: %w", server.Name, err)
			}
		}
		for family, language := range server.Families {
			if !text[family] {
				return fmt.Errorf("language server %s serves %q, which is not a registered text family", server.Name, family)
			}
			if language == "" {
				return fmt.Errorf("language server %s has no language ID for %q", server.Name, family)
			}
			if previous, ok := covered[family]; ok {
				return fmt.Errorf("language servers %s and %s both serve %q", previous, server.Name, family)
			}
			covered[family] = server.Name
		}
	}
	return nil
}

// Close ends event streams and stops running language servers.
func (a *App) Close() {
	a.StopLiveUpdates()
	a.servers.Close()
}

// StopLiveUpdates ends every event stream and walkthrough notice
// subscription and stops watching, so a server shutdown need not wait for
// streams that never end on their own.
func (a *App) StopLiveUpdates() {
	a.watches.Close()
	a.notices.close()
}

func openFile(name string) (fs.File, error) { return os.Open(name) }

// project resolves a request's project ID within a snapshot of the roots.
func (a *App) project(id string) (workspace, int, string) {
	return projectIn(a.roots.List(), id)
}

// projectIn resolves a project ID within roots.
func projectIn(roots []string, id string) (workspace, int, string) {
	if id == "" {
		return workspace{}, http.StatusBadRequest, noProjectMessage
	}
	root, err := projects.Resolve(roots, id)
	if errors.Is(err, files.ErrPathEscape) {
		return workspace{}, http.StatusBadRequest, projectUnavailableMessage
	}
	if errors.Is(err, projects.ErrInsideRepository) {
		return workspace{}, http.StatusNotFound, insideRepositoryMessage
	}
	if err != nil {
		return workspace{}, http.StatusNotFound, projectUnavailableMessage
	}
	return workspace{id: id, root: root}, http.StatusOK, ""
}

func (a *App) scan(root string) (*files.Index, error) {
	return files.Scan(root, a.config.Depth, func(string) bool { return true })
}

func warningStrings(warnings []files.Warning) []string {
	result := make([]string, len(warnings))
	for i, warning := range warnings {
		result[i] = warning.Error()
	}
	return result
}
