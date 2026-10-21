package web

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/git"
)

// historyRepo is master at an initial commit and a checked-out feature
// branch one commit ahead with one change of every kind, plus an
// uncommitted edit. It returns the root and both commits.
func historyRepo(t *testing.T) (string, plumbing.Hash, plumbing.Hash) {
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
	zone := time.FixedZone("", 11*60*60)
	commit := func(message string, when int64) plumbing.Hash {
		signature := &object.Signature{Name: "servef", Email: "servef@example.com", When: time.Unix(when, 0).In(zone)}
		hash, err := worktree.Commit(message, &gogit.CommitOptions{Author: signature, Committer: signature})
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	add := func(names ...string) {
		for _, name := range names {
			if _, err := worktree.Add(name); err != nil {
				t.Fatal(err)
			}
		}
	}

	for name, content := range map[string]string{
		"doc.md":      "# Before\n",
		"same.md":     "# Same\n",
		"gone.md":     "# Gone\n",
		"old.md":      "# Moved\n",
		"data.bin":    "head\x00binary",
		"large.txt":   strings.Repeat("a", document.PreviewLimit+1),
		"scope/x.zzz": "outside the filename scope\n",
	} {
		writeMarkdown(t, filepath.Join(root, filepath.FromSlash(name)), content)
		add(name)
	}
	initial := commit("initial", 1700000000)

	writeMarkdown(t, filepath.Join(root, "doc.md"), "# After\n")
	writeMarkdown(t, filepath.Join(root, "data.bin"), "work\x00binary")
	writeMarkdown(t, filepath.Join(root, "large.txt"), strings.Repeat("b", document.PreviewLimit+1))
	writeMarkdown(t, filepath.Join(root, "scope", "x.zzz"), "changed\n")
	writeMarkdown(t, filepath.Join(root, "added.go"), "package main\n")
	add("doc.md", "data.bin", "large.txt", "scope/x.zzz", "added.go")
	if _, err := worktree.Remove("gone.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Move("old.md", "moved.md"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("feature"))); err != nil {
		t.Fatal(err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), initial)); err != nil {
		t.Fatal(err)
	}
	change := commit("Change everything\n\nWith a body.\n", 1700000100)

	writeMarkdown(t, filepath.Join(root, "doc.md"), "# Uncommitted\n")
	if err := os.Remove(filepath.Join(root, "same.md")); err != nil {
		t.Fatal(err)
	}
	return root, initial, change
}

func TestCommitsListsBranchAndFullHistory(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newScopedApp(t, root, "*.md")

	changeCommit := commitResponse{
		Hash:       change.String(),
		Parents:    []string{initial.String()},
		Subject:    "Change everything",
		Body:       "With a body.",
		Author:     git.Author{Name: "servef", Email: "servef@example.com"},
		AuthorTime: "2023-11-15T09:15:00+11:00",
	}
	initialCommit := commitResponse{
		Hash:       initial.String(),
		Parents:    []string{},
		Subject:    "initial",
		Author:     git.Author{Name: "servef", Email: "servef@example.com"},
		AuthorTime: "2023-11-15T09:13:20+11:00",
	}
	for _, test := range []struct {
		query string
		want  commitsResponse
	}{
		{"", commitsResponse{Repository: true, Branch: "feature", Base: "master", Range: git.RangeBranch, Head: change.String(), Ahead: 1, Commits: []commitResponse{changeCommit}}},
		{"&range=all", commitsResponse{Repository: true, Branch: "feature", Base: "master", Range: git.RangeAll, Head: change.String(), Ahead: 1, Commits: []commitResponse{changeCommit, initialCommit}}},
		{"&range=all&limit=1", commitsResponse{Repository: true, Branch: "feature", Base: "master", Range: git.RangeAll, Head: change.String(), Ahead: 1, Commits: []commitResponse{changeCommit}, More: true}},
		{"&range=all&skip=1&head=" + change.String(), commitsResponse{Repository: true, Branch: "feature", Base: "master", Range: git.RangeAll, Head: change.String(), Ahead: 1, Commits: []commitResponse{initialCommit}}},
		{"&range=branch&head=" + initial.String(), commitsResponse{Repository: true, Branch: "feature", Base: "master", Range: git.RangeBranch, Head: initial.String(), Commits: []commitResponse{}}},
	} {
		var got commitsResponse
		if status := requestJSON(t, app, "/api/commits?project=."+test.query, &got); status != http.StatusOK {
			t.Fatalf("%s status = %d", test.query, status)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s = %+v\nwant %+v", test.query, got, test.want)
		}
	}

	body := request(t, app, "/api/commits?project=.&range=all&skip=1").Body.String()
	if !strings.Contains(body, `"parents":[]`) || !strings.Contains(body, `"body":""`) {
		t.Fatalf("root commit JSON = %s", body)
	}
	if body := request(t, app, "/api/commits?project=.&head="+initial.String()).Body.String(); !strings.Contains(body, `"commits":[]`) {
		t.Fatalf("empty range JSON = %s", body)
	}
}

