package git

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// file is one tree entry a test commit records.
type file struct {
	content string
	mode    filemode.FileMode
}

func regular(content string) file { return file{content: content, mode: filemode.Regular} }

// tree writes files, keyed by slash-separated path, as a git tree and
// returns its hash. A Submodule mode records content as the gitlink hash.
func (f *fixture) tree(files map[string]file) plumbing.Hash {
	f.t.Helper()
	children := map[string]map[string]file{}
	var entries []object.TreeEntry
	for name, entry := range files {
		if dir, rest, nested := strings.Cut(name, "/"); nested {
			if children[dir] == nil {
				children[dir] = map[string]file{}
			}
			children[dir][rest] = entry
			continue
		}
		hash := plumbing.NewHash(entry.content)
		if entry.mode != filemode.Submodule {
			hash = f.object(plumbing.BlobObject, []byte(entry.content))
		}
		entries = append(entries, object.TreeEntry{Name: name, Mode: entry.mode, Hash: hash})
	}
	for dir, nested := range children {
		entries = append(entries, object.TreeEntry{Name: dir, Mode: filemode.Dir, Hash: f.tree(nested)})
	}
	// Git sorts directories as if their names ended in a slash.
	key := func(entry object.TreeEntry) string {
		if entry.Mode == filemode.Dir {
			return entry.Name + "/"
		}
		return entry.Name
	}
	sort.Slice(entries, func(a, b int) bool { return key(entries[a]) < key(entries[b]) })
	obj := f.repo.Storer.NewEncodedObject()
	if err := (&object.Tree{Entries: entries}).Encode(obj); err != nil {
		f.t.Fatal(err)
	}
	hash, err := f.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		f.t.Fatal(err)
	}
	return hash
}

func (f *fixture) object(kind plumbing.ObjectType, content []byte) plumbing.Hash {
	f.t.Helper()
	obj := f.repo.Storer.NewEncodedObject()
	obj.SetType(kind)
	writer, err := obj.Writer()
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := writer.Write(content); err != nil {
		f.t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		f.t.Fatal(err)
	}
	hash, err := f.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		f.t.Fatal(err)
	}
	return hash
}

// at records a commit of tree with committer time when (seconds) and the
// given parents.
func (f *fixture) at(tree plumbing.Hash, message string, when int64, parents ...plumbing.Hash) plumbing.Hash {
	f.t.Helper()
	signature := object.Signature{Name: "servef", Email: "servef@example.com", When: time.Unix(when, 0).In(time.FixedZone("", 11*60*60))}
	commit := &object.Commit{Author: signature, Committer: signature, Message: message, TreeHash: tree, ParentHashes: parents}
	obj := f.repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		f.t.Fatal(err)
	}
	hash, err := f.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		f.t.Fatal(err)
	}
	return hash
}

// empty records a commit of an empty tree.
func (f *fixture) empty(message string, when int64, parents ...plumbing.Hash) plumbing.Hash {
	f.t.Helper()
	return f.at(f.tree(nil), message, when, parents...)
}

func (f *fixture) ref(name string, hash plumbing.Hash) {
	f.t.Helper()
	if err := f.repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(name), hash)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) symbolic(name, target string) {
	f.t.Helper()
	if err := f.repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.ReferenceName(name), plumbing.ReferenceName(target))); err != nil {
		f.t.Fatal(err)
	}
}

func logOf(t *testing.T, repo *Repository, options LogOptions) History {
	t.Helper()
	history, err := repo.Log(context.Background(), options)
	if err != nil {
		t.Fatalf("Log(%+v) error = %v", options, err)
	}
	return history
}

func hashes(history History) []plumbing.Hash {
	result := []plumbing.Hash{}
	for _, commit := range history.Commits {
		result = append(result, plumbing.NewHash(commit.Hash))
	}
	return result
}

