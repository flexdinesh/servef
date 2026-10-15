package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/navigation"
)

// navigate requests a navigation endpoint, waiting out the first index
// build.
func navigate(t *testing.T, app *App, target string) navigation.Result {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		response := request(t, app, target)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d: %s", target, response.Code, response.Body.String())
		}
		var result navigation.Result
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if !result.Indexing {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s still indexing", target)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNavigationResolvesAcrossSampleFiles(t *testing.T) {
	app := newTestApp(t, filepath.Join("..", "..", "testdata", "markdown"))

	definition := navigate(t, app, "/api/definition?project=.&path=navigation%2Fgreet.go&line=6&column=8")
	want := []navigation.Location{{Path: "navigation/format.go", Line: 6, Column: 5, Kind: "function", Snippet: "func Format(words ...string) string {"}}
	if definition.Symbol != "Format" || !reflect.DeepEqual(definition.Locations, want) {
		t.Fatalf("definition = %+v", definition)
	}

	references := navigate(t, app, "/api/references?project=.&path=navigation%2Fformat.go&line=6&column=5")
	if len(references.Locations) != 2 || references.Locations[0].Path != "navigation/format.go" || references.Locations[1].Path != "navigation/greet.go" {
		t.Fatalf("references = %+v", references)
	}

	tsx := navigate(t, app, "/api/definition?project=.&path=navigation%2Fweb%2Fapp.tsx&line=4&column=22")
	if len(tsx.Locations) != 1 || tsx.Locations[0].Path != "navigation/web/label.ts" || tsx.Locations[0].Line != 2 {
		t.Fatalf("tsx definition = %+v", tsx)
	}
}

func TestNavigationRejectsBadRequests(t *testing.T) {
	app := newTestApp(t, filepath.Join("..", "..", "testdata", "markdown"))
	for target, status := range map[string]int{
		"/api/definition?project=.&path=navigation%2Fgreet.go&line=0&column=0":   http.StatusBadRequest,
		"/api/definition?project=.&path=navigation%2Fgreet.go&line=x&column=0":   http.StatusBadRequest,
		"/api/references?project=.&path=README.md&line=1&column=0":               http.StatusBadRequest,
		"/api/definition?project=.&path=navigation%2Fmissing.go&line=1&column=0": http.StatusNotFound,
		"/api/definition?project=.&path=..%2Fsecret.go&line=1&column=0":          http.StatusNotFound,
		"/api/definition?project=missing&path=a.go&line=1&column=0":              http.StatusNotFound,
	} {
		if response := request(t, app, target); response.Code != status {
			t.Errorf("%s status = %d, want %d: %s", target, response.Code, status, response.Body.String())
		}
	}
}

func TestGitignoredFilesStayBrowsableButLeaveSearchAndNavigation(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		".gitignore":       "generated/\n",
		"main.go":          "package app\n\nfunc main() { Build() }\n",
		"generated/gen.go": "package app\n\nfunc Build() {}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app := newTestApp(t, root)

	if response, data := requestPageData(t, app, "/api/page?project=.&path=generated%2Fgen.go"); response.Code != http.StatusOK || !data.HasFile {
		t.Fatalf("ignored file not browsable: %d %+v", response.Code, data.Error)
	}
	var search searchDocumentsResponse
	if err := json.Unmarshal(request(t, app, "/api/search-documents?project=.").Body.Bytes(), &search); err != nil {
		t.Fatal(err)
	}
	for _, document := range search.Documents {
		if document.Path == "generated/gen.go" {
			t.Fatal("search includes a gitignored file")
		}
	}
	if result := navigate(t, app, "/api/definition?project=.&path=main.go&line=3&column=15"); len(result.Locations) != 0 {
		t.Fatalf("definition reached a gitignored file: %+v", result)
	}
}

func TestNavigationFixturesMatchServer(t *testing.T) {
	app := newTestApp(t, filepath.Join("..", "..", "testdata", "markdown"))
	for name, target := range map[string]string{
		"definition.json": "/api/definition?project=.&path=navigation%2Fgreet.go&line=6&column=8",
		"references.json": "/api/references?project=.&path=navigation%2Fgreet.go&line=6&column=8",
	} {
		content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "api", "default", name))
		if err != nil {
			t.Fatal(err)
		}
		var fixture navigation.Result
		if err := json.Unmarshal(content, &fixture); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if served := navigate(t, app, target); !reflect.DeepEqual(fixture, served) {
			t.Errorf("%s = %+v, server = %+v", name, fixture, served)
		}
	}
}
