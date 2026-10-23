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
		var failure errorResponse
		if status := requestJSON(t, app, "/api/diff?project=.&"+query, &failure); status != http.StatusNotFound || failure != (errorResponse{Error: changeUnavailableMessage}) {
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

func TestBranchFilesListMergeBaseToHeadChanges(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newScopedApp(t, root, "*.md")

	var got branchFilesResponse
	if status := requestJSON(t, app, "/api/branch-files?project=.", &got); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	want := branchFilesResponse{Repository: true, Branch: "feature", Base: "master", MergeBase: initial.String(), Head: change.String(), Ahead: 1, Files: []git.Change{
		{Path: "added.go", Status: git.Added},
		{Path: "data.bin", Status: git.Modified},
		{Path: "doc.md", Status: git.Modified},
		{Path: "gone.md", Status: git.Deleted},
		{Path: "large.txt", Status: git.Modified},
		{Path: "moved.md", OldPath: "old.md", Status: git.Renamed},
		{Path: "scope/x.zzz", Status: git.Modified},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("branch files = %+v\nwant %+v", got, want)
	}
	if response := request(t, app, "/api/branch-files?project=."); response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}

	// On the base, nothing is ahead and there is no merge base.
	gitRepo, err := gogit.PlainOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := gitRepo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("master"), change)); err != nil {
		t.Fatal(err)
	}
	body := request(t, app, "/api/branch-files?project=.").Body.String()
	if want := `{"repository":true,"branch":"feature","base":"master","mergeBase":"","head":"` + change.String() + `","ahead":0,"files":[],"truncated":false}`; strings.TrimSpace(body) != want {
		t.Fatalf("nothing ahead = %s", body)
	}
	var unchanged errorResponse
	if status := requestJSON(t, app, "/api/diff?project=.&branch=1&path=doc.md", &unchanged); status != http.StatusNotFound || unchanged.Code != "unchanged" || unchanged.Error == "" {
		t.Fatalf("nothing ahead diff = %d %+v", status, unchanged)
	}
}

func TestRangeFilesCompareOldestParentWithNewest(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newTestApp(t, root)

	var single rangeFilesResponse
	if status := requestJSON(t, app, "/api/range-files?project=.&from="+change.String()+"&to="+change.String(), &single); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	var commit commitFilesResponse
	requestJSON(t, app, "/api/commit-files?project=.&commit="+change.String(), &commit)
	if single.From != change.String() || single.To != change.String() || !reflect.DeepEqual(single.Files, commit.Files) || single.Truncated {
		t.Fatalf("single commit range = %+v, commit = %+v", single, commit)
	}
	// A root oldest commit compares with an empty tree.
	var rooted rangeFilesResponse
	if status := requestJSON(t, app, "/api/range-files?project=.&from="+initial.String()+"&to="+change.String(), &rooted); status != http.StatusOK {
		t.Fatalf("root range status = %d", status)
	}
	var names []string
	for _, file := range rooted.Files {
		if file.Status != git.Added || file.OldPath != "" {
			t.Errorf("root range change %+v, want added", file)
		}
		names = append(names, file.Path)
	}
	if want := []string{"added.go", "data.bin", "doc.md", "large.txt", "moved.md", "same.md", "scope/x.zzz"}; !reflect.DeepEqual(names, want) {
		t.Errorf("root range paths = %v", names)
	}

	for _, test := range []struct {
		query  string
		status int
	}{
		{"", http.StatusBadRequest},
		{"&from=" + change.String(), http.StatusBadRequest},
		{"&from=HEAD&to=" + change.String(), http.StatusBadRequest},
		{"&from=" + change.String() + "&to=" + change.String()[:7], http.StatusBadRequest},
		{"&from=" + strings.Repeat("1", 40) + "&to=" + change.String(), http.StatusNotFound},
		{"&from=" + change.String() + "&to=" + strings.Repeat("1", 40), http.StatusNotFound},
	} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/range-files?project=."+test.query, &failure); status != test.status || failure.Error == "" {
			t.Errorf("%s = %d %+v", test.query, status, failure)
		}
	}
}

