package web

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/git"
)

// changesRepo is a repository with one change of every kind.
func changesRepo(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	committed := map[string]string{
		".gitignore":   "*.log\n",
		"doc.md":       "# Before\n",
		"same.md":      "# Same\n",
		"gone.md":      "# Gone\n",
		"old.md":       "# Moved\n",
		"data.bin":     "head\x00binary",
		"large.txt":    strings.Repeat("a", document.PreviewLimit+1),
		"scope/x.zzz":  "outside the filename scope\n",
		"docs/keep.md": "# Keep\n",
	}
	for name, content := range committed {
		writeMarkdown(t, filepath.Join(root, filepath.FromSlash(name)), content)
		if _, err := worktree.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := worktree.Commit("initial", &gogit.CommitOptions{Author: &object.Signature{Name: "servef", Email: "servef@example.com", When: time.Unix(1700000000, 0)}}); err != nil {
		t.Fatal(err)
	}

	writeMarkdown(t, filepath.Join(root, "doc.md"), "# After\n")
	writeMarkdown(t, filepath.Join(root, "data.bin"), "work\x00binary")
	writeMarkdown(t, filepath.Join(root, "large.txt"), strings.Repeat("b", document.PreviewLimit+1))
	writeMarkdown(t, filepath.Join(root, "scope", "x.zzz"), "changed\n")
	for _, name := range []string{"gone.md", "old.md"} {
		if err := os.Remove(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	writeMarkdown(t, filepath.Join(root, "moved", "new.md"), "# Moved\n")
	writeMarkdown(t, filepath.Join(root, "added.go"), "package main\n")
	if _, err := worktree.Add("added.go"); err != nil {
		t.Fatal(err)
	}
	writeMarkdown(t, filepath.Join(root, "notes.txt"), "loose\n")
	writeMarkdown(t, filepath.Join(root, "debug.log"), "ignored\n")
	return root
}

func requestJSON(t *testing.T, app *App, target string, value any) int {
	t.Helper()
	response := request(t, app, target)
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("%s Content-Type = %q", target, got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("%s Cache-Control = %q", target, got)
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Body.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatalf("%s: %v\n%s", target, err, response.Body.String())
	}
	return response.Code
}

func TestChangesListsRepositoryChangesBeyondFilenameScope(t *testing.T) {
	root := changesRepo(t)
	app := newScopedApp(t, root, "*.md")

	var changes changesResponse
	if status := requestJSON(t, app, "/api/changes?project=.", &changes); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	want := changesResponse{
		Repository: true,
		Branch:     "master",
		Files: []git.Change{
			{Path: "added.go", Status: git.Added},
			{Path: "data.bin", Status: git.Modified},
			{Path: "doc.md", Status: git.Modified},
			{Path: "gone.md", Status: git.Deleted},
			{Path: "large.txt", Status: git.Modified},
			{Path: "moved/new.md", OldPath: "old.md", Status: git.Renamed},
			{Path: "notes.txt", Status: git.Untracked},
			{Path: "scope/x.zzz", Status: git.Modified},
		},
		Warnings: []string{},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes = %+v\nwant      %+v", changes, want)
	}
}

func TestChangesReportsFoldersAndProjectErrors(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "README.md"), "# Folder")
	app := newTestApp(t, root)

	var changes changesResponse
	if status := requestJSON(t, app, "/api/changes?project=.", &changes); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if want := (changesResponse{Files: []git.Change{}, Warnings: []string{}}); !reflect.DeepEqual(changes, want) {
		t.Fatalf("folder changes = %+v", changes)
	}
	var failure errorResponse
	if status := requestJSON(t, app, "/api/changes", &failure); status != http.StatusBadRequest || failure.Error != noProjectMessage {
		t.Fatalf("no project = %d %+v", status, failure)
	}
	if status := requestJSON(t, app, "/api/changes?project=%2Fmissing", &failure); status != http.StatusNotFound {
		t.Fatalf("missing project = %d %+v", status, failure)
	}
	if status := requestJSON(t, app, "/api/diff?project=.&path=README.md", &failure); status != http.StatusNotFound || failure.Error != changeUnavailableMessage {
		t.Fatalf("folder diff = %d %+v", status, failure)
	}
}

func diff(t *testing.T, app *App, query string) diffResponse {
	t.Helper()
	var response diffResponse
	if status := requestJSON(t, app, "/api/diff?project=.&"+query, &response); status != http.StatusOK {
		t.Fatalf("%s status = %d", query, status)
	}
	return response
}

func text(content string) *diffSide {
	return &diffSide{Content: &content, Size: int64(len(content))}
}

func TestDiffReadsBothSidesUnderSizeLimits(t *testing.T) {
	root := changesRepo(t)
	app := newScopedApp(t, root, "*.md")
	markdown := app.config.Types.Lookup("doc.md").Type()

	for _, test := range []struct {
		query string
		want  diffResponse
	}{
		{"path=doc.md", diffResponse{Path: "doc.md", Status: git.Modified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}},
		{"path=added.go", diffResponse{Path: "added.go", Status: git.Added, DocumentType: app.config.Types.Lookup("added.go").Type(), Modified: text("package main\n")}},
		{"path=notes.txt", diffResponse{Path: "notes.txt", Status: git.Untracked, DocumentType: app.config.Types.Lookup("notes.txt").Type(), Modified: text("loose\n")}},
		{"path=gone.md", diffResponse{Path: "gone.md", Status: git.Deleted, DocumentType: markdown, Original: text("# Gone\n")}},
		{"path=moved/new.md&oldPath=old.md", diffResponse{Path: "moved/new.md", OldPath: "old.md", Status: git.Renamed, DocumentType: markdown, Original: text("# Moved\n"), Modified: text("# Moved\n")}},
		{"path=data.bin", diffResponse{Path: "data.bin", Status: git.Modified, DocumentType: app.config.Types.Lookup("data.bin").Type(),
			Original: &diffSide{Size: 11, Binary: true}, Modified: &diffSide{Size: 11, Binary: true}}},
		{"path=large.txt", diffResponse{Path: "large.txt", Status: git.Modified, DocumentType: app.config.Types.Lookup("large.txt").Type(),
			Original: &diffSide{Size: document.PreviewLimit + 1, TooLarge: true}, Modified: &diffSide{Size: document.PreviewLimit + 1, TooLarge: true}}},
	} {
		if got := diff(t, app, test.query); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s = %s\nwant %s", test.query, describeDiff(got), describeDiff(test.want))
		}
	}
}

