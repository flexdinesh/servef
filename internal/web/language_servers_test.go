package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/lsp"
	"github.com/flexdinesh/servef/internal/lsp/lsptest"
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
	req := localRequest(http.MethodGet, target)
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

func navigationErrorCode(t *testing.T, app *App, target string, enabled bool, wantStatus int, wantCode string) {
	t.Helper()
	target = rootProject.ReplaceAllString(target, "project="+url.QueryEscape(app.roots[0])+"$1")
	req := localRequest(http.MethodGet, target)
	if enabled {
		req.Header.Set(languageServerHeader, "on")
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, req)
	var result navigationError
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != wantStatus || result.Code != wantCode || result.Error == "" {
		t.Fatalf("%s: %d %+v", target, response.Code, result)
	}
}

func TestLanguageServersAnswerOnlyWhenEnabled(t *testing.T) {
	app := serverApp(t, true)
	navigationErrorCode(t, app, definitionTarget, false, http.StatusBadRequest, "disabled")
	if status := requestStatus(t, app, statusTarget, false); !status.Supported || status.Enabled || !status.Eligible || status.Server == nil || status.Server.State != lsp.StateStopped || status.Server.Capabilities != nil {
		t.Fatalf("disabled request started a server: %+v %+v", status, status.Server)
	}
	on := serverNavigate(t, app, definitionTarget, true)
	if on.Server != "fake" || on.Symbol != "Build" || len(on.Locations) != 1 || on.Locations[0].Path != "target.go" || on.Locations[0].Line != 5 || on.Locations[0].Snippet != "func Build() {}" || on.Filtered != 0 {
		t.Fatalf("definition = %+v", on)
	}
	references := serverNavigate(t, app, referencesTarget, true)
	if len(references.Locations) != 1 || references.Locations[0].Path != "main.go" || references.Locations[0].Kind != "reference" || references.Filtered != 1 {
		t.Fatalf("references = %+v", references)
	}
	status := requestStatus(t, app, statusTarget, true)
	if !status.Supported || !status.Enabled || !status.Eligible || status.Server == nil || status.Server.State != lsp.StateReady || status.Server.IdleMinutes != 10 || status.Server.Capabilities == nil || !status.Server.Capabilities.Definition || !status.Server.Capabilities.References {
		t.Fatalf("status = %+v %+v", status, status.Server)
	}
}