func TestBranchAndRangeFilesInFolders(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "README.md"), "# Folder")
	app := newTestApp(t, root)

	response := request(t, app, "/api/branch-files?project=.")
	want := `{"repository":false,"branch":"","base":"","mergeBase":"","head":"","ahead":0,"files":[],"truncated":false}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("folder branch files = %d %s", response.Code, response.Body.String())
	}
	hash := strings.Repeat("1", 40)
	for _, target := range []string{
		"/api/range-files?project=.&from=" + hash + "&to=" + hash,
		"/api/diff?project=.&path=README.md&from=" + hash + "&to=" + hash,
		"/api/diff?project=.&path=README.md&branch=1",
	} {
		var failure errorResponse
		if status := requestJSON(t, app, target, &failure); status != http.StatusNotFound || failure.Error == "" {
			t.Errorf("%s = %d %+v", target, status, failure)
		}
	}
}

func TestRangeAndBranchDiffsReadEndpointSides(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newScopedApp(t, root, "*.md")
	markdown := app.config.Types.Lookup("doc.md").Type()
	ranged := "from=" + change.String() + "&to=" + change.String() + "&"

	for _, test := range []struct {
		query string
		want  diffResponse
	}{
		{ranged + "path=doc.md", diffResponse{Path: "doc.md", Status: git.Modified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}},
		{ranged + "path=moved.md", diffResponse{Path: "moved.md", OldPath: "old.md", Status: git.Renamed, DocumentType: markdown, Original: text("# Moved\n"), Modified: text("# Moved\n")}},
		{ranged + "path=gone.md", diffResponse{Path: "gone.md", Status: git.Deleted, DocumentType: markdown, Original: text("# Gone\n")}},
		{"from=" + initial.String() + "&to=" + change.String() + "&path=doc.md", diffResponse{Path: "doc.md", Status: git.Added, DocumentType: markdown, Modified: text("# After\n")}},
		{"branch=1&path=doc.md", diffResponse{Path: "doc.md", Status: git.Modified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}},
		{"branch=1&path=moved.md&oldPath=old.md", diffResponse{Path: "moved.md", OldPath: "old.md", Status: git.Renamed, DocumentType: markdown, Original: text("# Moved\n"), Modified: text("# Moved\n")}},
		{"branch=1&path=added.go", diffResponse{Path: "added.go", Status: git.Added, DocumentType: app.config.Types.Lookup("added.go").Type(), Modified: text("package main\n")}},
		{"branch=1&path=large.txt", diffResponse{Path: "large.txt", Status: git.Modified, DocumentType: app.config.Types.Lookup("large.txt").Type(),
			Original: &diffSide{Size: document.PreviewLimit + 1, TooLarge: true}, Modified: &diffSide{Size: document.PreviewLimit + 1, TooLarge: true}}},
	} {
		if got := diff(t, app, test.query); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s = %s\nwant %s", test.query, describeDiff(got), describeDiff(test.want))
		}
	}

	for _, query := range []string{ranged + "path=same.md", ranged + "path=old.md", ranged + "path=doc.md&oldPath=old.md", ranged + "path=../doc.md"} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/diff?project=.&"+query, &failure); status != http.StatusNotFound || failure != (errorResponse{Error: changeUnavailableMessage}) {
			t.Errorf("%s = %d %+v", query, status, failure)
		}
	}
	for _, query := range []string{"branch=1&path=same.md", "branch=1&path=old.md", "branch=1&path=missing.md"} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/diff?project=.&"+query, &failure); status != http.StatusNotFound || failure.Code != "unchanged" || failure.Error == "" {
			t.Errorf("%s = %d %+v", query, status, failure)
		}
	}
	hash := change.String()
	for _, query := range []string{
		"commit=" + hash + "&branch=1&path=doc.md",
		"commit=" + hash + "&from=" + hash + "&to=" + hash + "&path=doc.md",
		"branch=1&from=" + hash + "&to=" + hash + "&path=doc.md",
		"branch=1&to=" + hash + "&path=doc.md",
		"from=" + hash + "&path=doc.md",
		"to=" + hash + "&path=doc.md",
		"branch=0&path=doc.md",
		"from=HEAD&to=" + hash + "&path=doc.md",
		"from=" + hash + "&to=" + hash,
		"branch=1",
	} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/diff?project=.&"+query, &failure); status != http.StatusBadRequest || failure.Error == "" {
			t.Errorf("%s = %d %+v", query, status, failure)
		}
	}
}

// The branch diff is resolved on every request, so a new commit shows next.
func TestBranchDiffFollowsNewCommits(t *testing.T) {
	root, _, _ := historyRepo(t)
	app := newScopedApp(t, root, "*.md")
	markdown := app.config.Types.Lookup("doc.md").Type()

	if got, want := diff(t, app, "branch=1&path=doc.md"), (diffResponse{Path: "doc.md", Status: git.Modified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}); !reflect.DeepEqual(got, want) {
		t.Fatalf("before commit = %s", describeDiff(got))
	}
	var unchanged errorResponse
	if status := requestJSON(t, app, "/api/diff?project=.&branch=1&path=fresh.md", &unchanged); status != http.StatusNotFound || unchanged.Code != "unchanged" {
		t.Fatalf("fresh.md before commit = %d %+v", status, unchanged)
	}

	gitRepo, err := gogit.PlainOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := gitRepo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	writeMarkdown(t, filepath.Join(root, "fresh.md"), "# Fresh\n")
	for _, name := range []string{"doc.md", "fresh.md"} {
		if _, err := worktree.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	signature := &object.Signature{Name: "servef", Email: "servef@example.com", When: time.Unix(1700000200, 0)}
	if _, err := worktree.Commit("More", &gogit.CommitOptions{Author: signature, Committer: signature}); err != nil {
		t.Fatal(err)
	}

	if got, want := diff(t, app, "branch=1&path=doc.md"), (diffResponse{Path: "doc.md", Status: git.Modified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# Uncommitted\n")}); !reflect.DeepEqual(got, want) {
		t.Fatalf("after commit = %s", describeDiff(got))
	}
	if got, want := diff(t, app, "branch=1&path=fresh.md"), (diffResponse{Path: "fresh.md", Status: git.Added, DocumentType: markdown, Modified: text("# Fresh\n")}); !reflect.DeepEqual(got, want) {
		t.Fatalf("fresh.md after commit = %s", describeDiff(got))
	}
	var files branchFilesResponse
	if requestJSON(t, app, "/api/branch-files?project=.", &files); files.Ahead != 2 {
		t.Fatalf("ahead after commit = %d", files.Ahead)
	}
}

// Frontend history fixtures must decode into the server's response shapes
// and agree on the branch they describe.
func TestHistoryFixturesMatchServerContract(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "api", "default")
	var commits, all, folder commitsResponse
	decodeFixture(t, filepath.Join(dir, "commits.json"), &commits)
	decodeFixture(t, filepath.Join(dir, "commits-all.json"), &all)
	decodeFixture(t, filepath.Join(dir, "commits-not-repo.json"), &folder)
	var files commitFilesResponse
	decodeFixture(t, filepath.Join(dir, "commit-files.json"), &files)
	var branch branchFilesResponse
	decodeFixture(t, filepath.Join(dir, "branch-files.json"), &branch)
	var ranged rangeFilesResponse
	decodeFixture(t, filepath.Join(dir, "range-files.json"), &ranged)

	for _, hash := range []string{branch.MergeBase, branch.Head, ranged.From, ranged.To} {
		if _, ok := git.ParseHash(hash); !ok {
			t.Errorf("fixture hash %q is not a full hash", hash)
		}
	}
	if branch.Base != commits.Base || branch.Head != commits.Head || branch.Ahead != commits.Ahead || branch.Branch != commits.Branch {
		t.Errorf("branch-files = %+v, commits = base %q head %q ahead %d", branch, commits.Base, commits.Head, commits.Ahead)
	}
	if oldest := commits.Commits[len(commits.Commits)-1]; len(oldest.Parents) == 0 || branch.MergeBase != oldest.Parents[0] || ranged.From != oldest.Hash || ranged.To != commits.Commits[0].Hash {
		t.Errorf("merge base %s and range %s..%s do not match commits.json", branch.MergeBase, ranged.From, ranged.To)
	}
}