func assertHashes(t *testing.T, label string, got []plumbing.Hash, want ...plumbing.Hash) {
	t.Helper()
	if want == nil {
		want = []plumbing.Hash{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v\nwant %v", label, got, want)
	}
}

func TestLogListsHistoryNewestFirst(t *testing.T) {
	f := newFixture(t)
	first := f.at(f.tree(map[string]file{"a.txt": regular("a")}), "First\n", 100)
	second := f.empty("Second subject  \n\n\nBody first\n\nsecond line  \n\n", 200, first)
	third := f.empty("Third", 300, second)
	f.ref("refs/heads/master", third)

	repo := f.open()
	history := logOf(t, repo, LogOptions{Range: RangeAll})
	assertHashes(t, "all", hashes(history), third, second, first)
	if history.Head != Hash(third) || history.More {
		t.Fatalf("Head = %s, More = %v", history.Head, history.More)
	}
	got := history.Commits[1]
	want := Commit{
		Hash:       second.String(),
		Parents:    []string{first.String()},
		Subject:    "Second subject",
		Body:       "Body first\n\nsecond line",
		Author:     Author{Name: "servef", Email: "servef@example.com"},
		AuthorTime: got.AuthorTime,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commit = %+v\nwant     %+v", got, want)
	}
	if _, offset := got.AuthorTime.Zone(); !got.AuthorTime.Equal(time.Unix(200, 0)) || offset != 11*60*60 {
		t.Fatalf("AuthorTime = %v, want 200s in +11:00", got.AuthorTime)
	}
	if root := history.Commits[2]; !reflect.DeepEqual(root.Parents, []string{}) || root.Body != "" || root.Subject != "First" {
		t.Fatalf("root = %+v", root)
	}

	// On the base branch itself the branch range is empty.
	branch := logOf(t, repo, LogOptions{Range: RangeBranch})
	if branch.Base != "master" || branch.Ahead != 0 || len(branch.Commits) != 0 || branch.Head != Hash(third) {
		t.Fatalf("branch on base = %+v", branch)
	}
}

func TestLogPagesStablyFromPinnedHead(t *testing.T) {
	f := newFixture(t)
	var chain []plumbing.Hash
	var parents []plumbing.Hash
	for i := range 5 {
		hash := f.empty("commit", int64(100*(i+1)), parents...)
		chain = append([]plumbing.Hash{hash}, chain...)
		parents = []plumbing.Hash{hash}
	}
	f.ref("refs/heads/master", chain[0])
	repo := f.open()

	page := logOf(t, repo, LogOptions{Range: RangeAll, Limit: 2})
	assertHashes(t, "page 1", hashes(page), chain[0], chain[1])
	if !page.More {
		t.Fatal("page 1 More = false")
	}
	newer := f.empty("newer", 600, chain[0])
	f.ref("refs/heads/master", newer)

	page = logOf(t, repo, LogOptions{Range: RangeAll, Head: Hash(chain[0]), Skip: 2, Limit: 2})
	assertHashes(t, "page 2", hashes(page), chain[2], chain[3])
	if !page.More || page.Head != Hash(chain[0]) {
		t.Fatalf("page 2 More = %v, Head = %s", page.More, page.Head)
	}
	page = logOf(t, repo, LogOptions{Range: RangeAll, Head: Hash(chain[0]), Skip: 4, Limit: 2})
	assertHashes(t, "page 3", hashes(page), chain[4])
	if page.More {
		t.Fatal("page 3 More = true")
	}
	// Unpinned, the new commit shifts the pages.
	page = logOf(t, repo, LogOptions{Range: RangeAll, Skip: 2, Limit: 2})
	assertHashes(t, "unpinned page 2", hashes(page), chain[1], chain[2])
}

// branchGraph is main m1-m2-m3 and feature f1-f2 forked from m1, then
// merging m3 as f3 and adding f4.
type branchGraph struct {
	m1, m2, m3, f1, f2, f3, f4 plumbing.Hash
}

func newBranchGraph(f *fixture) branchGraph {
	var g branchGraph
	g.m1 = f.empty("m1", 100)
	g.f1 = f.empty("f1", 150, g.m1)
	g.m2 = f.empty("m2", 200, g.m1)
	g.f2 = f.empty("f2", 250, g.f1)
	g.m3 = f.empty("m3", 300, g.m2)
	g.f3 = f.empty("Merge main", 400, g.f2, g.m3)
	g.f4 = f.empty("f4", 500, g.f3)
	f.ref("refs/heads/main", g.m3)
	f.ref("refs/heads/feature", g.f4)
	f.symbolic("HEAD", "refs/heads/feature")
	return g
}

func TestLogBranchRangeExcludesMergedBaseHistory(t *testing.T) {
	f := newFixture(t)
	g := newBranchGraph(f)
	repo := f.open()

	branch := logOf(t, repo, LogOptions{Range: RangeBranch})
	assertHashes(t, "branch", hashes(branch), g.f4, g.f3, g.f2, g.f1)
	if branch.Base != "main" || branch.Ahead != 4 || branch.Head != Hash(g.f4) {
		t.Fatalf("branch = Base %q, Ahead %d, Head %s", branch.Base, branch.Ahead, branch.Head)
	}
	page := logOf(t, repo, LogOptions{Range: RangeBranch, Skip: 1, Limit: 2})
	assertHashes(t, "branch page", hashes(page), g.f3, g.f2)
	if !page.More || page.Ahead != 4 {
		t.Fatalf("branch page More = %v, Ahead = %d", page.More, page.Ahead)
	}
	all := logOf(t, repo, LogOptions{Range: RangeAll})
	assertHashes(t, "all", hashes(all), g.f4, g.f3, g.m3, g.f2, g.m2, g.f1, g.m1)
	if all.Ahead != 4 {
		t.Fatalf("all Ahead = %d, want 4", all.Ahead)
	}
	if merge := all.Commits[1]; !reflect.DeepEqual(merge.Parents, []string{g.f2.String(), g.m3.String()}) {
		t.Fatalf("merge parents = %v", merge.Parents)
	}
}

func TestLogBranchRangeOnDetachedHead(t *testing.T) {
	f := newFixture(t)
	g := newBranchGraph(f)
	f.ref("HEAD", g.f2)
	repo := f.open()
	if branch := repo.Branch(); branch != "" {
		t.Fatalf("Branch() = %q, want detached", branch)
	}
	history := logOf(t, repo, LogOptions{Range: RangeBranch})
	assertHashes(t, "detached", hashes(history), g.f2, g.f1)
	if history.Head != Hash(g.f2) || history.Ahead != 2 {
		t.Fatalf("detached Head = %s, Ahead = %d", history.Head, history.Ahead)
	}
}

// Branch and full ranges list what git rev-list does, in its order.
func TestLogRangesMatchGit(t *testing.T) {
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	f := newFixture(t)
	g := newBranchGraph(f)
	// A base tip dated before its parent still excludes the merged history.
	skewed := f.empty("skewed", 50, f.empty("x", 900, g.m3))
	f.ref("refs/heads/main", skewed)
	cmd := exec.Command(gitBinary, "rev-list", "main..feature")
	cmd.Dir = f.dir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-list: %v", err)
	}
	var want []plumbing.Hash
	for _, line := range strings.Fields(string(output)) {
		want = append(want, plumbing.NewHash(line))
	}
	assertHashes(t, "branch", hashes(logOf(t, f.open(), LogOptions{Range: RangeBranch})), want...)

	cmd = exec.Command(gitBinary, "rev-list", "feature")
	cmd.Dir = f.dir
	if output, err = cmd.Output(); err != nil {
		t.Fatalf("git rev-list: %v", err)
	}
	want = nil
	for _, line := range strings.Fields(string(output)) {
		want = append(want, plumbing.NewHash(line))
	}
	assertHashes(t, "all", hashes(logOf(t, f.open(), LogOptions{Range: RangeAll})), want...)
}

