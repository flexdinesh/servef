package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/lsp"
	"github.com/flexdinesh/servef/internal/lsp/lsptest"
	"github.com/flexdinesh/servef/internal/navigation"
)

// The test binary doubles as a fake language server.
func TestMain(m *testing.M) {
	if lsptest.Serving() {
		lsptest.Serve()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

var fakeServer = lsp.Server{
	Name:     "fake",
	Language: "Go",
	Families: map[string]string{"go": "go"},
	Install:  []lsp.Install{{Label: "Go", Command: "go install fake@latest"}},
	Docs:     "https://example.com/fake",
}

// serverApp serves a Go project through the fake server. installed
// decides whether the server is found.
func serverApp(t *testing.T, installed bool) *App {
	t.Helper()
	t.Setenv(lsptest.Env, "1")
	root := t.TempDir()
	for name, content := range map[string]string{
		".gitignore":       "generated/\n",
		"main.go":          "package app\n\nfunc main() { Build() }\n",
		"target.go":        "package app\n\n// Build is the target.\n\nfunc Build() {}\n",
		"generated/gen.go": "package app\n\nfunc Generated() {}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	servers := lsp.NewManager([]lsp.Server{fakeServer}, lsp.Options{
		LookPath: func(string) (string, bool) { return os.Args[0], installed },
	})
	app, err := New(Config{Roots: []string{root}, Depth: 5, LanguageServers: servers})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return app
}

func serverRequest(t *testing.T, app *App, target string, enabled bool) *httptest.ResponseRecorder {
	t.Helper()
	target = rootProject.ReplaceAllString(target, "project="+url.QueryEscape(app.roots[0])+"$1")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if enabled {
		req.Header.Set(languageServerHeader, "on")
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("%s status = %d: %s", target, response.Code, response.Body.String())
	}
	return response
}

func serverNavigate(t *testing.T, app *App, target string, enabled bool) navigationResponse {
	t.Helper()
	var result navigationResponse
	if err := json.Unmarshal(serverRequest(t, app, target, enabled).Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func requestStatus(t *testing.T, app *App, target string, enabled bool) navigationStatus {
	t.Helper()
	var status navigationStatus
	if err := json.Unmarshal(serverRequest(t, app, target, enabled).Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	return status
}

const (
	definitionTarget = "/api/definition?project=.&path=main.go&line=3&column=15"
	referencesTarget = "/api/references?project=.&path=main.go&line=3&column=15"
	statusTarget     = "/api/navigation-status?project=.&path=main.go"
)

func TestLanguageServersAnswerOnlyWhenEnabled(t *testing.T) {
	app := serverApp(t, true)

	off := serverNavigate(t, app, definitionTarget, false)
	if off.Provider != providerSyntax || len(off.Locations) != 1 || off.Locations[0].Line != 5 {
		t.Fatalf("disabled = %+v, want syntax navigation", off)
	}
	if status := requestStatus(t, app, statusTarget, false); status.Server == nil || status.Server.State != lsp.StateStopped {
		t.Fatalf("navigating without servers started one: %+v", status.Server)
	}

	on := serverNavigate(t, app, definitionTarget, true)
	want := navigation.Location{Path: "target.go", Line: 5, Column: 2, Kind: "definition", Snippet: "func Build() {}"}
	if on.Provider != providerServer || on.Server != "fake" || on.Symbol != "Build" || len(on.Locations) != 1 || on.Locations[0] != want {
		t.Fatalf("enabled = %+v, want %+v from the server", on, want)
	}

	// Locations outside the project are dropped.
	references := serverNavigate(t, app, referencesTarget, true)
	if references.Provider != providerServer || len(references.Locations) != 1 || references.Locations[0].Path != "main.go" || references.Locations[0].Kind != "reference" {
		t.Fatalf("references = %+v", references)
	}
	// The declaration among references is labeled as the definition.
	declaration := serverNavigate(t, app, "/api/references?project=.&path=target.go&line=5&column=2", true)
	if len(declaration.Locations) != 1 || declaration.Locations[0].Path != "target.go" || declaration.Locations[0].Kind != "definition" {
		t.Fatalf("references at the declaration = %+v", declaration)
	}

	status := requestStatus(t, app, statusTarget, true)
	if !status.Navigable || !status.LanguageServers || status.Provider != providerServer || status.Server == nil || status.Server.State != lsp.StateReady || status.Server.IdleMinutes != 10 {
		t.Fatalf("status = %+v %+v", status, status.Server)
	}
}

func TestLanguageServerDefinitionsOutsideTheProjectFallBack(t *testing.T) {
	t.Setenv("SERVEF_FAKE_LSP_TARGET", "generated/gen.go")
	app := serverApp(t, true)
	result := serverNavigate(t, app, definitionTarget, true)
	if result.Provider != providerSyntax || len(result.Locations) != 1 || result.Locations[0].Path != "target.go" {
		t.Fatalf("definition in a gitignored file = %+v, want syntax navigation", result)
	}
}

func TestMissingLanguageServersExplainHowToInstall(t *testing.T) {
	app := serverApp(t, false)
	status := requestStatus(t, app, statusTarget, true)
	if status.Provider != providerSyntax || status.Server == nil || status.Server.State != lsp.StateMissing || len(status.Server.Install) != 1 || status.Server.Docs == "" {
		t.Fatalf("status = %+v %+v", status, status.Server)
	}
	if result := serverNavigate(t, app, definitionTarget, true); result.Provider != providerSyntax || len(result.Locations) != 1 {
		t.Fatalf("definition = %+v, want syntax navigation", result)
	}
}

func TestCrashedLanguageServersFallBackAndReport(t *testing.T) {
	t.Setenv("SERVEF_FAKE_LSP_CRASH", "1")
	app := serverApp(t, true)
	if result := serverNavigate(t, app, definitionTarget, true); result.Provider != providerSyntax || len(result.Locations) != 1 {
		t.Fatalf("definition = %+v, want syntax navigation", result)
	}
	status := requestStatus(t, app, statusTarget, true)
	if status.Provider != providerSyntax || status.Server == nil || status.Server.State != lsp.StateFailed || !strings.Contains(status.Server.Message, "fake server crashed") {
		t.Fatalf("status = %+v %+v", status, status.Server)
	}
	// Check again retries the server.
	if status := requestStatus(t, app, statusTarget+"&refresh=1", true); status.Server == nil || status.Server.State != lsp.StateStopped {
		t.Fatalf("after refresh = %+v", status.Server)
	}
}

func TestStartingLanguageServersDoNotHoldRequests(t *testing.T) {
	t.Setenv("SERVEF_FAKE_LSP_SLOW", "5s")
	app := serverApp(t, true)
	started := time.Now()
	result := serverNavigate(t, app, definitionTarget, true)
	if result.Provider != providerSyntax || len(result.Locations) != 1 {
		t.Fatalf("definition = %+v, want syntax navigation", result)
	}
	if elapsed := time.Since(started); elapsed > serverStartWait+time.Second {
		t.Fatalf("waited %s for a starting server", elapsed)
	}
	if status := requestStatus(t, app, statusTarget, true); status.Server == nil || status.Server.State != lsp.StateStarting || status.Provider != providerServer {
		t.Fatalf("status = %+v %+v", status, status.Server)
	}
}

func TestNavigationStatusForFilesWithoutNavigation(t *testing.T) {
	app := serverApp(t, true)
	if err := os.WriteFile(filepath.Join(app.roots[0], "README.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status := requestStatus(t, app, "/api/navigation-status?project=.&path=README.md", true)
	if status.Navigable || status.Provider != "" || status.Server != nil {
		t.Fatalf("status = %+v", status)
	}
}
