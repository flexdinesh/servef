package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/internal/projects"
)

func newProjectsApp(t *testing.T) (*App, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeMarkdown(t, filepath.Join(root, "notes.md"), "# Notes\n")
	writeMarkdown(t, filepath.Join(root, "api", ".git", "HEAD"), "ref: refs/heads/main\n")
	writeMarkdown(t, filepath.Join(root, "api", "README.md"), "# API\n\n[Guide](docs/guide.md)\n\n![logo](logo.png)\n")
	writeMarkdown(t, filepath.Join(root, "api", "docs", "guide.md"), "# Guide\n")
	writeMarkdown(t, filepath.Join(root, "api", "logo.png"), "png")
	writeMarkdown(t, filepath.Join(root, "web", ".git", "HEAD"), "ref: refs/heads/main\n")
	writeMarkdown(t, filepath.Join(root, "web", "secret.md"), "# Web only\n")
	writeMarkdown(t, filepath.Join(root, "web", "logo.png"), "png")
	app, err := New(Config{Root: root, ScanDepth: 10, Depth: 5, Exclusions: []string{".git", "node_modules", "vendor"}})
	if err != nil {
		t.Fatal(err)
	}
	return app, root
}

func TestFileAPIsRequireProject(t *testing.T) {
	app, _ := newProjectsApp(t)
	for _, target := range []string{"/api/page", "/api/page?path=notes.md", "/api/format?path=notes.md", "/api/search-documents"} {
		response := request(t, app, target)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), noProjectMessage) {
			t.Errorf("%s = %d %s, want 400 %q", target, response.Code, response.Body.String(), noProjectMessage)
		}
	}
	for _, target := range []string{"/api/raw?path=api/logo.png", "/api/image?path=api/logo.png"} {
		if response := request(t, app, target); response.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", target, response.Code)
		}
	}
}

func TestPageIsScopedToProject(t *testing.T) {
	app, _ := newProjectsApp(t)
	response, data := requestPageData(t, app, "/api/page?project=api")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body.String())
	}
	if data.Project != "api" || data.RootName != "api" || data.FileCount != 3 {
		t.Fatalf("page = project %q root %q files %d", data.Project, data.RootName, data.FileCount)
	}
	if strings.Contains(response.Body.String(), "secret.md") || strings.Contains(response.Body.String(), "notes.md") {
		t.Fatal("api project lists files outside it")
	}

	for target, want := range map[string]int{
		"/api/page?project=api&path=../web/secret.md":     http.StatusBadRequest,
		"/api/page?project=api&path=secret.md":            http.StatusNotFound,
		"/api/image?project=api&path=../web/logo.png":     http.StatusNotFound,
		"/api/raw?project=api&path=../web/logo.png":       http.StatusNotFound,
		"/api/format?project=api&path=../web/secret.md":   http.StatusNotFound,
		"/api/page?project=web&path=secret.md":            http.StatusOK,
		"/api/image?project=api&path=logo.png":            http.StatusOK,
		"/api/page?project=.&path=web%2Fsecret.md":        http.StatusOK,
		"/api/page?project=missing":                       http.StatusNotFound,
		"/api/page?project=..":                            http.StatusBadRequest,
		"/api/page?project=api%2F.git":                    http.StatusNotFound,
		"/api/search-documents?project=..%2F..":           http.StatusBadRequest,
		"/api/search-documents?project=" + "%2Fetc":       http.StatusNotFound,
		"/api/page?project=api%2Fdocs&path=guide.md":      http.StatusOK,
		"/api/page?project=api%2Fdocs%2Fguide.md":         http.StatusNotFound,
		"/api/page?project=web&path=..%2Fapi%2FREADME.md": http.StatusBadRequest,
	} {
		if response := request(t, app, target); response.Code != want {
			t.Errorf("%s = %d, want %d", target, response.Code, want)
		}
	}

	_, missing := requestPageData(t, app, "/api/page?project=missing&path=README.md")
	if missing.Error != projectUnavailableMessage || missing.Project != "missing" || missing.Selected != "README.md" {
		t.Fatalf("missing project page = %+v", missing)
	}
}

func TestProjectLinksStayInProject(t *testing.T) {
	app, _ := newProjectsApp(t)
	_, data := requestPageData(t, app, "/api/page?project=api&path=README.md")
	assertContains(t, data.Content,
		`href="/view?project=api&amp;path=docs%2Fguide.md"`,
		`src="/api/image?project=api&amp;path=logo.png"`)
}

