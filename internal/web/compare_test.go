package web

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/git"
)

// blobHash is git hash-object of content, computed independently of go-git.
func blobHash(content string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(content), content)))
	return hex.EncodeToString(sum[:])
}

func counted(path string, additions, deletions int, blob string) fileStats {
	return fileStats{Path: path, Status: statsCounted, Additions: &additions, Deletions: &deletions, Blob: blob}
}

func uncounted(path string, status statsStatus, blob string) fileStats {
	return fileStats{Path: path, Status: status, Blob: blob}
}

func compareStats(t *testing.T, app *App, query string) compareStatsResponse {
	t.Helper()
	var response compareStatsResponse
	if status := requestJSON(t, app, "/api/compare-stats?project=.&"+query, &response); status != http.StatusOK {
		t.Fatalf("%s status = %d", query, status)
	}
	return response
}

func assertStats(t *testing.T, label string, got compareStatsResponse, want compareStatsResponse) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %s\nwant %s", label, describeStats(got), describeStats(want))
	}
}

func describeStats(response compareStatsResponse) string {
	var lines []string
	for _, file := range response.Files {
		line := fmt.Sprintf("%s %s %s", file.Path, file.Status, file.Blob)
		if file.Additions != nil && file.Deletions != nil {
			line += fmt.Sprintf(" +%d -%d", *file.Additions, *file.Deletions)
		}
		lines = append(lines, line)
	}
	return fmt.Sprintf("budgetExceeded %v\n  %s", response.BudgetExceeded, strings.Join(lines, "\n  "))
}

func paths[T any](items []T, path func(T) string) []string {
	result := []string{}
	for _, item := range items {
		result = append(result, path(item))
	}
	return result
}

func TestRefsListBranchesAndDefaultBase(t *testing.T) {
	root, initial, change := historyRepo(t)
	gitRepo, err := gogit.PlainOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	for name, hash := range map[string]plumbing.Hash{"refs/remotes/origin/main": initial, "refs/remotes/origin/feature": change, "refs/tags/v1": initial} {
		if err := gitRepo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(name), hash)); err != nil {
			t.Fatal(err)
		}
	}
	if err := gitRepo.Storer.SetReference(plumbing.NewSymbolicReference("refs/remotes/origin/HEAD", "refs/remotes/origin/main")); err != nil {
		t.Fatal(err)
	}
	app := newScopedApp(t, root, "*.md")

	var got refsResponse
	if status := requestJSON(t, app, "/api/refs?project=.", &got); status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	want := refsResponse{Repository: true, Branch: "feature", DefaultBase: "origin/main", Branches: []git.BranchRef{
		{Name: "feature"}, {Name: "master"}, {Name: "origin/feature", Remote: true}, {Name: "origin/main", Remote: true},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("refs = %+v\nwant %+v", got, want)
	}
}

