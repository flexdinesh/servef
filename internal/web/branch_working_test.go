package web

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/flexdinesh/servef/internal/git"
)

// historyRepo's feature branch is one commit ahead of master, with doc.md
// edited and same.md deleted on top, uncommitted.
var branchWorkingChanges = []git.Change{
	{Path: "added.go", Status: git.Added},
	{Path: "data.bin", Status: git.Modified},
	{Path: "doc.md", Status: git.Modified},
	{Path: "gone.md", Status: git.Deleted},
	{Path: "large.txt", Status: git.Modified},
	{Path: "moved.md", OldPath: "old.md", Status: git.Renamed},
	{Path: "same.md", Status: git.Deleted},
	{Path: "scope/x.zzz", Status: git.Modified},
}

func TestBranchFilesWithUncommittedListMergeBaseToWorkingTree(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newScopedApp(t, root, "*.md")

	var got branchFilesResponse
	if status := requestJSON(t, app, "/api/branch-files?project=.&uncommitted=1", &got); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	want := branchFilesResponse{Repository: true, Branch: "feature", Base: "master", MergeBase: initial.String(), Head: change.String(), Ahead: 1,
		Files: branchWorkingChanges, Warnings: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("branch files = %+v\nwant %+v", got, want)
	}
	var based branchFilesResponse
	if status := requestJSON(t, app, "/api/branch-files?project=.&uncommitted=1&base=master", &based); status != http.StatusOK || !reflect.DeepEqual(based, want) {
		t.Fatalf("base=master = %d %+v", status, based)
	}
}

func TestBranchDiffWithUncommittedReadsMergeBaseAndWorktree(t *testing.T) {
	root, _, _ := historyRepo(t)
	app := newScopedApp(t, root, "*.md")

	var modified diffResponse
	if status := requestJSON(t, app, "/api/diff?project=.&branch=1&uncommitted=1&path=doc.md", &modified); status != http.StatusOK {
		t.Fatalf("doc.md status = %d", status)
	}
	if modified.Status != git.Modified || content(modified.Original) != "# Before\n" || content(modified.Modified) != "# Uncommitted\n" {
		t.Fatalf("doc.md = %+v", modified)
	}
	var deleted diffResponse
	if status := requestJSON(t, app, "/api/diff?project=.&branch=1&uncommitted=1&path=same.md", &deleted); status != http.StatusOK {
		t.Fatalf("same.md status = %d", status)
	}
	if deleted.Status != git.Deleted || content(deleted.Original) != "# Same\n" || deleted.Modified != nil {
		t.Fatalf("same.md = %+v", deleted)
	}
	var renamed diffResponse
	if status := requestJSON(t, app, "/api/diff?project=.&branch=1&uncommitted=1&path=moved.md&oldPath=old.md", &renamed); status != http.StatusOK {
		t.Fatalf("moved.md status = %d", status)
	}
	if renamed.Status != git.Renamed || content(renamed.Original) != "# Moved\n" || content(renamed.Modified) != "# Moved\n" {
		t.Fatalf("moved.md = %+v", renamed)
	}
	for _, path := range []string{"missing.md", "../doc.md"} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/diff?project=.&branch=1&uncommitted=1&path="+path, &failure); status != http.StatusNotFound || failure.Code != unchangedCode {
			t.Errorf("%s = %d %+v", path, status, failure)
		}
	}
	var unknown errorResponse
	if status := requestJSON(t, app, "/api/diff?project=.&branch=1&uncommitted=1&base=nope&path=doc.md", &unknown); status != http.StatusBadRequest || unknown.Code != unknownBaseCode {
		t.Fatalf("unknown base = %d %+v", status, unknown)
	}
}

func TestCompareStatsWithUncommittedCountWorktreeSides(t *testing.T) {
	root, _, _ := historyRepo(t)
	app := newScopedApp(t, root, "*.md")

	assertStats(t, "branch uncommitted", compareStats(t, app, "compare=branch&uncommitted=1"), compareStatsResponse{Files: []fileStats{
		counted("added.go", 1, 0, blobHash("package main\n")),
		uncounted("data.bin", statsBinary, blobHash("work\x00binary")),
		counted("doc.md", 1, 1, blobHash("# Uncommitted\n")),
		counted("gone.md", 0, 1, blobHash("# Gone\n")),
		// The worktree side is over the limit, so it is never read or hashed.
		uncounted("large.txt", statsTooLarge, ""),
		counted("moved.md", 0, 0, blobHash("# Moved\n")),
		counted("same.md", 0, 1, blobHash("# Same\n")),
		counted("scope/x.zzz", 1, 1, blobHash("changed\n")),
	}})
}

func TestUncommittedRejectsInvalidQueries(t *testing.T) {
	root, _, change := historyRepo(t)
	app := newTestApp(t, root)
	for _, target := range []string{
		"/api/branch-files?project=.&uncommitted=2",
		"/api/branch-files?project=.&uncommitted=1&head=feature",
		"/api/diff?project=.&uncommitted=1&path=doc.md",
		"/api/diff?project=.&commit=" + change.String() + "&uncommitted=1&path=doc.md",
		"/api/diff?project=.&branch=1&uncommitted=true&path=doc.md",
		"/api/diff?project=.&branch=1&uncommitted=1&head=feature&path=doc.md",
		"/api/compare-stats?project=.&compare=working&uncommitted=1",
		"/api/compare-stats?project=.&compare=branch&uncommitted=0",
		"/api/compare-stats?project=.&compare=branch&uncommitted=1&head=feature",
	} {
		var failure errorResponse
		if status := requestJSON(t, app, target, &failure); status != http.StatusBadRequest || failure.Error == "" || failure.Code != "" {
			t.Errorf("%s = %d %+v", target, status, failure)
		}
	}
}

func content(side *diffSide) string {
	if side == nil || side.Content == nil {
		return ""
	}
	return *side.Content
}
