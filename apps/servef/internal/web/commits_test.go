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
	"github.com/flexdinesh/servef/pkg/contract"
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
		"scope/x.zzz": "unregistered extension\n",
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
	app := newTestApp(t, root)

	changeCommit := contract.Commit{
		Hash:       change.String(),
		Parents:    []string{initial.String()},
		Subject:    "Change everything",
		Body:       "With a body.",
		Author:     contract.Author{Name: "servef", Email: "servef@example.com"},
		AuthorTime: "2023-11-15T09:15:00+11:00",
	}
	initialCommit := contract.Commit{
		Hash:       initial.String(),
		Parents:    []string{},
		Subject:    "initial",
		Author:     contract.Author{Name: "servef", Email: "servef@example.com"},
		AuthorTime: "2023-11-15T09:13:20+11:00",
	}
	for _, test := range []struct {
		query string
		want  contract.CommitsResponse
	}{
		{"", contract.CommitsResponse{Repository: true, Branch: "feature", Base: "master", Range: contract.RangeBranch, Head: change.String(), Ahead: 1, Commits: []contract.Commit{changeCommit}}},
		{"&range=all", contract.CommitsResponse{Repository: true, Branch: "feature", Base: "master", Range: contract.RangeAll, Head: change.String(), Ahead: 1, Commits: []contract.Commit{changeCommit, initialCommit}}},
		{"&range=all&limit=1", contract.CommitsResponse{Repository: true, Branch: "feature", Base: "master", Range: contract.RangeAll, Head: change.String(), Ahead: 1, Commits: []contract.Commit{changeCommit}, More: true}},
		{"&range=all&skip=1&head=" + change.String(), contract.CommitsResponse{Repository: true, Branch: "feature", Base: "master", Range: contract.RangeAll, Head: change.String(), Ahead: 1, Commits: []contract.Commit{initialCommit}}},
		{"&range=branch&head=" + initial.String(), contract.CommitsResponse{Repository: true, Branch: "feature", Base: "master", Range: contract.RangeBranch, Head: initial.String(), Commits: []contract.Commit{}}},
	} {
		var got contract.CommitsResponse
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
	var page contract.CommitsResponse
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

func TestCommitFilesListsChanges(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newTestApp(t, root)

	var got contract.CommitFilesResponse
	if status := requestJSON(t, app, "/api/commit-files?project=.&commit="+change.String(), &got); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	want := contract.CommitFilesResponse{Commit: change.String(), Files: []contract.ChangedFile{
		{Path: "added.go", Status: contract.ChangeAdded},
		{Path: "data.bin", Status: contract.ChangeModified},
		{Path: "doc.md", Status: contract.ChangeModified},
		{Path: "gone.md", Status: contract.ChangeDeleted},
		{Path: "large.txt", Status: contract.ChangeModified},
		{Path: "moved.md", OldPath: "old.md", Status: contract.ChangeRenamed},
		{Path: "scope/x.zzz", Status: contract.ChangeModified},
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
	app := newTestApp(t, root)
	markdown := documentType(app, "doc.md")
	commit := "commit=" + change.String() + "&"

	for _, test := range []struct {
		query string
		want  contract.DiffResponse
	}{
		{"path=doc.md", contract.DiffResponse{Path: "doc.md", Status: contract.ChangeModified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}},
		{"path=added.go", contract.DiffResponse{Path: "added.go", Status: contract.ChangeAdded, DocumentType: documentType(app, "added.go"), Modified: text("package main\n")}},
		{"path=gone.md", contract.DiffResponse{Path: "gone.md", Status: contract.ChangeDeleted, DocumentType: markdown, Original: text("# Gone\n")}},
		{"path=moved.md&oldPath=old.md", contract.DiffResponse{Path: "moved.md", OldPath: "old.md", Status: contract.ChangeRenamed, DocumentType: markdown, Original: text("# Moved\n"), Modified: text("# Moved\n")}},
		// Commit diff tabs remember only the path; the rename is found again.
		{"path=moved.md", contract.DiffResponse{Path: "moved.md", OldPath: "old.md", Status: contract.ChangeRenamed, DocumentType: markdown, Original: text("# Moved\n"), Modified: text("# Moved\n")}},
		{"path=scope/x.zzz", contract.DiffResponse{Path: "scope/x.zzz", Status: contract.ChangeModified, DocumentType: documentType(app, "scope/x.zzz"), Original: text("unregistered extension\n"), Modified: text("changed\n")}},
		{"path=data.bin", contract.DiffResponse{Path: "data.bin", Status: contract.ChangeModified, DocumentType: documentType(app, "data.bin"),
			Original: &contract.DiffSide{Size: 11, Binary: true}, Modified: &contract.DiffSide{Size: 11, Binary: true}}},
		{"path=large.txt", contract.DiffResponse{Path: "large.txt", Status: contract.ChangeModified, DocumentType: documentType(app, "large.txt"),
			Original: &contract.DiffSide{Size: document.PreviewLimit + 1, TooLarge: true}, Modified: &contract.DiffSide{Size: document.PreviewLimit + 1, TooLarge: true}}},
	} {
		if got := diff(t, app, commit+test.query); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s = %s\nwant %s", test.query, describeDiff(got), describeDiff(test.want))
		}
	}

	// The working tree diff still compares HEAD with the disk.
	if got, want := diff(t, app, "path=doc.md"), (contract.DiffResponse{Path: "doc.md", Status: contract.ChangeModified, DocumentType: markdown, Original: text("# After\n"), Modified: text("# Uncommitted\n")}); !reflect.DeepEqual(got, want) {
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
	app := newTestApp(t, root)

	var got contract.BranchFilesResponse
	if status := requestJSON(t, app, "/api/branch-files?project=.", &got); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	want := contract.BranchFilesResponse{Repository: true, Branch: "feature", Base: "master", MergeBase: initial.String(), Head: change.String(), Ahead: 1, Files: []contract.ChangedFile{
		{Path: "added.go", Status: contract.ChangeAdded},
		{Path: "data.bin", Status: contract.ChangeModified},
		{Path: "doc.md", Status: contract.ChangeModified},
		{Path: "gone.md", Status: contract.ChangeDeleted},
		{Path: "large.txt", Status: contract.ChangeModified},
		{Path: "moved.md", OldPath: "old.md", Status: contract.ChangeRenamed},
		{Path: "scope/x.zzz", Status: contract.ChangeModified},
	}, Warnings: []string{}}
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
	if want := `{"repository":true,"branch":"feature","base":"master","mergeBase":"","head":"` + change.String() + `","ahead":0,"files":[],"truncated":false,"warnings":[]}`; strings.TrimSpace(body) != want {
		t.Fatalf("nothing ahead = %s", body)
	}
	// With uncommitted changes, nothing ahead lists the working tree's
	// changes since HEAD.
	var working contract.BranchFilesResponse
	requestJSON(t, app, "/api/branch-files?project=.&uncommitted=1", &working)
	var changes contract.ChangesResponse
	requestJSON(t, app, "/api/changes?project=.", &changes)
	if working.Ahead != 0 || len(working.Files) == 0 || !reflect.DeepEqual(working.Files, changes.Files) {
		t.Fatalf("nothing ahead with uncommitted = %+v, changes %+v", working.Files, changes.Files)
	}
	var unchanged errorResponse
	if status := requestJSON(t, app, "/api/diff?project=.&branch=1&path=doc.md", &unchanged); status != http.StatusNotFound || unchanged.Code != "unchanged" || unchanged.Error == "" {
		t.Fatalf("nothing ahead diff = %d %+v", status, unchanged)
	}
}

func TestRangeFilesCompareOldestParentWithNewest(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newTestApp(t, root)

	var single contract.RangeFilesResponse
	if status := requestJSON(t, app, "/api/range-files?project=.&from="+change.String()+"&to="+change.String(), &single); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	var commit contract.CommitFilesResponse
	requestJSON(t, app, "/api/commit-files?project=.&commit="+change.String(), &commit)
	if single.From != change.String() || single.To != change.String() || !reflect.DeepEqual(single.Files, commit.Files) || single.Truncated {
		t.Fatalf("single commit range = %+v, commit = %+v", single, commit)
	}
	// A root oldest commit compares with an empty tree.
	var rooted contract.RangeFilesResponse
	if status := requestJSON(t, app, "/api/range-files?project=.&from="+initial.String()+"&to="+change.String(), &rooted); status != http.StatusOK {
		t.Fatalf("root range status = %d", status)
	}
	var names []string
	for _, file := range rooted.Files {
		if file.Status != contract.ChangeAdded || file.OldPath != "" {
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
	want := `{"repository":false,"branch":"","base":"","mergeBase":"","head":"","ahead":0,"files":[],"truncated":false,"warnings":[]}`
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
	app := newTestApp(t, root)
	markdown := documentType(app, "doc.md")
	ranged := "from=" + change.String() + "&to=" + change.String() + "&"

	for _, test := range []struct {
		query string
		want  contract.DiffResponse
	}{
		{ranged + "path=doc.md", contract.DiffResponse{Path: "doc.md", Status: contract.ChangeModified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}},
		{ranged + "path=moved.md", contract.DiffResponse{Path: "moved.md", OldPath: "old.md", Status: contract.ChangeRenamed, DocumentType: markdown, Original: text("# Moved\n"), Modified: text("# Moved\n")}},
		{ranged + "path=gone.md", contract.DiffResponse{Path: "gone.md", Status: contract.ChangeDeleted, DocumentType: markdown, Original: text("# Gone\n")}},
		{"from=" + initial.String() + "&to=" + change.String() + "&path=doc.md", contract.DiffResponse{Path: "doc.md", Status: contract.ChangeAdded, DocumentType: markdown, Modified: text("# After\n")}},
		{"branch=1&path=doc.md", contract.DiffResponse{Path: "doc.md", Status: contract.ChangeModified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}},
		{"branch=1&path=moved.md&oldPath=old.md", contract.DiffResponse{Path: "moved.md", OldPath: "old.md", Status: contract.ChangeRenamed, DocumentType: markdown, Original: text("# Moved\n"), Modified: text("# Moved\n")}},
		{"branch=1&path=added.go", contract.DiffResponse{Path: "added.go", Status: contract.ChangeAdded, DocumentType: documentType(app, "added.go"), Modified: text("package main\n")}},
		{"branch=1&path=large.txt", contract.DiffResponse{Path: "large.txt", Status: contract.ChangeModified, DocumentType: documentType(app, "large.txt"),
			Original: &contract.DiffSide{Size: document.PreviewLimit + 1, TooLarge: true}, Modified: &contract.DiffSide{Size: document.PreviewLimit + 1, TooLarge: true}}},
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
	app := newTestApp(t, root)
	markdown := documentType(app, "doc.md")

	if got, want := diff(t, app, "branch=1&path=doc.md"), (contract.DiffResponse{Path: "doc.md", Status: contract.ChangeModified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}); !reflect.DeepEqual(got, want) {
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

	if got, want := diff(t, app, "branch=1&path=doc.md"), (contract.DiffResponse{Path: "doc.md", Status: contract.ChangeModified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# Uncommitted\n")}); !reflect.DeepEqual(got, want) {
		t.Fatalf("after commit = %s", describeDiff(got))
	}
	if got, want := diff(t, app, "branch=1&path=fresh.md"), (contract.DiffResponse{Path: "fresh.md", Status: contract.ChangeAdded, DocumentType: markdown, Modified: text("# Fresh\n")}); !reflect.DeepEqual(got, want) {
		t.Fatalf("fresh.md after commit = %s", describeDiff(got))
	}
	var files contract.BranchFilesResponse
	if requestJSON(t, app, "/api/branch-files?project=.", &files); files.Ahead != 2 {
		t.Fatalf("ahead after commit = %d", files.Ahead)
	}
}

// Frontend history fixtures must decode into the server's response shapes
// and agree on the branch they describe.
func TestHistoryFixturesMatchServerContract(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "testdata", "api", "default")
	var commits, all, folder contract.CommitsResponse
	decodeFixture(t, filepath.Join(dir, "commits.json"), &commits)
	decodeFixture(t, filepath.Join(dir, "commits-all.json"), &all)
	decodeFixture(t, filepath.Join(dir, "commits-not-repo.json"), &folder)
	var files contract.CommitFilesResponse
	decodeFixture(t, filepath.Join(dir, "commit-files.json"), &files)
	var branch contract.BranchFilesResponse
	decodeFixture(t, filepath.Join(dir, "branch-files.json"), &branch)
	var ranged contract.RangeFilesResponse
	decodeFixture(t, filepath.Join(dir, "range-files.json"), &ranged)
	var stack contract.StackResponse
	decodeFixture(t, filepath.Join(dir, "branch-stack.json"), &stack)
	var refs contract.RefsResponse
	decodeFixture(t, filepath.Join(dir, "refs.json"), &refs)

	hashes := []string{branch.MergeBase, branch.Head, ranged.From, ranged.To, stack.MergeBase, stack.Head}
	for _, layer := range stack.Layers {
		hashes = append(hashes, layer.Head)
	}
	for _, hash := range hashes {
		if _, ok := git.ParseHash(hash); !ok {
			t.Errorf("fixture hash %q is not a full hash", hash)
		}
	}

	// The branch with uncommitted changes shares the branch's range and lists
	// the working tree's files, which hold the branch's, as the dev server
	// pairs it with compare-stats.json and the working tree's diffs.
	var working contract.BranchFilesResponse
	decodeFixture(t, filepath.Join(dir, "branch-files-uncommitted.json"), &working)
	var changes contract.ChangesResponse
	decodeFixture(t, filepath.Join(dir, "changes.json"), &changes)
	if working.Branch != branch.Branch || working.Base != branch.Base || working.MergeBase != branch.MergeBase || working.Head != branch.Head ||
		working.Ahead != branch.Ahead || !reflect.DeepEqual(working.Files, changes.Files) {
		t.Errorf("branch-files-uncommitted = %+v, branch-files = %+v, changes = %+v", working, branch, changes.Files)
	}

	// The stack describes branch-files' branch: its current layer is HEAD,
	// layers are listed local branches, and the aheads down to the base add
	// up to the branch's.
	if stack.Branch != branch.Branch || stack.Base != branch.Base || stack.MergeBase != branch.MergeBase || stack.Head != branch.Head {
		t.Errorf("branch-stack = %+v, branch-files = %+v", stack, branch)
	}
	local := map[string]bool{}
	for _, ref := range refs.Branches {
		local[ref.Name] = local[ref.Name] || !ref.Remote
	}
	byName := map[string]contract.StackLayer{}
	for _, layer := range stack.Layers {
		if !local[layer.Name] {
			t.Errorf("layer %q is not a local branch in refs.json", layer.Name)
		}
		byName[layer.Name] = layer
	}
	ahead := 0
	for name, steps := stack.Branch, 0; name != stack.Base && steps <= len(stack.Layers); steps++ {
		layer, ok := byName[name]
		if !ok {
			t.Fatalf("layer %q is missing from branch-stack.json", name)
		}
		if (name == stack.Branch) != layer.Current || (layer.Current && layer.Head != stack.Head) {
			t.Errorf("layer %+v does not match HEAD %s", layer, stack.Head)
		}
		ahead += layer.Ahead
		name = layer.Parent
	}
	if ahead != branch.Ahead {
		t.Errorf("stack aheads to the base = %d, branch-files ahead = %d", ahead, branch.Ahead)
	}
	if branch.Base != commits.Base || branch.Head != commits.Head || branch.Ahead != commits.Ahead || branch.Branch != commits.Branch {
		t.Errorf("branch-files = %+v, commits = base %q head %q ahead %d", branch, commits.Base, commits.Head, commits.Ahead)
	}
	if oldest := commits.Commits[len(commits.Commits)-1]; len(oldest.Parents) == 0 || branch.MergeBase != oldest.Parents[0] || ranged.From != oldest.Hash || ranged.To != commits.Commits[0].Hash {
		t.Errorf("merge base %s and range %s..%s do not match commits.json", branch.MergeBase, ranged.From, ranged.To)
	}
}
