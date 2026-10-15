package web

import (
	"encoding/json"
	"net/http"
	"net/url"
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
	app, err := New(Config{Roots: []string{root}, ScanDepth: 10, Depth: 5, Exclusions: []string{".git", "node_modules", "vendor"}})
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
	app, root := newProjectsApp(t)
	api, web := projectQuery(root, "api"), projectQuery(root, "web")
	response, data := requestPageData(t, app, "/api/page?project="+api)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body.String())
	}
	if data.Project != filepath.Join(root, "api") || data.ProjectUnavailable || data.RootName != "api" || data.Branch != "main" || data.FileCount != 3 ||
		data.Repository != "api" || data.ProjectKind != projects.KindRepo || data.Worktrees != 1 {
		t.Fatalf("page = project %q root %q repository %q kind %q branch %q worktrees %d files %d",
			data.Project, data.RootName, data.Repository, data.ProjectKind, data.Branch, data.Worktrees, data.FileCount)
	}
	if !reflect.DeepEqual(data.Roots, []string{root}) {
		t.Fatalf("roots = %v", data.Roots)
	}
	if strings.Contains(response.Body.String(), "secret.md") || strings.Contains(response.Body.String(), "notes.md") {
		t.Fatal("api project lists files outside it")
	}

	for target, want := range map[string]int{
		"/api/page?project=" + api + "&path=../web/secret.md":     http.StatusBadRequest,
		"/api/page?project=" + api + "&path=secret.md":            http.StatusNotFound,
		"/api/image?project=" + api + "&path=../web/logo.png":     http.StatusNotFound,
		"/api/raw?project=" + api + "&path=../web/logo.png":       http.StatusNotFound,
		"/api/format?project=" + api + "&path=../web/secret.md":   http.StatusNotFound,
		"/api/page?project=" + web + "&path=secret.md":            http.StatusOK,
		"/api/image?project=" + api + "&path=logo.png":            http.StatusOK,
		"/api/page?project=.&path=web%2Fsecret.md":                http.StatusOK,
		"/api/page?project=" + projectQuery(root, "missing"):      http.StatusNotFound,
		"/api/page?project=api":                                   http.StatusNotFound,
		"/api/page?project=" + url.QueryEscape(root+"/api/.."):    http.StatusNotFound,
		"/api/page?project=" + projectQuery(root, "api", ".git"):  http.StatusNotFound,
		"/api/search-documents?project=" + url.QueryEscape("/"):   http.StatusBadRequest,
		"/api/page?project=" + projectQuery(root, "api", "docs"):  http.StatusNotFound,
		"/api/page?project=" + web + "&path=..%2Fapi%2FREADME.md": http.StatusBadRequest,
	} {
		if response := request(t, app, target); response.Code != want {
			t.Errorf("%s = %d, want %d", target, response.Code, want)
		}
	}

	_, missing := requestPageData(t, app, "/api/page?project=missing&path=README.md")
	if missing.Error != projectUnavailableMessage || !missing.ProjectUnavailable || missing.Project != "missing" || missing.Selected != "README.md" {
		t.Fatalf("missing project page = %+v", missing)
	}
	_, inside := requestPageData(t, app, "/api/page?project="+projectQuery(root, "api", "docs"))
	if inside.Error != insideRepositoryMessage || !inside.ProjectUnavailable {
		t.Fatalf("folder inside repository page = %+v", inside)
	}
}

func TestProjectLinksStayInProject(t *testing.T) {
	app, root := newProjectsApp(t)
	api := projectQuery(root, "api")
	_, data := requestPageData(t, app, "/api/page?project="+api+"&path=README.md")
	assertContains(t, data.Content,
		`href="/view?project=`+api+`&amp;path=docs%2Fguide.md"`,
		`src="/api/image?project=`+api+`&amp;path=logo.png"`)
}

func TestSwitchingProjectsReadsCurrentFiles(t *testing.T) {
	app, root := newProjectsApp(t)
	web := "/api/page?project=" + projectQuery(root, "web")
	_, before := requestPageData(t, app, web)
	writeMarkdown(t, filepath.Join(root, "web", "later.md"), "# Later\n")
	if err := os.Remove(filepath.Join(root, "web", "secret.md")); err != nil {
		t.Fatal(err)
	}
	_, after := requestPageData(t, app, web)
	if before.FileCount != 2 || after.FileCount != 2 {
		t.Fatalf("file counts = %d, %d", before.FileCount, after.FileCount)
	}
	if got := after.Tree[0].Path; got != "later.md" {
		t.Fatalf("first file after switch = %q, want later.md", got)
	}
	writeMarkdown(t, filepath.Join(root, "late", ".git", "HEAD"), "ref: refs/heads/main\n")
	if response := request(t, app, "/api/page?project="+projectQuery(root, "late")); response.Code != http.StatusOK {
		t.Fatalf("new repository status = %d", response.Code)
	}
}