func describeDiff(response diffResponse) string {
	encoded, _ := json.Marshal(response)
	return string(encoded)
}

func TestDiffNeverReadsWorktreeFilesOverDisplayLimit(t *testing.T) {
	root := changesRepo(t)
	huge := filepath.Join(root, "huge.txt")
	file, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	// A sparse file reaches the size without writing it.
	if err := file.Truncate(document.DisplayLimit + 1); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	app := newTestApp(t, root)
	app.openFile = func(name string) (fs.File, error) {
		t.Errorf("opened %s", name)
		return os.Open(name)
	}

	got := diff(t, app, "path=huge.txt")
	if want := (&diffSide{Size: document.DisplayLimit + 1, TooLarge: true}); got.Status != git.Untracked || got.Original != nil || !reflect.DeepEqual(got.Modified, want) {
		t.Fatalf("huge diff = %s", describeDiff(got))
	}
}

func TestDiffRejectsPathsOutsideTheChangeSet(t *testing.T) {
	root := changesRepo(t)
	app := newTestApp(t, root)

	var unchanged errorResponse
	if status := requestJSON(t, app, "/api/diff?project=.&path=same.md", &unchanged); status != http.StatusNotFound {
		t.Fatalf("unchanged status = %d", status)
	}
	if want := (errorResponse{Error: "No changes to same.md since the last commit.", Code: "unchanged"}); unchanged != want {
		t.Fatalf("unchanged = %+v", unchanged)
	}
	for _, query := range []string{
		"path=debug.log",
		"path=missing.md",
		"path=../outside.md",
		"path=" + filepath.Join(root, "doc.md"),
		"path=.git/config",
		"path=moved/new.md&oldPath=gone.md",
		"path=doc.md&oldPath=old.md",
	} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/diff?project=.&"+query, &failure); status != http.StatusNotFound || failure != (errorResponse{Error: changeUnavailableMessage}) {
			t.Errorf("%s = %d %+v", query, status, failure)
		}
	}
	var failure errorResponse
	if status := requestJSON(t, app, "/api/diff?project=.", &failure); status != http.StatusBadRequest {
		t.Fatalf("missing path status = %d", status)
	}
}

// Frontend change and diff fixtures must use the server's shapes and the
// registry's real document types.
func TestChangeFixturesMatchServerContract(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "api", "default")
	types := document.Builtins()
	names, err := filepath.Glob(filepath.Join(dir, "diff-*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("diff fixtures = %v, %v", names, err)
	}
	for _, name := range names {
		var fixture diffResponse
		decodeFixture(t, name, &fixture)
		if want := types.Lookup(fixture.Path).Type(); !reflect.DeepEqual(fixture.DocumentType, want) {
			t.Errorf("%s documentType = %+v, want %+v", name, fixture.DocumentType, want)
		}
		for _, side := range []*diffSide{fixture.Original, fixture.Modified} {
			if side != nil && side.Content != nil && (side.Binary || side.TooLarge || int64(len(*side.Content)) != side.Size) {
				t.Errorf("%s side %+v carries content it should not, or a wrong size", name, *side)
			}
		}
	}
	for _, name := range []string{"changes.json", "changes-not-repo.json"} {
		var fixture changesResponse
		decodeFixture(t, filepath.Join(dir, name), &fixture)
	}
}

func decodeFixture(t *testing.T, name string, value any) {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}