func TestLogDetectsBaseBranchInOrder(t *testing.T) {
	f := newFixture(t)
	root := f.empty("root", 100)
	tip := f.empty("tip", 200, root)
	f.ref("refs/heads/feature", tip)
	f.symbolic("HEAD", "refs/heads/feature")

	history := logOf(t, f.open(), LogOptions{Range: RangeBranch})
	if history.Base != "" || history.Ahead != 0 || len(history.Commits) != 0 {
		t.Fatalf("no base = %+v", history)
	}
	if all := logOf(t, f.open(), LogOptions{Range: RangeAll}); len(all.Commits) != 2 || all.Ahead != 0 {
		t.Fatalf("all without base = %+v", all)
	}
	for _, step := range []struct {
		set  func()
		want string
	}{
		{func() { f.ref("refs/remotes/origin/master", root) }, "origin/master"},
		{func() { f.ref("refs/remotes/origin/main", root) }, "origin/main"},
		{func() { f.ref("refs/heads/master", root) }, "master"},
		{func() { f.ref("refs/heads/main", root) }, "main"},
		// A dangling origin/HEAD is passed over.
		{func() { f.symbolic("refs/remotes/origin/HEAD", "refs/remotes/origin/missing") }, "main"},
		{func() {
			f.ref("refs/remotes/origin/develop", root)
			f.symbolic("refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
		}, "origin/develop"},
	} {
		step.set()
		history := logOf(t, f.open(), LogOptions{Range: RangeBranch})
		if history.Base != step.want || history.Ahead != 1 {
			t.Fatalf("Base = %q, Ahead = %d, want %q and 1", history.Base, history.Ahead, step.want)
		}
		assertHashes(t, step.want, hashes(history), tip)
	}
}

func TestLogInEmptyRepository(t *testing.T) {
	f := newFixture(t)
	history := logOf(t, f.open(), LogOptions{Range: RangeAll})
	if !history.Head.IsZero() || len(history.Commits) != 0 || history.Commits == nil || history.More {
		t.Fatalf("empty = %+v", history)
	}
}

func TestLogRejectsUnknownHeadsAndStopsWhenCancelled(t *testing.T) {
	f := newFixture(t)
	tree := f.tree(map[string]file{"a.txt": regular("a")})
	commit := f.at(tree, "commit", 100)
	f.ref("refs/heads/master", commit)
	repo := f.open()
	for _, head := range []plumbing.Hash{plumbing.NewHash("1111111111111111111111111111111111111111"), tree} {
		if _, err := repo.Log(context.Background(), LogOptions{Range: RangeAll, Head: Hash(head)}); !errors.Is(err, ErrUnavailable) {
			t.Errorf("Log(head %s) error = %v, want ErrUnavailable", head, err)
		}
		if _, err := repo.CommitFiles(context.Background(), Hash(head), 0); !errors.Is(err, ErrUnavailable) {
			t.Errorf("CommitFiles(%s) error = %v, want ErrUnavailable", head, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, options := range []LogOptions{{Range: RangeAll}, {Range: RangeBranch}} {
		if _, err := repo.Log(ctx, options); !errors.Is(err, context.Canceled) {
			t.Errorf("Log(%s) error = %v, want context.Canceled", options.Range, err)
		}
	}
	if _, err := repo.CommitFiles(ctx, Hash(commit), 0); !errors.Is(err, context.Canceled) {
		t.Errorf("CommitFiles error = %v, want context.Canceled", err)
	}
}

func TestParseHashAcceptsFullLowercaseHashesOnly(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef01234567"
	if hash, ok := ParseHash(valid); !ok || hash.String() != valid {
		t.Fatalf("ParseHash(valid) = %s, %v", hash, ok)
	}
	for _, value := range []string{"", "0123456", strings.ToUpper(valid), valid + "0", valid[:39] + "g", "HEAD", "main"} {
		if _, ok := ParseHash(value); ok {
			t.Errorf("ParseHash(%q) ok", value)
		}
	}
}

func TestCommitFilesComparesWithFirstParent(t *testing.T) {
	f := newFixture(t)
	before := map[string]file{
		"keep.txt":       regular("keep"),
		"edit.txt":       regular("before"),
		"gone.txt":       regular("gone"),
		"a/twin.txt":     regular("twin"),
		"b/twin.txt":     regular("twin"),
		"mode.sh":        regular("run"),
		"link":           regular("target.txt"),
		"dir/inside.txt": regular("inside"),
		"deep/x/y.txt":   regular("unchanged subtree"),
	}
	after := map[string]file{
		"keep.txt":     regular("keep"),
		"edit.txt":     regular("after"),
		"new.txt":      regular("new"),
		"c/twin.txt":   regular("twin"),
		"d/twin.txt":   regular("twin"),
		"mode.sh":      {content: "run", mode: filemode.Executable},
		"link":         {content: "target.txt", mode: filemode.Symlink},
		"dir":          regular("now a file"),
		"sub":          {content: "1111111111111111111111111111111111111111", mode: filemode.Submodule},
		"deep/x/y.txt": regular("unchanged subtree"),
	}
	root := f.at(f.tree(before), "root", 100)
	child := f.at(f.tree(after), "child", 200, root)
	repo := f.open()

	changes, err := repo.CommitFiles(context.Background(), Hash(child), 0)
	if err != nil {
		t.Fatal(err)
	}
	assertChanges(t, changes.Files,
		Change{Path: "c/twin.txt", OldPath: "a/twin.txt", Status: Renamed},
		Change{Path: "d/twin.txt", OldPath: "b/twin.txt", Status: Renamed},
		Change{Path: "dir", Status: Added},
		Change{Path: "dir/inside.txt", Status: Deleted},
		Change{Path: "edit.txt", Status: Modified},
		Change{Path: "gone.txt", Status: Deleted},
		Change{Path: "link", Status: Modified},
		Change{Path: "mode.sh", Status: Modified},
		Change{Path: "new.txt", Status: Added},
	)
	if changes.Truncated {
		t.Fatal("Truncated without a limit")
	}

	limited, err := repo.CommitFiles(context.Background(), Hash(child), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Truncated || !reflect.DeepEqual(limited.Files, changes.Files[:3]) {
		t.Fatalf("limited = %+v", limited)
	}

	// A root commit adds every file but submodules.
	rootChanges, err := repo.CommitFiles(context.Background(), Hash(root), 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, change := range rootChanges.Files {
		if change.Status != Added {
			t.Errorf("root change %+v, want added", change)
		}
		names = append(names, change.Path)
	}
	if want := []string{"a/twin.txt", "b/twin.txt", "deep/x/y.txt", "dir/inside.txt", "edit.txt", "gone.txt", "keep.txt", "link", "mode.sh"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("root paths = %v", names)
	}
}

func TestCommitFilesOfMergeUseFirstParent(t *testing.T) {
	f := newFixture(t)
	base := f.at(f.tree(map[string]file{"a.txt": regular("a"), "b.txt": regular("b")}), "base", 100)
	ours := f.at(f.tree(map[string]file{"a.txt": regular("a2"), "b.txt": regular("b")}), "ours", 200, base)
	theirs := f.at(f.tree(map[string]file{"a.txt": regular("a"), "b.txt": regular("b2"), "c.txt": regular("c")}), "theirs", 250, base)
	merge := f.at(f.tree(map[string]file{"a.txt": regular("a2"), "b.txt": regular("b2"), "c.txt": regular("c")}), "merge", 300, ours, theirs)

	changes, err := f.open().CommitFiles(context.Background(), Hash(merge), 0)
	if err != nil {
		t.Fatal(err)
	}
	assertChanges(t, changes.Files,
		Change{Path: "b.txt", Status: Modified},
		Change{Path: "c.txt", Status: Added},
	)
}

func TestCommitLookupAndBlobsFollowCommitFiles(t *testing.T) {
	f := newFixture(t)
	root := f.at(f.tree(map[string]file{"same.txt": regular("same"), "edit.txt": regular("before"), "old.txt": regular("moved"), "gone.txt": regular("gone")}), "root", 100)
	child := f.at(f.tree(map[string]file{"same.txt": regular("same"), "edit.txt": regular("after"), "new/moved.txt": regular("moved"), "added.txt": regular("added")}), "child", 200, root)
	repo := f.open()
	ctx := context.Background()

	for _, test := range []struct {
		name, oldPath string
		want          Change
	}{
		{"edit.txt", "", Change{Path: "edit.txt", Status: Modified}},
		{"added.txt", "", Change{Path: "added.txt", Status: Added}},
		{"gone.txt", "", Change{Path: "gone.txt", Status: Deleted}},
		{"new/moved.txt", "old.txt", Change{Path: "new/moved.txt", OldPath: "old.txt", Status: Renamed}},
		// A rename's destination alone reports the rename.
		{"new/moved.txt", "", Change{Path: "new/moved.txt", OldPath: "old.txt", Status: Renamed}},
	} {
		got, err := repo.CommitLookup(ctx, Hash(child), test.name, test.oldPath)
		if err != nil || got != test.want {
			t.Errorf("CommitLookup(%s, %s) = %+v, %v; want %+v", test.name, test.oldPath, got, err, test.want)
		}
	}
	for _, test := range [][2]string{
		{"same.txt", ""},
		{"old.txt", ""},
		{"missing.txt", ""},
		{"new/moved.txt", "gone.txt"},
		{"added.txt", "old.txt"},
		{"../edit.txt", ""},
		{".git/HEAD", ""},
		{"edit.txt", "edit.txt"},
	} {
		if got, err := repo.CommitLookup(ctx, Hash(child), test[0], test[1]); !errors.Is(err, ErrUnavailable) {
			t.Errorf("CommitLookup(%s, %s) = %+v, %v; want ErrUnavailable", test[0], test[1], got, err)
		}
	}
	if _, err := repo.CommitLookup(ctx, Hash(plumbing.NewHash("1111111111111111111111111111111111111111")), "edit.txt", ""); !errors.Is(err, ErrUnavailable) {
		t.Errorf("CommitLookup(unknown commit) error = %v", err)
	}

	read := func(blob Blob, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return readBlob(t, blob)
	}
	if got := read(repo.CommitBlob(Hash(child), "edit.txt")); got != "after" {
		t.Errorf("CommitBlob(edit.txt) = %q", got)
	}
	if got := read(repo.ParentBlob(Hash(child), "edit.txt")); got != "before" {
		t.Errorf("ParentBlob(edit.txt) = %q", got)
	}
	if got := read(repo.ParentBlob(Hash(child), "old.txt")); got != "moved" {
		t.Errorf("ParentBlob(old.txt) = %q", got)
	}
	if _, err := repo.ParentBlob(Hash(root), "edit.txt"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("ParentBlob(root) error = %v, want ErrUnavailable", err)
	}
	if _, err := repo.CommitBlob(Hash(child), "gone.txt"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("CommitBlob(gone.txt) error = %v, want ErrUnavailable", err)
	}
}

func TestLogBoundsBranchRangesButStillListsAll(t *testing.T) {
	f := newFixture(t)
	g := newBranchGraph(f)
	repo := f.open()
	defer func(branch, walk int) { branchLimit, walkLimit = branch, walk }(branchLimit, walkLimit)

	for _, limits := range [][2]int{{3, 100}, {100, 3}} {
		branchLimit, walkLimit = limits[0], limits[1]
		_, err := repo.Log(context.Background(), LogOptions{Range: RangeBranch})
		if !errors.Is(err, ErrRangeTooLarge) || err.Error() != "the branch has too many commits to compare with main" {
			t.Fatalf("limits %v: branch error = %v, want ErrRangeTooLarge naming main", limits, err)
		}
		all := logOf(t, repo, LogOptions{Range: RangeAll, Limit: 2})
		assertHashes(t, "all", hashes(all), g.f4, g.f3)
		if all.Ahead != 0 || !all.More || all.Base != "main" {
			t.Fatalf("limits %v: all = Ahead %d, More %v, Base %q", limits, all.Ahead, all.More, all.Base)
		}
	}
}

func rangeFiles(t *testing.T, repo *Repository, from, to plumbing.Hash, limit int) CommitChanges {
	t.Helper()
	changes, err := repo.RangeFiles(context.Background(), Hash(from), Hash(to), limit)
	if err != nil {
		t.Fatalf("RangeFiles(%s, %s) error = %v", from, to, err)
	}
	return changes
}

func TestRangeFilesCompareOldestParentWithNewest(t *testing.T) {
	f := newFixture(t)
	root := f.at(f.tree(map[string]file{"keep.txt": regular("keep"), "edit.txt": regular("v0"), "old.txt": regular("moved")}), "root", 100)
	c1 := f.at(f.tree(map[string]file{"keep.txt": regular("keep"), "edit.txt": regular("v1"), "old.txt": regular("moved"), "temp.txt": regular("temp")}), "c1", 200, root)
	c2 := f.at(f.tree(map[string]file{"keep.txt": regular("keep"), "edit.txt": regular("v2"), "dir/new.txt": regular("moved"), "temp.txt": regular("temp")}), "c2", 300, c1)
	c3 := f.at(f.tree(map[string]file{"keep.txt": regular("keep"), "edit.txt": regular("v2"), "dir/new.txt": regular("moved"), "added.txt": regular("added")}), "c3", 400, c2)
	repo := f.open()

	// c1..c3 against root: temp.txt added then deleted nets to nothing, and
	// the rename in c2 spans the range.
	got := rangeFiles(t, repo, c1, c3, 0)
	assertChanges(t, got.Files,
		Change{Path: "added.txt", Status: Added},
		Change{Path: "dir/new.txt", OldPath: "old.txt", Status: Renamed},
		Change{Path: "edit.txt", Status: Modified},
	)
	if got.Truncated {
		t.Fatal("Truncated without a limit")
	}
	if limited := rangeFiles(t, repo, c1, c3, 2); !limited.Truncated || !reflect.DeepEqual(limited.Files, got.Files[:2]) {
		t.Fatalf("limited = %+v", limited)
	}
	// One commit is that commit's own changes.
	single := rangeFiles(t, repo, c2, c2, 0)
	if commit, err := repo.CommitFiles(context.Background(), Hash(c2), 0); err != nil || !reflect.DeepEqual(single, commit) {
		t.Fatalf("single = %+v, commit = %+v, %v", single, commit, err)
	}
	// A root oldest commit compares with an empty tree.
	assertChanges(t, rangeFiles(t, repo, root, c1, 0).Files,
		Change{Path: "edit.txt", Status: Added},
		Change{Path: "keep.txt", Status: Added},
		Change{Path: "old.txt", Status: Added},
		Change{Path: "temp.txt", Status: Added},
	)

	ctx := context.Background()
	for _, test := range []struct {
		name, oldPath string
		want          Change
	}{
		{"dir/new.txt", "", Change{Path: "dir/new.txt", OldPath: "old.txt", Status: Renamed}},
		{"dir/new.txt", "old.txt", Change{Path: "dir/new.txt", OldPath: "old.txt", Status: Renamed}},
		{"edit.txt", "", Change{Path: "edit.txt", Status: Modified}},
	} {
		if change, err := repo.RangeLookup(ctx, Hash(c1), Hash(c3), test.name, test.oldPath); err != nil || change != test.want {
			t.Errorf("RangeLookup(%s, %s) = %+v, %v", test.name, test.oldPath, change, err)
		}
	}
	for _, test := range [][2]string{{"temp.txt", ""}, {"old.txt", ""}, {"keep.txt", ""}, {"dir/new.txt", "edit.txt"}, {"../edit.txt", ""}} {
		if change, err := repo.RangeLookup(ctx, Hash(c1), Hash(c3), test[0], test[1]); !errors.Is(err, ErrUnavailable) {
			t.Errorf("RangeLookup(%s, %s) = %+v, %v; want ErrUnavailable", test[0], test[1], change, err)
		}
	}
	if got := readBlob(t, mustBlob(t)(repo.ParentBlob(Hash(c1), "edit.txt"))); got != "v0" {
		t.Errorf("old side = %q", got)
	}
	if got := readBlob(t, mustBlob(t)(repo.CommitBlob(Hash(c3), "edit.txt"))); got != "v2" {
		t.Errorf("new side = %q", got)
	}

	unknown := plumbing.NewHash("1111111111111111111111111111111111111111")
	for _, pair := range [][2]plumbing.Hash{{unknown, c3}, {c1, unknown}} {
		if _, err := repo.RangeFiles(ctx, Hash(pair[0]), Hash(pair[1]), 0); !errors.Is(err, ErrUnavailable) {
			t.Errorf("RangeFiles(%v) error = %v, want ErrUnavailable", pair, err)
		}
		if _, err := repo.RangeLookup(ctx, Hash(pair[0]), Hash(pair[1]), "edit.txt", ""); !errors.Is(err, ErrUnavailable) {
			t.Errorf("RangeLookup(%v) error = %v, want ErrUnavailable", pair, err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.RangeFiles(cancelled, Hash(c1), Hash(c3), 0); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled RangeFiles error = %v", err)
	}
}

func mustBlob(t *testing.T) func(Blob, error) Blob {
	return func(blob Blob, err error) Blob {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return blob
	}
}

func compareBranch(t *testing.T, repo *Repository) BranchRange {
	t.Helper()
	compared, err := repo.BranchCompare(context.Background())
	if err != nil {
		t.Fatalf("BranchCompare error = %v", err)
	}
	return compared
}

func branchFiles(t *testing.T, repo *Repository) (BranchRange, []Change) {
	t.Helper()
	compared, changes, err := repo.BranchFiles(context.Background(), 0)
	if err != nil {
		t.Fatalf("BranchFiles error = %v", err)
	}
	return compared, changes.Files
}

func TestBranchFilesCompareMergeBaseWithHead(t *testing.T) {
	f := newFixture(t)
	m1 := f.at(f.tree(map[string]file{"shared.txt": regular("v0"), "base.txt": regular("b0")}), "m1", 100)
	f1 := f.at(f.tree(map[string]file{"shared.txt": regular("v1"), "base.txt": regular("b0"), "feature.txt": regular("f")}), "f1", 150, m1)
	// main advances after the branch: its edit must not show.
	m2 := f.at(f.tree(map[string]file{"shared.txt": regular("v0"), "base.txt": regular("b1"), "main.txt": regular("m")}), "m2", 200, m1)
	f.ref("refs/heads/main", m2)
	f.ref("refs/heads/feature", f1)
	f.symbolic("HEAD", "refs/heads/feature")
	repo := f.open()

	compared, changes := branchFiles(t, repo)
	if want := (BranchRange{Base: "main", MergeBase: Hash(m1), Head: Hash(f1), Ahead: 1}); compared != want {
		t.Fatalf("range = %+v, want %+v", compared, want)
	}
	assertChanges(t, changes,
		Change{Path: "feature.txt", Status: Added},
		Change{Path: "shared.txt", Status: Modified},
	)
	if history := logOf(t, repo, LogOptions{Range: RangeBranch}); history.Ahead != compared.Ahead {
		t.Fatalf("Log Ahead = %d, BranchCompare Ahead = %d", history.Ahead, compared.Ahead)
	}
	ctx := context.Background()
	if _, change, err := repo.BranchLookup(ctx, "shared.txt", ""); err != nil || change != (Change{Path: "shared.txt", Status: Modified}) {
		t.Fatalf("BranchLookup(shared.txt) = %+v, %v", change, err)
	}
	for _, name := range []string{"base.txt", "main.txt", "missing.txt", "../shared.txt"} {
		if _, change, err := repo.BranchLookup(ctx, name, ""); !errors.Is(err, ErrUnavailable) {
			t.Errorf("BranchLookup(%s) = %+v, %v; want ErrUnavailable", name, change, err)
		}
	}

	// Merging main into the branch moves the merge base to main's tip.
	f2 := f.at(f.tree(map[string]file{"shared.txt": regular("v1"), "base.txt": regular("b1"), "main.txt": regular("m"), "feature.txt": regular("f2")}), "Merge main", 300, f1, m2)
	f.ref("refs/heads/feature", f2)
	compared, changes = branchFiles(t, repo)
	if want := (BranchRange{Base: "main", MergeBase: Hash(m2), Head: Hash(f2), Ahead: 2}); compared != want {
		t.Fatalf("merged range = %+v, want %+v", compared, want)
	}
	assertChanges(t, changes,
		Change{Path: "feature.txt", Status: Added},
		Change{Path: "shared.txt", Status: Modified},
	)
	if history := logOf(t, repo, LogOptions{Range: RangeBranch}); history.Ahead != compared.Ahead {
		t.Fatalf("merged Log Ahead = %d, BranchCompare Ahead = %d", history.Ahead, compared.Ahead)
	}
}

func TestBranchCompareMatchesLogOnMergedHistory(t *testing.T) {
	f := newFixture(t)
	g := newBranchGraph(f)
	compared := compareBranch(t, f.open())
	if want := (BranchRange{Base: "main", MergeBase: Hash(g.m3), Head: Hash(g.f4), Ahead: 4}); compared != want {
		t.Fatalf("range = %+v, want %+v", compared, want)
	}
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		return
	}
	cmd := exec.Command(gitBinary, "merge-base", "main", "feature")
	cmd.Dir = f.dir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git merge-base: %v", err)
	}
	if want := plumbing.NewHash(strings.TrimSpace(string(output))); compared.MergeBase != Hash(want) {
		t.Fatalf("MergeBase = %s, git merge-base = %s", compared.MergeBase, want)
	}
}

// Criss-cross merges leave two best common ancestors; the newer wins.
func TestBranchComparePicksNewestOfCrissCrossMergeBases(t *testing.T) {
	f := newFixture(t)
	root := f.empty("root", 100)
	a := f.at(f.tree(map[string]file{"a.txt": regular("a")}), "a", 200, root)
	b := f.at(f.tree(map[string]file{"b.txt": regular("b")}), "b", 210, root)
	main := f.empty("main merge", 300, a, b)
	feature := f.at(f.tree(map[string]file{"a.txt": regular("a"), "b.txt": regular("b")}), "feature merge", 310, a, b)
	f.ref("refs/heads/main", main)
	f.ref("refs/heads/feature", feature)
	f.symbolic("HEAD", "refs/heads/feature")

	compared, changes := branchFiles(t, f.open())
	if want := (BranchRange{Base: "main", MergeBase: Hash(b), Head: Hash(feature), Ahead: 1}); compared != want {
		t.Fatalf("range = %+v, want %+v (a %s)", compared, want, a)
	}
	assertChanges(t, changes, Change{Path: "a.txt", Status: Added})
}

// A boundary commit that is an ancestor of another is not a best one, even
// when clock skew dates it newer.
func TestBranchCompareDropsRedundantBoundaryCommits(t *testing.T) {
	f := newFixture(t)
	m1 := f.empty("m1", 400)
	m2 := f.empty("m2", 200, m1)
	f1 := f.empty("f1", 450, m1)
	f2 := f.empty("Merge main", 500, f1, m2)
	f.ref("refs/heads/main", m2)
	f.ref("refs/heads/feature", f2)
	f.symbolic("HEAD", "refs/heads/feature")
	if compared := compareBranch(t, f.open()); compared.MergeBase != Hash(m2) || compared.Ahead != 2 {
		t.Fatalf("range = %+v, want merge base m2 %s", compared, m2)
	}
}

func TestBranchFilesWithNothingAheadOrNoBase(t *testing.T) {
	f := newFixture(t)
	root := f.at(f.tree(map[string]file{"a.txt": regular("a")}), "root", 100)
	tip := f.at(f.tree(map[string]file{"a.txt": regular("b")}), "tip", 200, root)
	f.ref("refs/heads/feature", tip)
	f.symbolic("HEAD", "refs/heads/feature")
	repo := f.open()
	ctx := context.Background()

	compared, changes := branchFiles(t, repo)
	if want := (BranchRange{Head: Hash(tip)}); compared != want || changes == nil || len(changes) != 0 {
		t.Fatalf("no base = %+v, %+v", compared, changes)
	}
	if _, _, err := repo.BranchLookup(ctx, "a.txt", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no base lookup error = %v", err)
	}

	f.ref("refs/heads/main", tip)
	compared, changes = branchFiles(t, repo)
	if want := (BranchRange{Base: "main", Head: Hash(tip)}); compared != want || len(changes) != 0 {
		t.Fatalf("on base = %+v, %+v", compared, changes)
	}
	if _, _, err := repo.BranchLookup(ctx, "a.txt", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("on base lookup error = %v", err)
	}
	// Behind the base is nothing ahead too.
	f.ref("refs/heads/feature", root)
	if compared := compareBranch(t, repo); compared != (BranchRange{Base: "main", Head: Hash(root)}) {
		t.Fatalf("behind = %+v", compared)
	}
}

func TestBranchCompareInEmptyRepository(t *testing.T) {
	f := newFixture(t)
	f.ref("refs/heads/main", f.empty("main", 100))
	f.symbolic("HEAD", "refs/heads/unborn")
	compared, changes := branchFiles(t, f.open())
	if compared != (BranchRange{Base: "main"}) || len(changes) != 0 {
		t.Fatalf("unborn = %+v, %+v", compared, changes)
	}
}

func TestBranchCompareIsBoundedAndCancellable(t *testing.T) {
	f := newFixture(t)
	newBranchGraph(f)
	repo := f.open()
	defer func(branch, walk int) { branchLimit, walkLimit = branch, walk }(branchLimit, walkLimit)
	for _, limits := range [][2]int{{3, 100}, {100, 3}} {
		branchLimit, walkLimit = limits[0], limits[1]
		if _, err := repo.BranchCompare(context.Background()); !errors.Is(err, ErrRangeTooLarge) || err.Error() != "the branch has too many commits to compare with main" {
			t.Errorf("limits %v: BranchCompare error = %v", limits, err)
		}
		if _, _, err := repo.BranchFiles(context.Background(), 0); !errors.Is(err, ErrRangeTooLarge) {
			t.Errorf("limits %v: BranchFiles error = %v", limits, err)
		}
	}
	branchLimit, walkLimit = 10_000, 100_000
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repo.BranchCompare(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled BranchCompare error = %v", err)
	}
	if _, _, err := repo.BranchLookup(ctx, "a.txt", ""); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled BranchLookup error = %v", err)
	}
}