func TestCommitsRejectsInvalidQueries(t *testing.T) {
	root, _, _ := historyRepo(t)
	app := newTestApp(t, root)
	for _, query := range []string{
		"range=main",
		"skip=-1",
		"skip=one",
		"limit=0",
		"limit=101",
		"limit=ten",
		"head=abc123",
		"head=" + strings.Repeat("A", 40),
	} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/commits?project=.&"+query, &failure); status != http.StatusBadRequest || failure.Error == "" {
			t.Errorf("%s = %d %+v", query, status, failure)
		}
	}
	var failure errorResponse
	if status := requestJSON(t, app, "/api/commits?project=.&head="+strings.Repeat("1", 40), &failure); status != http.StatusNotFound || failure.Error != commitUnavailableMessage {
		t.Fatalf("unknown head = %d %+v", status, failure)
	}
	var page commitsResponse
	if status := requestJSON(t, app, "/api/commits?project=.&range=all&limit=100&skip=5", &page); status != http.StatusOK || len(page.Commits) != 0 || page.More {
		t.Fatalf("past the end = %d %+v", status, page)
	}
}

func TestCommitsReportsFolders(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "README.md"), "# Folder")
	app := newTestApp(t, root)

	response := request(t, app, "/api/commits?project=.&range=all")
	want := `{"repository":false,"branch":"","base":"","range":"all","head":"","ahead":0,"commits":[],"more":false}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("folder commits = %d %s", response.Code, response.Body.String())
	}
	var failure errorResponse
	for _, target := range []string{
		"/api/commit-files?project=.&commit=" + strings.Repeat("1", 40),
		"/api/diff?project=.&path=README.md&commit=" + strings.Repeat("1", 40),
	} {
		if status := requestJSON(t, app, target, &failure); status != http.StatusNotFound {
			t.Errorf("%s = %d %+v", target, status, failure)
		}
	}
	if status := requestJSON(t, app, "/api/commits", &failure); status != http.StatusBadRequest || failure.Error != noProjectMessage {
		t.Fatalf("no project = %d %+v", status, failure)
	}
}

func TestCommitFilesListsChangesBeyondFilenameScope(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newScopedApp(t, root, "*.md")

	var got commitFilesResponse
	if status := requestJSON(t, app, "/api/commit-files?project=.&commit="+change.String(), &got); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	want := commitFilesResponse{Commit: change.String(), Files: []git.Change{
		{Path: "added.go", Status: git.Added},
		{Path: "data.bin", Status: git.Modified},
		{Path: "doc.md", Status: git.Modified},
		{Path: "gone.md", Status: git.Deleted},
		{Path: "large.txt", Status: git.Modified},
		{Path: "moved.md", OldPath: "old.md", Status: git.Renamed},
		{Path: "scope/x.zzz", Status: git.Modified},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %+v\nwant  %+v", got, want)
	}
	if status := requestJSON(t, app, "/api/commit-files?project=.&commit="+initial.String(), &got); status != http.StatusOK || len(got.Files) != 7 || got.Truncated {
		t.Fatalf("root files = %d %+v", status, got)
	}

	for _, test := range []struct {
		query  string
		status int
	}{
		{"", http.StatusBadRequest},
		{"&commit=HEAD", http.StatusBadRequest},
		{"&commit=" + change.String()[:7], http.StatusBadRequest},
		{"&commit=" + strings.Repeat("1", 40), http.StatusNotFound},
	} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/commit-files?project=."+test.query, &failure); status != test.status || failure.Error == "" {
			t.Errorf("%s = %d %+v", test.query, status, failure)
		}
	}
}

func TestCommitDiffReadsParentAndCommitSides(t *testing.T) {
	root, _, change := historyRepo(t)
	app := newScopedApp(t, root, "*.md")
	markdown := app.config.Types.Lookup("doc.md").Type()
	commit := "commit=" + change.String() + "&"

	for _, test := range []struct {
		query string
		want  diffResponse
	}{
		{"path=doc.md", diffResponse{Path: "doc.md", Status: git.Modified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}},
		{"path=added.go", diffResponse{Path: "added.go", Status: git.Added, DocumentType: app.config.Types.Lookup("added.go").Type(), Modified: text("package main\n")}},
		{"path=gone.md", diffResponse{Path: "gone.md", Status: git.Deleted, DocumentType: markdown, Original: text("# Gone\n")}},
		{"path=moved.md&oldPath=old.md", diffResponse{Path: "moved.md", OldPath: "old.md", Status: git.Renamed, DocumentType: markdown, Original: text("# Moved\n"), Modified: text("# Moved\n")}},
		// Commit diff tabs remember only the path; the rename is found again.
		{"path=moved.md", diffResponse{Path: "moved.md", OldPath: "old.md", Status: git.Renamed, DocumentType: markdown, Original: text("# Moved\n"), Modified: text("# Moved\n")}},
		{"path=scope/x.zzz", diffResponse{Path: "scope/x.zzz", Status: git.Modified, DocumentType: app.config.Types.Lookup("scope/x.zzz").Type(), Original: text("outside the filename scope\n"), Modified: text("changed\n")}},
		{"path=data.bin", diffResponse{Path: "data.bin", Status: git.Modified, DocumentType: app.config.Types.Lookup("data.bin").Type(),
			Original: &diffSide{Size: 11, Binary: true}, Modified: &diffSide{Size: 11, Binary: true}}},
		{"path=large.txt", diffResponse{Path: "large.txt", Status: git.Modified, DocumentType: app.config.Types.Lookup("large.txt").Type(),
			Original: &diffSide{Size: document.PreviewLimit + 1, TooLarge: true}, Modified: &diffSide{Size: document.PreviewLimit + 1, TooLarge: true}}},
	} {
		if got := diff(t, app, commit+test.query); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s = %s\nwant %s", test.query, describeDiff(got), describeDiff(test.want))
		}
	}

	// The working tree diff still compares HEAD with the disk.
	if got, want := diff(t, app, "path=doc.md"), (diffResponse{Path: "doc.md", Status: git.Modified, DocumentType: markdown, Original: text("# After\n"), Modified: text("# Uncommitted\n")}); !reflect.DeepEqual(got, want) {
		t.Errorf("working tree diff = %s", describeDiff(got))
	}
}

func TestCommitDiffRejectsPathsOutsideTheCommit(t *testing.T) {
	root, _, change := historyRepo(t)
	app := newTestApp(t, root)
	commit := "commit=" + change.String() + "&"

	for _, query := range []string{
		commit + "path=same.md",
		commit + "path=old.md",
		commit + "path=missing.md",
		commit + "path=../doc.md",
		commit + "path=.git/HEAD",
		commit + "path=moved.md&oldPath=gone.md",
		commit + "path=doc.md&oldPath=old.md",
		"commit=" + strings.Repeat("1", 40) + "&path=doc.md",
	} {
		var failure diffErrorResponse
		if status := requestJSON(t, app, "/api/diff?project=.&"+query, &failure); status != http.StatusNotFound || failure != (diffErrorResponse{Error: changeUnavailableMessage}) {
			t.Errorf("%s = %d %+v", query, status, failure)
		}
	}
	for _, query := range []string{"commit=&path=doc.md", "commit=HEAD&path=doc.md", "commit=" + strings.ToUpper(change.String()) + "&path=doc.md", commit} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/diff?project=.&"+query, &failure); status != http.StatusBadRequest {
			t.Errorf("%s = %d %+v", query, status, failure)
		}
	}
}