func TestLanguageServerDefinitionsOutsideScopeRemainFiltered(t *testing.T) {
	for _, target := range []string{"generated/gen.go", "../outside.go"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv("SERVEF_FAKE_LSP_TARGET", target)
			app := serverApp(t, true)
			result := serverNavigate(t, app, definitionTarget, true)
			if len(result.Locations) != 0 || result.Filtered != 1 || result.Server != "fake" {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestMissingLanguageServersExplainHowToInstall(t *testing.T) {
	app := serverApp(t, false)
	status := requestStatus(t, app, statusTarget, true)
	if !status.Supported || !status.Eligible || status.Server == nil || status.Server.State != lsp.StateMissing || len(status.Server.Install) != 1 || status.Server.Docs == "" {
		t.Fatalf("status = %+v %+v", status, status.Server)
	}
	navigationErrorCode(t, app, definitionTarget, true, http.StatusServiceUnavailable, "missing")
}

func TestCrashedLanguageServersReportFailureWithoutFallback(t *testing.T) {
	t.Setenv("SERVEF_FAKE_LSP_CRASH", "1")
	app := serverApp(t, true)
	navigationErrorCode(t, app, definitionTarget, true, http.StatusServiceUnavailable, "failed")
	status := requestStatus(t, app, statusTarget, true)
	if status.Server == nil || status.Server.State != lsp.StateFailed || !strings.Contains(status.Server.Message, "fake server crashed") {
		t.Fatalf("status = %+v %+v", status, status.Server)
	}
	if status := requestStatus(t, app, statusTarget+"&refresh=1", true); status.Server == nil || status.Server.State != lsp.StateStopped {
		t.Fatalf("after refresh = %+v", status.Server)
	}
}

func TestNavigationStatusForUnsupportedAndIneligibleFiles(t *testing.T) {
	app := serverApp(t, true)
	for name, content := range map[string]string{"notes.txt": "hello", "binary.go": "abc\x00def", "large.go": strings.Repeat("a", (4<<20)+1)} {
		if err := os.WriteFile(filepath.Join(app.roots[0], name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	unsupported := requestStatus(t, app, "/api/navigation-status?project=.&path=notes.txt", true)
	if unsupported.Supported || !unsupported.Eligible || unsupported.Reason == "" || unsupported.Server != nil {
		t.Fatalf("status = %+v", unsupported)
	}
	for _, name := range []string{"binary.go", "large.go", "generated/gen.go", "missing.go"} {
		status := requestStatus(t, app, "/api/navigation-status?project=.&path="+url.QueryEscape(name), true)
		if !status.Supported || status.Eligible || status.Reason == "" || status.Server == nil || status.Server.State != lsp.StateStopped {
			t.Fatalf("%s status = %+v %+v", name, status, status.Server)
		}
		navigationErrorCode(t, app, "/api/definition?project=.&path="+url.QueryEscape(name)+"&line=1&column=0", true, http.StatusNotFound, "unavailable")
	}
	navigationErrorCode(t, app, "/api/definition?project=.&path=notes.txt&line=1&column=0", true, http.StatusBadRequest, "unsupported")
}

func TestUnsupportedLanguageServerMethodIsExplicit(t *testing.T) {
	t.Setenv("SERVEF_FAKE_LSP_UNSUPPORTED", "1")
	app := serverApp(t, true)
	navigationErrorCode(t, app, definitionTarget, true, http.StatusBadRequest, "unsupported-method")
	status := requestStatus(t, app, statusTarget, true)
	if status.Server == nil || status.Server.Capabilities == nil || status.Server.Capabilities.Definition || status.Server.Capabilities.References {
		t.Fatalf("unsupported capabilities = %+v", status.Server)
	}
}

func TestNavigationCancellationExplainsStartingAndQueryTimeout(t *testing.T) {
	for _, test := range []struct{ name, env, code string }{
		{"starting", "SERVEF_FAKE_LSP_SLOW", "starting"},
		{"query", "SERVEF_FAKE_LSP_QUERY_SLOW", "timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.env, "500ms")
			app := serverApp(t, true)
			target := rootProject.ReplaceAllString(definitionTarget, "project="+url.QueryEscape(app.roots[0])+"$1")
			req := localRequest(http.MethodGet, target)
			req.Header.Set(languageServerHeader, "on")
			ctx, cancel := context.WithTimeout(req.Context(), 100*time.Millisecond)
			defer cancel()
			response := httptest.NewRecorder()
			app.ServeHTTP(response, req.WithContext(ctx))
			var result navigationError
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusServiceUnavailable || result.Code != test.code {
				t.Fatalf("response = %d %+v", response.Code, result)
			}
		})
	}
}

func TestNavigationRefreshNeverStartsServer(t *testing.T) {
	app := serverApp(t, true)
	for _, enabled := range []bool{false, true} {
		status := requestStatus(t, app, statusTarget+"&refresh=1", enabled)
		if status.Server == nil || status.Server.State != lsp.StateStopped || status.Server.Capabilities != nil {
			t.Fatalf("refresh started server: %+v", status.Server)
		}
	}
}

func TestWorkspaceConfigurationChangesOutsideBrowseScopeNotifyServer(t *testing.T) {
	app := serverApp(t, true)
	filter, err := files.NewNameFilter([]string{"*.go"})
	if err != nil {
		t.Fatal(err)
	}
	app.config.Filenames = filter
	server := fakeServer
	server.WorkspaceFiles = []string{"go.mod"}
	app.servers.Close()
	app.servers = lsp.NewManager([]lsp.Server{server}, lsp.Options{LookPath: func(string) (string, bool) { return os.Args[0], true }})
	logPath := filepath.Join(t.TempDir(), "notifications.log")
	t.Setenv("SERVEF_FAKE_LSP_LOG", logPath)
	configPath := filepath.Join(app.roots[0], "go.mod")
	if err := os.WriteFile(configPath, []byte("module example.com/original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	serverNavigate(t, app, definitionTarget, true)
	if err := os.WriteFile(configPath, []byte("module example.com/changed-longer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	serverNavigate(t, app, definitionTarget, true)
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "watched go.mod 2") {
		t.Fatalf("config change not sent: %s", content)
	}
	response, _ := requestPageData(t, app, "/api/page?project=.&path=go.mod")
	if response.Code != http.StatusNotFound {
		t.Fatalf("config outside browse scope became accessible: %d", response.Code)
	}
}

func TestNavigationSyncsFreshDocumentAfterSlowStartup(t *testing.T) {
	t.Setenv("SERVEF_FAKE_LSP_SLOW", "400ms")
	app := serverApp(t, true)
	logPath := filepath.Join(t.TempDir(), "notifications.log")
	t.Setenv("SERVEF_FAKE_LSP_LOG", logPath)
	target := rootProject.ReplaceAllString(definitionTarget, "project="+url.QueryEscape(app.roots[0])+"$1")
	req := localRequest(http.MethodGet, target)
	req.Header.Set(languageServerHeader, "on")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { app.ServeHTTP(response, req); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, ok := app.servers.Status(app.roots[0], "go")
		if ok && status.State == lsp.StateStarting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not enter startup")
		}
		time.Sleep(time.Millisecond)
	}
	updated := "package app\n\nfunc main() { ChangedAfterStartup() }\n"
	if err := os.WriteFile(filepath.Join(app.roots[0], "main.go"), []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	<-done
	if response.Code != http.StatusOK {
		t.Fatalf("first response = %d: %s", response.Code, response.Body.String())
	}
	serverNavigate(t, app, definitionTarget, true)
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var synced []string
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(line, "open ") {
			continue
		}
		var opened struct {
			TextDocument struct {
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "open ")), &opened); err != nil {
			t.Fatal(err)
		}
		synced = append(synced, opened.TextDocument.Text)
	}
	if len(synced) != 1 || synced[0] != updated {
		t.Fatalf("synced snapshots = %q, want newest contents %q", synced, updated)
	}
	if strings.Contains(string(content), "textDocument/didChange main.go") {
		t.Fatalf("unchanged second request required correction: %s", content)
	}
}
