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

	"github.com/flexdinesh/srcz/internal/document"
	"github.com/flexdinesh/srcz/pkg/contract"
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
		"scope/x.zzz":  "unregistered extension\n",
		"docs/keep.md": "# Keep\n",
	}
	for name, content := range committed {
		writeMarkdown(t, filepath.Join(root, filepath.FromSlash(name)), content)
		if _, err := worktree.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := worktree.Commit("initial", &gogit.CommitOptions{Author: &object.Signature{Name: "srcz", Email: "srcz@example.com", When: time.Unix(1700000000, 0)}}); err != nil {
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

func TestChangesListsRepositoryChanges(t *testing.T) {
	root := changesRepo(t)
	app := newTestApp(t, root)

	var changes contract.ChangesResponse
	if status := requestJSON(t, app, "/api/changes?project=.", &changes); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	want := contract.ChangesResponse{
		Repository: true,
		Branch:     "master",
		Files: []contract.ChangedFile{
			{Path: "added.go", Status: contract.ChangeAdded},
			{Path: "data.bin", Status: contract.ChangeModified},
			{Path: "doc.md", Status: contract.ChangeModified},
			{Path: "gone.md", Status: contract.ChangeDeleted},
			{Path: "large.txt", Status: contract.ChangeModified},
			{Path: "moved/new.md", OldPath: "old.md", Status: contract.ChangeRenamed},
			{Path: "notes.txt", Status: contract.ChangeUntracked},
			{Path: "scope/x.zzz", Status: contract.ChangeModified},
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

	var changes contract.ChangesResponse
	if status := requestJSON(t, app, "/api/changes?project=.", &changes); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if want := (contract.ChangesResponse{Files: []contract.ChangedFile{}, Warnings: []string{}}); !reflect.DeepEqual(changes, want) {
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

// diff reads a diff's sides; TestDiffCarriesTheLineDiff covers line diffs.
func diff(t *testing.T, app *App, query string) contract.DiffResponse {
	t.Helper()
	var response contract.DiffResponse
	if status := requestJSON(t, app, "/api/diff?project=.&"+query, &response); status != http.StatusOK {
		t.Fatalf("%s status = %d", query, status)
	}
	response.LineDiff = nil
	return response
}

func text(content string) *contract.DiffSide {
	return &contract.DiffSide{Content: &content, Size: int64(len(content))}
}

func TestDiffReadsBothSidesUnderSizeLimits(t *testing.T) {
	root := changesRepo(t)
	app := newTestApp(t, root)
	markdown := documentType(app, "doc.md")

	for _, test := range []struct {
		query string
		want  contract.DiffResponse
	}{
		{"path=doc.md", contract.DiffResponse{Path: "doc.md", Status: contract.ChangeModified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}},
		{"path=added.go", contract.DiffResponse{Path: "added.go", Status: contract.ChangeAdded, DocumentType: documentType(app, "added.go"), Modified: text("package main\n")}},
		{"path=notes.txt", contract.DiffResponse{Path: "notes.txt", Status: contract.ChangeUntracked, DocumentType: documentType(app, "notes.txt"), Modified: text("loose\n")}},
		{"path=gone.md", contract.DiffResponse{Path: "gone.md", Status: contract.ChangeDeleted, DocumentType: markdown, Original: text("# Gone\n")}},
		{"path=moved/new.md&oldPath=old.md", contract.DiffResponse{Path: "moved/new.md", OldPath: "old.md", Status: contract.ChangeRenamed, DocumentType: markdown, Original: text("# Moved\n"), Modified: text("# Moved\n")}},
		{"path=data.bin", contract.DiffResponse{Path: "data.bin", Status: contract.ChangeModified, DocumentType: documentType(app, "data.bin"),
			Original: &contract.DiffSide{Size: 11, Binary: true}, Modified: &contract.DiffSide{Size: 11, Binary: true}}},
		{"path=large.txt", contract.DiffResponse{Path: "large.txt", Status: contract.ChangeModified, DocumentType: documentType(app, "large.txt"),
			Original: &contract.DiffSide{Size: document.PreviewLimit + 1, TooLarge: true}, Modified: &contract.DiffSide{Size: document.PreviewLimit + 1, TooLarge: true}}},
	} {
		if got := diff(t, app, test.query); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s = %s\nwant %s", test.query, describeDiff(got), describeDiff(test.want))
		}
	}
}

// The line diff covers text sides only; ignoreWhitespace=1 compares lines
// without their leading and trailing whitespace.
func TestDiffCarriesTheLineDiff(t *testing.T) {
	root := changesRepo(t)
	writeMarkdown(t, filepath.Join(root, "doc.md"), "  # Before\n")
	app := newTestApp(t, root)
	lineDiff := func(query string) *contract.LineDiff {
		t.Helper()
		var response contract.DiffResponse
		if status := requestJSON(t, app, "/api/diff?project=.&"+query, &response); status != http.StatusOK {
			t.Fatalf("%s status = %d", query, status)
		}
		return response.LineDiff
	}
	changed := &contract.LineDiff{Additions: 1, Deletions: 1, Gaps: []contract.DiffGap{}, Hunks: []contract.DiffHunk{{
		OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 1, Additions: 1, Deletions: 1,
		Blocks: []contract.DiffBlock{{Kind: contract.DiffBlockChange, OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 1}},
	}}}
	if got := lineDiff("path=doc.md"); !reflect.DeepEqual(got, changed) {
		t.Errorf("doc.md line diff = %+v, want %+v", got, changed)
	}
	if got, want := lineDiff("path=doc.md&ignoreWhitespace=1"), (&contract.LineDiff{IgnoreWhitespace: true, Hunks: []contract.DiffHunk{}, Gaps: []contract.DiffGap{}}); !reflect.DeepEqual(got, want) {
		t.Errorf("doc.md ignoring whitespace = %+v, want %+v", got, want)
	}
	added := &contract.LineDiff{Additions: 1, Gaps: []contract.DiffGap{}, Hunks: []contract.DiffHunk{{
		OldStart: 1, NewStart: 1, NewCount: 1, Additions: 1,
		Blocks: []contract.DiffBlock{{Kind: contract.DiffBlockChange, OldStart: 1, NewStart: 1, NewCount: 1}},
	}}}
	if got := lineDiff("path=added.go"); !reflect.DeepEqual(got, added) {
		t.Errorf("added.go line diff = %+v, want %+v", got, added)
	}
	for _, query := range []string{"path=data.bin", "path=large.txt"} {
		if got := lineDiff(query); got != nil {
			t.Errorf("%s line diff = %+v, want none for binary and too-large sides", query, got)
		}
	}
	var failure errorResponse
	if status := requestJSON(t, app, "/api/diff?project=.&path=doc.md&ignoreWhitespace=true", &failure); status != http.StatusBadRequest || failure.Error != "IgnoreWhitespace must be 1." {
		t.Errorf("ignoreWhitespace=true = %d %+v", status, failure)
	}
}

func describeDiff(response contract.DiffResponse) string {
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
	if want := (&contract.DiffSide{Size: document.DisplayLimit + 1, TooLarge: true}); got.Status != contract.ChangeUntracked || got.Original != nil || !reflect.DeepEqual(got.Modified, want) {
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
	dir := filepath.Join("..", "..", "..", "..", "testdata", "api", "default")
	types := document.Builtins()
	names, err := filepath.Glob(filepath.Join(dir, "diff-*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("diff fixtures = %v, %v", names, err)
	}
	for _, name := range names {
		var fixture contract.DiffResponse
		decodeFixture(t, name, &fixture)
		if want := fileTypeContract(types.Lookup(fixture.Path).Type()); !reflect.DeepEqual(fixture.DocumentType, want) {
			t.Errorf("%s documentType = %+v, want %+v", name, fixture.DocumentType, want)
		}
		for _, side := range []*contract.DiffSide{fixture.Original, fixture.Modified} {
			if side != nil && side.Content != nil && (side.Binary || side.TooLarge || int64(len(*side.Content)) != side.Size) {
				t.Errorf("%s side %+v carries content it should not, or a wrong size", name, *side)
			}
		}
	}
	for _, name := range []string{"changes.json", "changes-not-repo.json"} {
		var fixture contract.ChangesResponse
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
