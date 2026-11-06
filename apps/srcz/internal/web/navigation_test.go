package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/flexdinesh/srcz/internal/document"
	"github.com/flexdinesh/srcz/internal/lsp"
	"github.com/flexdinesh/srcz/pkg/contract"
)

func TestNavigationRejectsBadRequests(t *testing.T) {
	app := serverApp(t, true)
	for _, target := range []string{
		"/api/definition?project=.&path=main.go&line=0&column=0",
		"/api/definition?project=.&path=main.go&line=x&column=0",
		"/api/definition?project=.&path=main.go&line=1&column=-1",
	} {
		navigationErrorCode(t, app, target, true, http.StatusBadRequest, "invalid")
	}
	for _, target := range []string{
		"/api/definition?project=.&path=missing.go&line=1&column=0",
		"/api/definition?project=.&path=..%2Fsecret.go&line=1&column=0",
		"/api/definition?project=missing&path=a.go&line=1&column=0",
	} {
		navigationErrorCode(t, app, target, true, http.StatusNotFound, "unavailable")
	}
}

func TestPageNavigationSupportUsesCatalog(t *testing.T) {
	app := serverApp(t, false)
	if err := os.WriteFile(filepath.Join(app.roots.List()[0], "plain.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{"main.go": true, "plain.txt": false, "": false} {
		_, page := requestPageData(t, app, "/api/page?project=.&path="+path)
		if page.NavigationSupported != want {
			t.Fatalf("%s supported = %v", path, page.NavigationSupported)
		}
	}
}

func TestLanguageServerCompositionUsesTextFamilies(t *testing.T) {
	registry, err := document.NewRegistry(document.CodeHandler{Definition: document.Type{ID: "text", Label: "Plain text", Language: "text", DefaultMode: "source"}}, document.CodeHandler{Definition: document.Type{ID: "typescript", Label: "TypeScript", Language: "ts", Extensions: []string{".ts"}, DefaultMode: "source"}})
	if err != nil {
		t.Fatal(err)
	}
	servers := builtinServers(registry)
	var found bool
	for _, server := range servers {
		if _, ok := server.Families["typescript"]; ok {
			found = true
			if len(server.Families) != 1 {
				t.Fatalf("custom registry retained unregistered families: %+v", server)
			}
		}
	}
	if !found {
		t.Fatal("TypeScript-only registry lost its server")
	}
	if err := checkLanguageServers([]lsp.Server{fakeServer}, registry); err == nil {
		t.Fatal("unknown family accepted")
	}
	if err := checkLanguageServers([]lsp.Server{fakeServer, fakeServer}, document.Builtins()); err == nil {
		t.Fatal("duplicate coverage accepted")
	}
}

func TestNavigationStatusFixtureMatchesServer(t *testing.T) {
	servers := lsp.NewManager(lsp.Builtins(), lsp.Options{LookPath: func(string) (string, bool) { return "", false }})
	app, err := New(Config{Roots: testRoots(t, filepath.Join("..", "..", "..", "..", "testdata", "markdown")), Depth: 5, LanguageServers: servers})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	var fixture contract.NavigationStatus
	readFixture(t, "navigation-status.json", &fixture)
	served := requestStatus(t, app, "/api/navigation-status?project=.&path=navigation%2Fgreet.go", true)
	// The checked-in browser fixture is portable; compare its transport shape
	// while testing host-specific installation choices in the lsp package.
	if fixture.Server != nil && served.Server != nil {
		fixture.Server.Install = served.Server.Install
	}
	if !reflect.DeepEqual(fixture, served) {
		t.Errorf("fixture = %+v, server = %+v", fixture, served)
	}
}

func readFixture(t *testing.T, name string, value any) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "testdata", "api", "default", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, value); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}