func TestProjectsEndpointRediscovers(t *testing.T) {
	app, root := newProjectsApp(t)
	first := requestProjects(t, app)
	if want := []projects.Root{{Path: root, Name: filepath.Base(root)}}; !reflect.DeepEqual(first.Roots, want) {
		t.Fatalf("roots = %#v", first.Roots)
	}
	api, web := filepath.Join(root, "api"), filepath.Join(root, "web")
	if got := projectIDs(first.Projects); !reflect.DeepEqual(got, []string{root, api, web}) {
		t.Fatalf("projects = %v", got)
	}
	late := filepath.Join(root, "late")
	writeMarkdown(t, filepath.Join(late, ".git", "HEAD"), "ref: refs/heads/main\n")
	if got := projectIDs(requestProjects(t, app).Projects); !reflect.DeepEqual(got, []string{root, api, late, web}) {
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
	for _, key := range []string{"id", "name", "path", "kind", "group", "root", "branch", "repository", "available"} {
		if _, ok := raw.Projects[0][key]; !ok {
			t.Errorf("project JSON lacks %q: %v", key, keys)
		}
	}
}

func TestProjectsSpanRoots(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(parent, "first"), filepath.Join(parent, "second")
	writeMarkdown(t, filepath.Join(first, "a", ".git", "HEAD"), "ref: refs/heads/main\n")
	writeMarkdown(t, filepath.Join(first, "a", "a.md"), "# A\n")
	writeMarkdown(t, filepath.Join(second, "b", "b.md"), "# B\n")
	writeMarkdown(t, filepath.Join(parent, "outside", "secret.md"), "# Secret\n")
	app, err := New(Config{Roots: []string{first, second}, ScanDepth: 10, Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	list := requestProjects(t, app)
	if got := projectIDs(list.Projects); !reflect.DeepEqual(got, []string{first, filepath.Join(first, "a"), second}) {
		t.Fatalf("projects = %v", got)
	}
	if len(list.Roots) != 2 || list.Roots[1].Path != second {
		t.Fatalf("roots = %#v", list.Roots)
	}
	for target, want := range map[string]int{
		"/api/page?project=" + projectQuery(second, "b") + "&path=b.md": http.StatusOK,
		"/api/page?project=" + projectQuery(first, "a") + "&path=a.md":  http.StatusOK,
		"/api/page?project=" + projectQuery(parent, "outside"):          http.StatusBadRequest,
		"/api/directories?path=" + url.QueryEscape(second):              http.StatusOK,
		"/api/directories?path=" + url.QueryEscape(parent):              http.StatusBadRequest,
	} {
		if response := request(t, app, target); response.Code != want {
			t.Errorf("%s = %d, want %d", target, response.Code, want)
		}
	}
	if _, err := New(Config{Roots: []string{parent, first}}); err == nil {
		t.Fatal("New() with nested roots error = nil")
	}
}

func TestDirectoriesEndpoint(t *testing.T) {
	app, root := newProjectsApp(t)
	response := request(t, app, "/api/directories?path="+url.QueryEscape(root))
	var listing directoriesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	want := directoriesResponse{Path: root, Directories: []projects.Directory{
		{Name: "api", Path: filepath.Join(root, "api"), IsProject: true},
		{Name: "web", Path: filepath.Join(root, "web"), IsProject: true},
	}}
	if response.Code != http.StatusOK || !reflect.DeepEqual(listing, want) {
		t.Fatalf("root listing = %d %+v", response.Code, listing)
	}
	roots := request(t, app, "/api/directories")
	if roots.Code != http.StatusOK || !strings.Contains(roots.Body.String(), `"path":"`+root+`"`) {
		t.Fatalf("roots listing = %d %s", roots.Code, roots.Body.String())
	}
	for target, status := range map[string]int{
		"/api/directories?path=" + projectQuery(root, "api"):         http.StatusBadRequest,
		"/api/directories?path=" + projectQuery(root, "api", "docs"): http.StatusBadRequest,
		"/api/directories?path=..":                                   http.StatusNotFound,
		"/api/directories?path=" + projectQuery(root, "missing"):     http.StatusNotFound,
		"/api/directories?path=%2F":                                  http.StatusBadRequest,
	} {
		if response := request(t, app, target); response.Code != status {
			t.Errorf("%s = %d, want %d", target, response.Code, status)
		}
	}
}

func TestShellEmbedsPageOnlyWithProject(t *testing.T) {
	app, root := newProjectsApp(t)
	if body := request(t, app, "/").Body.String(); !strings.Contains(body, `<template id="app-data">null</template>`) {
		t.Fatalf("shell without project embeds page data")
	}
	body := request(t, app, "/view?project="+projectQuery(root, "api")+"&path=README.md").Body.String()
	if !strings.Contains(body, `"project":"`+filepath.Join(root, "api")+`"`) || !strings.Contains(body, `"selected":"README.md"`) {
		t.Fatalf("shell with project lacks page data")
	}
}

// projectQuery is the query-escaped project ID of a directory below root.
func projectQuery(root string, segments ...string) string {
	return url.QueryEscape(filepath.Join(append([]string{root}, segments...)...))
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