func TestRefsReportFoldersAndProjectErrors(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "README.md"), "# Folder")
	app := newTestApp(t, root)

	response := request(t, app, "/api/refs?project=.")
	want := `{"repository":false,"branch":"","defaultBase":"","branches":[],"truncated":false}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("folder refs = %d %s", response.Code, response.Body.String())
	}
	var failure errorResponse
	if status := requestJSON(t, app, "/api/refs", &failure); status != http.StatusBadRequest || failure.Error != noProjectMessage {
		t.Fatalf("no project = %d %+v", status, failure)
	}
}

func TestBaseParameterChoosesTheBaseBranch(t *testing.T) {
	root, initial, change := historyRepo(t)
	gitRepo, err := gogit.PlainOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	// origin/feature sits at the branch's head, so nothing is ahead of it.
	if err := gitRepo.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/feature", change)); err != nil {
		t.Fatal(err)
	}
	app := newScopedApp(t, root, "*.md")
	markdown := app.config.Types.Lookup("doc.md").Type()

	// Naming the default base answers exactly as the heuristic does.
	var byDefault, named commitsResponse
	requestJSON(t, app, "/api/commits?project=.", &byDefault)
	if status := requestJSON(t, app, "/api/commits?project=.&base=master", &named); status != http.StatusOK || !reflect.DeepEqual(named, byDefault) || named.Base != "master" {
		t.Fatalf("base=master = %d %+v, default %+v", status, named, byDefault)
	}
	var defaultFiles, namedFiles branchFilesResponse
	requestJSON(t, app, "/api/branch-files?project=.", &defaultFiles)
	if status := requestJSON(t, app, "/api/branch-files?project=.&base=master", &namedFiles); status != http.StatusOK || !reflect.DeepEqual(namedFiles, defaultFiles) {
		t.Fatalf("branch-files base=master = %d %+v, default %+v", status, namedFiles, defaultFiles)
	}
	if empty := request(t, app, "/api/branch-files?project=.&base=").Body.String(); !strings.Contains(empty, `"base":"master"`) {
		t.Fatalf("empty base = %s", empty)
	}

	// A remote-tracking base the branch has not left behind.
	var remote commitsResponse
	if status := requestJSON(t, app, "/api/commits?project=.&base=origin/feature", &remote); status != http.StatusOK || remote.Base != "origin/feature" || remote.Ahead != 0 || len(remote.Commits) != 0 {
		t.Fatalf("base=origin/feature = %d %+v", status, remote)
	}
	var remoteAll commitsResponse
	if status := requestJSON(t, app, "/api/commits?project=.&range=all&base=origin/feature", &remoteAll); status != http.StatusOK || remoteAll.Base != "origin/feature" || len(remoteAll.Commits) != 2 {
		t.Fatalf("range=all base=origin/feature = %d %+v", status, remoteAll)
	}
	body := request(t, app, "/api/branch-files?project=.&base=origin/feature").Body.String()
	if want := `{"repository":true,"branch":"feature","base":"origin/feature","mergeBase":"","head":"` + change.String() + `","ahead":0,"files":[],"truncated":false,"warnings":[]}`; strings.TrimSpace(body) != want {
		t.Fatalf("branch-files base=origin/feature = %s", body)
	}
	var unchanged errorResponse
	if status := requestJSON(t, app, "/api/diff?project=.&branch=1&base=origin/feature&path=doc.md", &unchanged); status != http.StatusNotFound || unchanged.Code != unchangedCode {
		t.Fatalf("diff base=origin/feature = %d %+v", status, unchanged)
	}

	// A local base behind the branch.
	if err := gitRepo.Storer.SetReference(plumbing.NewHashReference("refs/heads/old", initial)); err != nil {
		t.Fatal(err)
	}
	if got, want := diff(t, app, "branch=1&base=old&path=doc.md"), (diffResponse{Path: "doc.md", Status: git.Modified, DocumentType: markdown, Original: text("# Before\n"), Modified: text("# After\n")}); !reflect.DeepEqual(got, want) {
		t.Fatalf("diff base=old = %s", describeDiff(got))
	}
	var local branchFilesResponse
	if status := requestJSON(t, app, "/api/branch-files?project=.&base=old", &local); status != http.StatusOK || local.Base != "old" || local.MergeBase != initial.String() || len(local.Files) != 7 {
		t.Fatalf("branch-files base=old = %d %+v", status, local)
	}

	for _, target := range []string{
		"/api/commits?project=.&base=missing",
		"/api/commits?project=.&range=all&base=missing",
		"/api/commits?project=.&base=origin/HEAD",
		"/api/commits?project=.&base=../master",
		"/api/branch-files?project=.&base=missing",
		"/api/branch-files?project=.&base=refs/heads/master",
		"/api/diff?project=.&branch=1&base=missing&path=doc.md",
		"/api/diff?project=.&branch=1&base=missing&path=../doc.md",
		"/api/compare-stats?project=.&compare=branch&base=missing",
	} {
		var failure errorResponse
		if status := requestJSON(t, app, target, &failure); status != http.StatusBadRequest || failure != (errorResponse{Error: unknownBaseMessage, Code: unknownBaseCode}) {
			t.Errorf("%s = %d %+v", target, status, failure)
		}
	}
	var failure errorResponse
	if status := requestJSON(t, app, "/api/diff?project=.&base=master&path=doc.md", &failure); status != http.StatusBadRequest || failure.Code != "" {
		t.Fatalf("base without branch = %d %+v", status, failure)
	}
}

func TestCompareStatsCountWorkingTreeChanges(t *testing.T) {
	root := changesRepo(t)
	app := newScopedApp(t, root, "*.md")

	got := compareStats(t, app, "compare=working")
	assertStats(t, "working", got, compareStatsResponse{Files: []fileStats{
		counted("added.go", 1, 0, blobHash("package main\n")),
		uncounted("data.bin", statsBinary, blobHash("work\x00binary")),
		counted("doc.md", 1, 1, blobHash("# After\n")),
		counted("gone.md", 0, 1, blobHash("# Gone\n")),
		// The worktree side is over the limit, so it is never read or hashed.
		uncounted("large.txt", statsTooLarge, ""),
		counted("moved/new.md", 0, 0, blobHash("# Moved\n")),
		counted("notes.txt", 1, 0, blobHash("loose\n")),
		counted("scope/x.zzz", 1, 1, blobHash("changed\n")),
	}})
	var changes changesResponse
	requestJSON(t, app, "/api/changes?project=.", &changes)
	if got, want := paths(got.Files, func(file fileStats) string { return file.Path }), paths(changes.Files, func(change git.Change) string { return change.Path }); !reflect.DeepEqual(got, want) {
		t.Fatalf("stats paths = %v, changes = %v", got, want)
	}
	if body := request(t, app, "/api/compare-stats?project=.&compare=working").Body.String(); !strings.Contains(body, `{"path":"large.txt","status":"tooLarge","blob":""}`) || !strings.Contains(body, `"additions":0,"deletions":0`) {
		t.Fatalf("JSON = %s", body)
	}
}

func TestCompareStatsCountHistoryChanges(t *testing.T) {
	root, initial, change := historyRepo(t)
	app := newScopedApp(t, root, "*.md")
	large := blobHash(strings.Repeat("b", document.PreviewLimit+1))

	commitStats := compareStatsResponse{Files: []fileStats{
		counted("added.go", 1, 0, blobHash("package main\n")),
		uncounted("data.bin", statsBinary, blobHash("work\x00binary")),
		counted("doc.md", 1, 1, blobHash("# After\n")),
		counted("gone.md", 0, 1, blobHash("# Gone\n")),
		// Git sides are hashed in their trees, so even unread ones carry it.
		uncounted("large.txt", statsTooLarge, large),
		counted("moved.md", 0, 0, blobHash("# Moved\n")),
		counted("scope/x.zzz", 1, 1, blobHash("changed\n")),
	}}
	assertStats(t, "commit", compareStats(t, app, "compare=commit&commit="+change.String()), commitStats)
	assertStats(t, "branch", compareStats(t, app, "compare=branch"), commitStats)
	assertStats(t, "branch base=master", compareStats(t, app, "compare=branch&base=master"), commitStats)
	assertStats(t, "single-commit range", compareStats(t, app, "compare=range&from="+change.String()+"&to="+change.String()), commitStats)
	assertStats(t, "range from root", compareStats(t, app, "compare=range&from="+initial.String()+"&to="+change.String()), compareStatsResponse{Files: []fileStats{
		counted("added.go", 1, 0, blobHash("package main\n")),
		uncounted("data.bin", statsBinary, blobHash("work\x00binary")),
		counted("doc.md", 1, 0, blobHash("# After\n")),
		uncounted("large.txt", statsTooLarge, large),
		counted("moved.md", 1, 0, blobHash("# Moved\n")),
		counted("same.md", 1, 0, blobHash("# Same\n")),
		counted("scope/x.zzz", 1, 0, blobHash("changed\n")),
	}})

	// Nothing ahead of the base lists nothing.
	gitRepo, err := gogit.PlainOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := gitRepo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("master"), change)); err != nil {
		t.Fatal(err)
	}
	if body := request(t, app, "/api/compare-stats?project=.&compare=branch").Body.String(); strings.TrimSpace(body) != `{"files":[],"budgetExceeded":false}` {
		t.Fatalf("nothing ahead = %s", body)
	}
}

// numstatRepo commits multi-line edits, then edits the worktree, so counts
// span several hunks.
func numstatRepo(t *testing.T) (string, plumbing.Hash, plumbing.Hash) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	lines := func(from, to int) string {
		var text strings.Builder
		for i := from; i <= to; i++ {
			fmt.Fprintf(&text, "line %d\n", i)
		}
		return text.String()
	}
	commit := func(message string, when int64, files map[string]string, removed ...string) plumbing.Hash {
		for name, content := range files {
			writeMarkdown(t, filepath.Join(root, name), content)
			if _, err := worktree.Add(name); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range removed {
			if _, err := worktree.Remove(name); err != nil {
				t.Fatal(err)
			}
		}
		signature := &object.Signature{Name: "servef", Email: "servef@example.com", When: time.Unix(when, 0)}
		hash, err := worktree.Commit(message, &gogit.CommitOptions{Author: signature, Committer: signature})
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	first := commit("first", 1700000000, map[string]string{"a.txt": lines(1, 20), "b.txt": lines(1, 10)})
	// a.txt: line 3 changes, two lines follow line 10, lines 15-16 go.
	edited := lines(1, 2) + "line three\n" + lines(4, 10) + "new 1\nnew 2\n" + lines(11, 14) + lines(17, 20)
	second := commit("second", 1700000100, map[string]string{"a.txt": edited, "c.txt": lines(1, 5)}, "b.txt")
	// The last line loses its newline.
	writeMarkdown(t, filepath.Join(root, "a.txt"), strings.TrimSuffix(edited, "\n"))
	return root, first, second
}

func TestCompareStatsMatchGitNumstat(t *testing.T) {
	root, first, second := numstatRepo(t)
	app := newTestApp(t, root)

	commit := compareStats(t, app, "compare=commit&commit="+second.String())
	want := map[string][2]int{"a.txt": {3, 3}, "b.txt": {0, 10}, "c.txt": {5, 0}}
	working := compareStats(t, app, "compare=working")
	wantWorking := map[string][2]int{"a.txt": {1, 1}}
	for _, check := range []struct {
		label string
		stats compareStatsResponse
		want  map[string][2]int
		args  []string
	}{
		{"commit", commit, want, []string{"diff", "--numstat", "--no-renames", first.String(), second.String()}},
		{"working", working, wantWorking, []string{"diff", "--numstat", "--no-renames", "HEAD"}},
	} {
		got := map[string][2]int{}
		for _, file := range check.stats.Files {
			if file.Status != statsCounted || file.Additions == nil || file.Deletions == nil {
				t.Fatalf("%s %s = %+v", check.label, file.Path, file)
			}
			got[file.Path] = [2]int{*file.Additions, *file.Deletions}
		}
		if !reflect.DeepEqual(got, check.want) {
			t.Errorf("%s = %v, want %v", check.label, got, check.want)
		}

		gitBinary, err := exec.LookPath("git")
		if err != nil {
			continue
		}
		cmd := exec.Command(gitBinary, check.args...)
		cmd.Dir = root
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", check.args, err)
		}
		numstat := map[string][2]int{}
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.Split(line, "\t")
			added, _ := strconv.Atoi(fields[0])
			deleted, _ := strconv.Atoi(fields[1])
			numstat[fields[2]] = [2]int{added, deleted}
		}
		if !reflect.DeepEqual(got, numstat) {
			t.Errorf("%s = %v, git numstat = %v", check.label, got, numstat)
		}
	}
}

func TestCompareStatsStopAtTheReadBudget(t *testing.T) {
	root := changesRepo(t)
	app := newTestApp(t, root)
	// added.go's 13 bytes fit; data.bin's two sides do not.
	app.statsBudget = 20

	got := compareStats(t, app, "compare=working")
	assertStats(t, "working", got, compareStatsResponse{BudgetExceeded: true, Files: []fileStats{
		counted("added.go", 1, 0, blobHash("package main\n")),
		uncounted("data.bin", statsBudget, ""),
		uncounted("doc.md", statsBudget, ""),
		// A deleted file's blob comes from HEAD's tree, free of the budget.
		uncounted("gone.md", statsBudget, blobHash("# Gone\n")),
		uncounted("large.txt", statsBudget, ""),
		uncounted("moved/new.md", statsBudget, ""),
		uncounted("notes.txt", statsBudget, ""),
		uncounted("scope/x.zzz", statsBudget, ""),
	}})

	historyRoot, _, change := historyRepo(t)
	history := newTestApp(t, historyRoot)
	history.statsBudget = 0
	stats := compareStats(t, history, "compare=commit&commit="+change.String())
	if !stats.BudgetExceeded || len(stats.Files) != 7 {
		t.Fatalf("zero budget = %s", describeStats(stats))
	}
	for _, file := range stats.Files {
		if file.Status != statsBudget || len(file.Blob) != 40 || file.Additions != nil {
			t.Errorf("zero budget %s = %+v, want budget with its tree blob", file.Path, file)
		}
	}
}

func TestCompareStatsStopWhenCancelled(t *testing.T) {
	root := changesRepo(t)
	app := newTestApp(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var opened []string
	app.openFile = func(name string) (fs.File, error) {
		opened = append(opened, name)
		cancel()
		return os.Open(name)
	}

	target := "/api/compare-stats?project=" + url.QueryEscape(app.roots[0]) + "&compare=working"
	response := httptest.NewRecorder()
	app.ServeHTTP(response, localRequest(http.MethodGet, target).WithContext(ctx))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "context canceled") {
		t.Fatalf("cancelled = %d %s", response.Code, response.Body.String())
	}
	if len(opened) != 1 {
		t.Fatalf("opened %v after cancelling", opened)
	}
}

func TestCompareStatsReadWorktreeFilesInsideTheRootOnly(t *testing.T) {
	root := changesRepo(t)
	outside := filepath.Join(t.TempDir(), "secret.md")
	writeMarkdown(t, outside, "one\ntwo\nthree\n")
	// An untracked symlink out of the root, and a tracked file replaced by
	// one: git records, and stats count, the link text.
	if err := os.Symlink(outside, filepath.Join(root, "escape.md")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.Remove(filepath.Join(root, "docs", "keep.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "docs", "keep.md")); err != nil {
		t.Fatal(err)
	}
	app := newTestApp(t, root)
	app.openFile = func(name string) (fs.File, error) {
		if !strings.HasPrefix(name, root+string(filepath.Separator)) {
			t.Errorf("opened %s outside the root", name)
		}
		return os.Open(name)
	}

	got := compareStats(t, app, "compare=working")
	byPath := map[string]fileStats{}
	for _, file := range got.Files {
		byPath[file.Path] = file
	}
	if want := counted("escape.md", 1, 0, blobHash(outside)); !reflect.DeepEqual(byPath["escape.md"], want) {
		t.Errorf("escape.md = %+v, want %+v", byPath["escape.md"], want)
	}
	if want := counted("docs/keep.md", 1, 1, blobHash(outside)); !reflect.DeepEqual(byPath["docs/keep.md"], want) {
		t.Errorf("docs/keep.md = %+v, want %+v", byPath["docs/keep.md"], want)
	}
	if want := counted("moved/new.md", 0, 0, blobHash("# Moved\n")); !reflect.DeepEqual(byPath["moved/new.md"], want) {
		t.Errorf("rename = %+v, want %+v", byPath["moved/new.md"], want)
	}
}

func TestCompareStatsRejectInvalidQueries(t *testing.T) {
	root, _, change := historyRepo(t)
	app := newTestApp(t, root)
	hash := change.String()
	for _, query := range []string{
		"",
		"compare=all",
		"compare=working&base=master",
		"compare=working&commit=" + hash,
		"compare=branch&from=" + hash,
		"compare=commit",
		"compare=commit&commit=HEAD",
		"compare=commit&commit=" + hash[:7],
		"compare=commit&commit=" + hash + "&base=master",
		"compare=range&from=" + hash,
		"compare=range&from=" + hash + "&to=HEAD",
	} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/compare-stats?project=.&"+query, &failure); status != http.StatusBadRequest || failure.Error == "" || failure.Code != "" {
			t.Errorf("%s = %d %+v", query, status, failure)
		}
	}
	unknown := strings.Repeat("1", 40)
	for _, query := range []string{"compare=commit&commit=" + unknown, "compare=range&from=" + unknown + "&to=" + hash, "compare=range&from=" + hash + "&to=" + unknown} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/compare-stats?project=.&"+query, &failure); status != http.StatusNotFound || failure != (errorResponse{Error: commitUnavailableMessage}) {
			t.Errorf("%s = %d %+v", query, status, failure)
		}
	}
	var failure errorResponse
	if status := requestJSON(t, app, "/api/compare-stats?compare=working", &failure); status != http.StatusBadRequest || failure.Error != noProjectMessage {
		t.Fatalf("no project = %d %+v", status, failure)
	}
}

func TestCompareStatsInFolders(t *testing.T) {
	root := t.TempDir()
	writeMarkdown(t, filepath.Join(root, "README.md"), "# Folder")
	app := newTestApp(t, root)
	for _, query := range []string{"compare=working", "compare=branch", "compare=branch&base=main"} {
		if response := request(t, app, "/api/compare-stats?project=.&"+query); response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"files":[],"budgetExceeded":false}` {
			t.Errorf("%s = %d %s", query, response.Code, response.Body.String())
		}
	}
	hash := strings.Repeat("1", 40)
	for _, query := range []string{"compare=commit&commit=" + hash, "compare=range&from=" + hash + "&to=" + hash} {
		var failure errorResponse
		if status := requestJSON(t, app, "/api/compare-stats?project=.&"+query, &failure); status != http.StatusNotFound || failure.Error != commitUnavailableMessage {
			t.Errorf("%s = %d %+v", query, status, failure)
		}
	}
}