func TestSwitchingProjectsReadsCurrentFiles(t *testing.T) {
	app, root := newProjectsApp(t)
	_, before := requestPageData(t, app, "/api/page?project=web")
	writeMarkdown(t, filepath.Join(root, "web", "later.md"), "# Later\n")
	if err := os.Remove(filepath.Join(root, "web", "secret.md")); err != nil {
		t.Fatal(err)
	}
	_, after := requestPageData(t, app, "/api/page?project=web")
	if before.FileCount != 2 || after.FileCount != 2 {
		t.Fatalf("file counts = %d, %d", before.FileCount, after.FileCount)
	}
	if got := after.Tree[0].Path; got != "later.md" {
		t.Fatalf("first file after switch = %q, want later.md", got)
	}
	writeMarkdown(t, filepath.Join(root, "late", ".git", "HEAD"), "ref: refs/heads/main\n")
	if response := request(t, app, "/api/page?project=late"); response.Code != http.StatusOK {
		t.Fatalf("new repository status = %d", response.Code)
	}
}

func TestProjectsEndpointRediscovers(t *testing.T) {
	app, root := newProjectsApp(t)
	first := requestProjects(t, app)
	if first.RootName != filepath.Base(root) {
		t.Fatalf("root name = %q", first.RootName)
	}
	if got := projectIDs(first.Projects); !reflect.DeepEqual(got, []string{".", "api", "web"}) {
		t.Fatalf("projects = %v", got)
	}
	writeMarkdown(t, filepath.Join(root, "late", ".git", "HEAD"), "ref: refs/heads/main\n")
	if got := projectIDs(requestProjects(t, app).Projects); !reflect.DeepEqual(got, []string{".", "api", "late", "web"}) {
		t.Fatalf("rediscovered projects = %v", got)
	}

	var raw struct {
		Projects []map[string]any `json:"projects"`
	}
	response := request(t, app, "/api/projects")
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0)
	for key := range raw.Projects[0] {
		keys = append(keys, key)
	}
	for _, key := range []string{"id", "name", "path", "kind", "group", "available"} {
		if _, ok := raw.Projects[0][key]; !ok {
			t.Errorf("project JSON lacks %q: %v", key, keys)
		}
	}
}

func TestDirectoriesEndpoint(t *testing.T) {
	app, _ := newProjectsApp(t)
	response := request(t, app, "/api/directories")
	var listing directoriesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	want := directoriesResponse{Path: ".", Directories: []projects.Directory{
		{Name: "api", Path: "api", IsProject: true},
		{Name: "web", Path: "web", IsProject: true},
	}}
	if response.Code != http.StatusOK || !reflect.DeepEqual(listing, want) {
		t.Fatalf("root listing = %d %+v", response.Code, listing)
	}
	for target, status := range map[string]int{
		"/api/directories?path=api":     http.StatusOK,
		"/api/directories?path=..":      http.StatusBadRequest,
		"/api/directories?path=missing": http.StatusNotFound,
		"/api/directories?path=%2Ftmp":  http.StatusNotFound,
	} {
		if response := request(t, app, target); response.Code != status {
			t.Errorf("%s = %d, want %d", target, response.Code, status)
		}
	}
}

func TestShellEmbedsPageOnlyWithProject(t *testing.T) {
	app, _ := newProjectsApp(t)
	if body := request(t, app, "/").Body.String(); !strings.Contains(body, `<template id="app-data">null</template>`) {
		t.Fatalf("shell without project embeds page data")
	}
	body := request(t, app, "/view?project=api&path=README.md").Body.String()
	if !strings.Contains(body, `"project":"api"`) || !strings.Contains(body, `"selected":"README.md"`) {
		t.Fatalf("shell with project lacks page data")
	}
}

func requestProjects(t *testing.T, app *App) projectsResponse {
	t.Helper()
	response := request(t, app, "/api/projects")
	if response.Code != http.StatusOK {
		t.Fatalf("projects status = %d", response.Code)
	}
	var payload projectsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func projectIDs(list []projects.Project) []string {
	ids := make([]string, len(list))
	for i, project := range list {
		ids[i] = project.ID
	}
	return ids
}

// Frontend project fixtures must decode into the server's response types.
func TestProjectFixturesMatchServerContract(t *testing.T) {
	for name, target := range map[string]any{
		"projects.json":    &projectsResponse{},
		"directories.json": &directoriesResponse{},
	} {
		content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "api", "default", name))
		if err != nil {
			t.Fatal(err)
		}
		decoder := json.NewDecoder(strings.NewReader(string(content)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
