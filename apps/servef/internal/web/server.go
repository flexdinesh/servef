// Package web serves an indexed local file workspace and document views.
package web

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"github.com/flexdinesh/servef/internal/app"
	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/lsp"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/watch"
	"github.com/flexdinesh/servef/pkg/contract"
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

// App adapts the application layer (internal/app) to the HTTP API.
type App struct {
	config Config
	// app holds the domain services (decision 0080).
	app *app.App
	// roots are the canonical served roots; the browser keys stored state by
	// them. Each request takes one List snapshot.
	roots    *projects.RootSet
	openFile func(string) (fs.File, error)
	shell    string
	servers  *lsp.Manager
	watches  *watch.Hub
	routes   *http.ServeMux
}

// errorResponse is every JSON error body (contract.ErrorResponse). Code,
// when set, names a failure the browser handles specially.
type errorResponse = contract.ErrorResponse

const (
	noProjectMessage          = app.NoProjectMessage
	projectUnavailableMessage = app.ProjectUnavailableMessage
	insideRepositoryMessage   = app.InsideRepositoryMessage
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
	if config.LiveUpdates == nil {
		config.LiveUpdates = watch.NewHub(watch.DefaultOptions())
	}
	a := &App{config: config, roots: config.Roots, openFile: openFile, shell: string(shell), servers: config.LanguageServers, watches: config.LiveUpdates}
	a.app, err = app.New(app.Config{
		Roots:           config.Roots,
		ScanDepth:       config.ScanDepth,
		Depth:           config.Depth,
		Types:           config.Types,
		LanguageServers: config.LanguageServers,
		SessionKey:      config.SessionKey,
		// Reads go through the App's openFile, which tests replace.
		OpenFile:             func(name string) (fs.File, error) { return a.openFile(name) },
		Watches:              config.LiveUpdates,
		WalkthroughDirectory: config.WalkthroughDirectory,
	})
	if err != nil {
		return nil, err
	}
	a.routes = a.newRoutes()
	return a, nil
}

// Application is the application layer this App adapts, for in-process
// clients (pkg/client).
func (a *App) Application() *app.App { return a.app }

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
	a.app.Walkthroughs.CloseNotices()
}

func openFile(name string) (fs.File, error) { return os.Open(name) }

// writeError writes err as an errorResponse with its kind's status.
func writeError(w http.ResponseWriter, err error) {
	failure := app.AsError(err)
	writeJSON(w, contract.HTTPStatus(failure.Kind), failure.Response())
}

// statusOf is the status an operation's outcome answers: 200, or err's kind.
func statusOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	return contract.HTTPStatus(app.AsError(err).Kind)
}