// Frontend refs and stats fixtures must decode into the server's shapes and
// list the same files, in order, as the change fixtures they describe.
func TestCompareFixturesMatchServerContract(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "api", "default")
	var refs refsResponse
	decodeFixture(t, filepath.Join(dir, "refs.json"), &refs)
	var commits commitsResponse
	decodeFixture(t, filepath.Join(dir, "commits.json"), &commits)
	listed := map[string]bool{}
	for _, branch := range refs.Branches {
		listed[branch.Name] = true
	}
	if !refs.Repository || refs.Branch != commits.Branch || refs.DefaultBase != commits.Base || !listed[refs.DefaultBase] || !listed[refs.Branch] {
		t.Errorf("refs = %+v, commits branch %q base %q", refs, commits.Branch, commits.Base)
	}

	var changes changesResponse
	decodeFixture(t, filepath.Join(dir, "changes.json"), &changes)
	var commitFiles commitFilesResponse
	decodeFixture(t, filepath.Join(dir, "commit-files.json"), &commitFiles)
	for name, files := range map[string][]git.Change{"compare-stats.json": changes.Files, "compare-stats-history.json": commitFiles.Files} {
		var stats compareStatsResponse
		decodeFixture(t, filepath.Join(dir, name), &stats)
		if got, want := paths(stats.Files, func(file fileStats) string { return file.Path }), paths(files, func(change git.Change) string { return change.Path }); !reflect.DeepEqual(got, want) {
			t.Errorf("%s paths = %v, want %v", name, got, want)
		}
		for _, file := range stats.Files {
			if (file.Status == statsCounted) != (file.Additions != nil && file.Deletions != nil) || (file.Status != statsCounted && (file.Additions != nil || file.Deletions != nil)) {
				t.Errorf("%s %s counts do not match status %s", name, file.Path, file.Status)
			}
			if _, ok := git.ParseHash(file.Blob); !ok && file.Blob != "" {
				t.Errorf("%s %s blob %q is not a hash", name, file.Path, file.Blob)
			}
		}
	}
}
