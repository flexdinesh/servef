package git

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// isolate points git's system, XDG, and global config at an empty home so
// the host's excludes do not leak into tests. It returns the home.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return home
}

type fixture struct {
	t    *testing.T
	home string
	dir  string
	repo *gogit.Repository
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	home := isolate(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, home: home, dir: dir, repo: repo}
}

func (f *fixture) write(name, content string) {
	f.t.Helper()
	writeFile(f.t, filepath.Join(f.dir, filepath.FromSlash(name)), content)
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) remove(name string) {
	f.t.Helper()
	if err := os.Remove(filepath.Join(f.dir, filepath.FromSlash(name))); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) worktree() *gogit.Worktree {
	f.t.Helper()
	worktree, err := f.repo.Worktree()
	if err != nil {
		f.t.Fatal(err)
	}
	return worktree
}

func (f *fixture) add(names ...string) {
	f.t.Helper()
	worktree := f.worktree()
	for _, name := range names {
		if err := worktree.AddWithOptions(&gogit.AddOptions{Path: name, SkipStatus: true}); err != nil {
			f.t.Fatalf("add %s: %v", name, err)
		}
	}
}

func (f *fixture) commit(names ...string) plumbing.Hash {
	f.t.Helper()
	f.add(names...)
	hash, err := f.worktree().Commit("commit", &gogit.CommitOptions{
		Author:            &object.Signature{Name: "servef", Email: "servef@example.com", When: time.Unix(1700000000, 0)},
		AllowEmptyCommits: true,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return hash
}

// age moves a file's mtime into the past so the index written after it is
// not racy.
func (f *fixture) age(names ...string) {
	f.t.Helper()
	past := time.Now().Add(-time.Hour)
	for _, name := range names {
		if err := os.Chtimes(filepath.Join(f.dir, filepath.FromSlash(name)), past, past); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *fixture) open() *Repository {
	f.t.Helper()
	return openRepository(f.t, f.dir)
}

func openRepository(t *testing.T, dir string) *Repository {
	t.Helper()
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func status(t *testing.T, repo *Repository) []Change {
	t.Helper()
	changes, err := repo.Status(context.Background(), StatusOptions{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if changes.Truncated {
		t.Fatal("Status() truncated without a limit")
	}
	return changes.Files
}

func assertChanges(t *testing.T, got []Change, want ...Change) {
	t.Helper()
	if want == nil {
		want = []Change{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %+v\nwant      %+v", got, want)
	}
}

func TestStatusReportsChangesSinceHead(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"keep.txt", "edit.txt", "mode.sh", "gone.txt", "old/name.txt", "staged-edit.txt"} {
		f.write(name, "content of "+name+"\n")
	}
	f.age("keep.txt", "edit.txt", "mode.sh", "gone.txt", "old/name.txt", "staged-edit.txt")
	f.commit("keep.txt", "edit.txt", "mode.sh", "gone.txt", "old/name.txt", "staged-edit.txt")

	f.write("edit.txt", "changed\n")
	if err := os.Chmod(filepath.Join(f.dir, "mode.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.remove("gone.txt")
	if err := os.MkdirAll(filepath.Join(f.dir, "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(f.dir, "old", "name.txt"), filepath.Join(f.dir, "new", "name.txt")); err != nil {
		t.Fatal(err)
	}
	f.write("staged-edit.txt", "staged change\n")
	f.write("staged.txt", "staged\n")
	f.add("staged-edit.txt", "staged.txt")
	f.write("untracked.txt", "untracked\n")

	assertChanges(t, status(t, f.open()),
		Change{Path: "edit.txt", Status: Modified},
		Change{Path: "gone.txt", Status: Deleted},
		Change{Path: "mode.sh", Status: Modified},
		Change{Path: "new/name.txt", OldPath: "old/name.txt", Status: Renamed},
		Change{Path: "staged-edit.txt", Status: Modified},
		Change{Path: "staged.txt", Status: Added},
		Change{Path: "untracked.txt", Status: Untracked},
	)
}

func TestStatusIsCleanAfterCommit(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "a\n")
	f.write("dir/b.txt", "b\n")
	f.commit("a.txt", "dir/b.txt")
	assertChanges(t, status(t, f.open()))

	// Reverting a change makes the path clean again, whatever its stat data.
	f.write("a.txt", "changed\n")
	f.write("a.txt", "a\n")
	assertChanges(t, status(t, f.open()))
}

func TestStatusPairsRenamesDeterministically(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "same\n")
	f.write("b.txt", "same\n")
	f.write("c.txt", "other\n")
	f.commit("a.txt", "b.txt", "c.txt")
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		f.remove(name)
	}
	f.write("x.txt", "same\n")
	f.write("y.txt", "same\n")
	f.write("z.txt", "other but longer\n")

	assertChanges(t, status(t, f.open()),
		Change{Path: "c.txt", Status: Deleted},
		Change{Path: "x.txt", OldPath: "a.txt", Status: Renamed},
		Change{Path: "y.txt", OldPath: "b.txt", Status: Renamed},
		Change{Path: "z.txt", Status: Untracked},
	)
}

func TestStatusHidesIgnoredUntrackedFilesButShowsTrackedOnes(t *testing.T) {
	f := newFixture(t)
	writeFile(t, filepath.Join(f.home, ".config", "git", "ignore"), "global.txt\n")
	f.write(".git/info/exclude", "local.txt\n")
	f.write(".gitignore", "*.log\nbuild/\n")
	f.write("tracked.log", "v1\n")
	f.write("build/kept.txt", "v1\n")
	f.commit(".gitignore", "tracked.log", "build/kept.txt")

	f.write("tracked.log", "v2\n")
	f.write("build/kept.txt", "v2\n")
	for _, name := range []string{"new.log", "build/out.txt", "global.txt", "local.txt", "docs/local.txt", "shown.txt"} {
		f.write(name, "ignored?\n")
	}

	assertChanges(t, status(t, f.open()),
		Change{Path: "build/kept.txt", Status: Modified},
		Change{Path: "shown.txt", Status: Untracked},
		Change{Path: "tracked.log", Status: Modified},
	)
}

func TestStatusRehashesRacyIndexEntries(t *testing.T) {
	f := newFixture(t)
	f.write("racy.txt", "aaaa\n")
	f.commit("racy.txt")

	// Same size and mtime as the index entry, and the index is no newer than
	// the entry: only rehashing can see the change.
	name := filepath.Join(f.dir, "racy.txt")
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	f.write("racy.txt", "bbbb\n")
	if err := os.Chtimes(name, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(f.dir, ".git", "index"), info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	assertChanges(t, status(t, f.open()), Change{Path: "racy.txt", Status: Modified})
}

func TestStatusInEmptyRepository(t *testing.T) {
	f := newFixture(t)
	f.write("staged.txt", "staged\n")
	f.write("loose.txt", "loose\n")
	f.add("staged.txt")

	repo := f.open()
	if got := repo.Branch(); got != "master" {
		t.Fatalf("Branch() = %q, want master", got)
	}
	assertChanges(t, status(t, repo),
		Change{Path: "loose.txt", Status: Untracked},
		Change{Path: "staged.txt", Status: Added},
	)
	if _, err := repo.HeadBlob("staged.txt"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("HeadBlob() error = %v, want ErrUnavailable", err)
	}
}

func TestStatusInLinkedWorktree(t *testing.T) {
	f := newFixture(t)
	writeFile(t, filepath.Join(f.home, ".config", "git", "ignore"), "global.txt\n")
	f.write(".git/info/exclude", "local.txt\n")
	f.write("a.txt", "a\n")
	f.write("b.txt", "b\n")
	commit := f.commit("a.txt", "b.txt")

	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(parent, "feature")
	admin := filepath.Join(f.dir, ".git", "worktrees", "feature")
	writeFile(t, filepath.Join(admin, "HEAD"), "ref: refs/heads/feature\n")
	writeFile(t, filepath.Join(admin, "commondir"), "../..\n")
	writeFile(t, filepath.Join(admin, "gitdir"), filepath.Join(worktree, ".git")+"\n")
	writeFile(t, filepath.Join(f.dir, ".git", "refs", "heads", "feature"), commit.String()+"\n")
	writeFile(t, filepath.Join(worktree, ".git"), "gitdir: "+admin+"\n")
	writeFile(t, filepath.Join(worktree, "a.txt"), "a changed\n")
	writeFile(t, filepath.Join(worktree, "b.txt"), "b\n")
	for _, name := range []string{"new.txt", "global.txt", "local.txt"} {
		writeFile(t, filepath.Join(worktree, name), "new\n")
	}

	repo := openRepository(t, worktree)
	if got := repo.Branch(); got != "feature" {
		t.Fatalf("Branch() = %q, want feature", got)
	}
	assertChanges(t, status(t, repo),
		Change{Path: "a.txt", Status: Modified},
		Change{Path: "new.txt", Status: Untracked},
	)
	blob, err := repo.HeadBlob("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := readBlob(t, blob); got != "a\n" {
		t.Fatalf("HEAD a.txt = %q", got)
	}
}

func TestStatusComparesSymlinksByTarget(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "a\n")
	f.write("dir/inside.txt", "inside\n")
	if err := os.Symlink("a.txt", filepath.Join(f.dir, "link")); err != nil {
		t.Fatal(err)
	}
	f.commit("a.txt", "link")

	f.remove("link")
	if err := os.Symlink("dir/inside.txt", filepath.Join(f.dir, "link")); err != nil {
		t.Fatal(err)
	}
	// A symlinked directory is one untracked entry, never followed.
	if err := os.Symlink("dir", filepath.Join(f.dir, "alias")); err != nil {
		t.Fatal(err)
	}

	repo := f.open()
	assertChanges(t, status(t, repo),
		Change{Path: "alias", Status: Untracked},
		Change{Path: "dir/inside.txt", Status: Untracked},
		Change{Path: "link", Status: Modified},
	)
	entry, err := repo.Lookup(context.Background(), "link", "")
	if err != nil {
		t.Fatal(err)
	}
	if !entry.Symlink || entry.Link != "dir/inside.txt" || entry.Status != Modified {
		t.Fatalf("Lookup(link) = %+v", entry)
	}
	blob, err := repo.HeadBlob("link")
	if err != nil {
		t.Fatal(err)
	}
	if got := readBlob(t, blob); got != "a.txt" {
		t.Fatalf("HEAD link = %q, want its target", got)
	}
	// A tracked path behind a symlinked directory is gone, as in git.
	if entry, err := repo.Lookup(context.Background(), "alias/inside.txt", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Lookup(alias/inside.txt) = %+v, %v", entry, err)
	}
}

func TestStatusSkipsSubmodulesAndNestedRepositories(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "a\n")
	f.add("a.txt")
	idx, err := f.repo.Storer.Index()
	if err != nil {
		t.Fatal(err)
	}
	idx.Entries = append(idx.Entries, &index.Entry{Name: "sub", Mode: filemode.Submodule, Hash: plumbing.NewHash("1111111111111111111111111111111111111111")})
	if err := f.repo.Storer.SetIndex(idx); err != nil {
		t.Fatal(err)
	}
	f.commit()

	f.write("sub/.git", "gitdir: ../.git/modules/sub\n")
	f.write("sub/changed.txt", "x\n")
	f.write("nested/.git/HEAD", "ref: refs/heads/main\n")
	f.write("nested/file.txt", "x\n")

	repo := f.open()
	assertChanges(t, status(t, repo))
	for _, name := range []string{"sub", "sub/changed.txt", "nested/file.txt", ".git/HEAD"} {
		if _, err := repo.Lookup(context.Background(), name, ""); !errors.Is(err, ErrUnavailable) {
			t.Errorf("Lookup(%s) error = %v, want ErrUnavailable", name, err)
		}
	}
}

func TestStatusCapsChanges(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"e.txt", "d.txt", "c.txt", "b.txt", "a.txt"} {
		f.write(name, name)
	}
	changes, err := f.open().Status(context.Background(), StatusOptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !changes.Truncated {
		t.Fatal("Truncated = false, want true")
	}
	assertChanges(t, changes.Files,
		Change{Path: "a.txt", Status: Untracked},
		Change{Path: "b.txt", Status: Untracked},
		Change{Path: "c.txt", Status: Untracked},
	)
}

func TestStatusStopsWhenCancelled(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "a\n")
	f.commit("a.txt")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.open().Status(ctx, StatusOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Status() error = %v, want context.Canceled", err)
	}
}

func TestOpenRejectsFolders(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	if _, err := Open(dir); !errors.Is(err, ErrNotRepository) {
		t.Fatalf("Open(folder) error = %v, want ErrNotRepository", err)
	}
	writeFile(t, filepath.Join(dir, ".git"), "gitdir: /missing\n")
	if _, err := Open(dir); !errors.Is(err, ErrNotRepository) {
		t.Fatalf("Open(dangling .git file) error = %v, want ErrNotRepository", err)
	}
}

func TestLookupClassifiesOnePath(t *testing.T) {
	f := newFixture(t)
	f.write(".gitignore", "*.log\n")
	for _, name := range []string{"same.txt", "edit.txt", "gone.txt", "old.txt"} {
		f.write(name, "content of "+name+"\n")
	}
	f.commit(".gitignore", "same.txt", "edit.txt", "gone.txt", "old.txt")
	f.write("edit.txt", "changed\n")
	f.remove("gone.txt")
	if err := os.Rename(filepath.Join(f.dir, "old.txt"), filepath.Join(f.dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	f.write("staged.txt", "staged\n")
	f.add("staged.txt")
	f.write("loose.txt", "loose\n")
	f.write("debug.log", "ignored\n")
	repo := f.open()

	for _, test := range []struct {
		path, oldPath string
		want          Change
	}{
		{"edit.txt", "", Change{Path: "edit.txt", Status: Modified}},
		{"gone.txt", "", Change{Path: "gone.txt", Status: Deleted}},
		{"staged.txt", "", Change{Path: "staged.txt", Status: Added}},
		{"loose.txt", "", Change{Path: "loose.txt", Status: Untracked}},
		{"new.txt", "", Change{Path: "new.txt", Status: Untracked}},
		{"new.txt", "old.txt", Change{Path: "new.txt", OldPath: "old.txt", Status: Renamed}},
	} {
		entry, err := repo.Lookup(context.Background(), test.path, test.oldPath)
		if err != nil || entry.Change != test.want || entry.Symlink {
			t.Errorf("Lookup(%q, %q) = %+v, %v; want %+v", test.path, test.oldPath, entry, err, test.want)
		}
	}

	if _, err := repo.Lookup(context.Background(), "same.txt", ""); !errors.Is(err, ErrUnchanged) {
		t.Errorf("Lookup(same.txt) error = %v, want ErrUnchanged", err)
	}
	for _, test := range [][2]string{
		{"debug.log", ""},       // ignored and untracked
		{"missing.txt", ""},     // nowhere
		{"../outside.txt", ""},  // escapes
		{"/etc/passwd", ""},     // absolute
		{"./edit.txt", ""},      // not clean
		{"new.txt", "gone.txt"}, // different content
		{"new.txt", "same.txt"}, // old path still present
		{"edit.txt", "old.txt"}, // new path in HEAD
		{"new.txt", "new.txt"},
	} {
		if _, err := repo.Lookup(context.Background(), test[0], test[1]); !errors.Is(err, ErrUnavailable) {
			t.Errorf("Lookup(%q, %q) error = %v, want ErrUnavailable", test[0], test[1], err)
		}
	}
}

func TestHeadBlobReadsCommittedContent(t *testing.T) {
	f := newFixture(t)
	f.write("doc.md", "# committed\n")
	f.commit("doc.md")
	f.write("doc.md", "# working\n")

	blob, err := f.open().HeadBlob("doc.md")
	if err != nil {
		t.Fatal(err)
	}
	if blob.Size != int64(len("# committed\n")) {
		t.Fatalf("Size = %d", blob.Size)
	}
	if got := readBlob(t, blob); got != "# committed\n" {
		t.Fatalf("content = %q", got)
	}
}

func readBlob(t *testing.T, blob Blob) string {
	t.Helper()
	reader, err := blob.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

// The status matches real git's for the same working tree, renames aside.
func TestStatusMatchesGitPorcelain(t *testing.T) {
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	f := newFixture(t)
	f.write(".gitignore", "*.log\nbuild/\n")
	for _, name := range []string{"keep.txt", "edit.txt", "gone.txt", "dir/deep.txt", "tracked.log"} {
		f.write(name, "content of "+name+"\n")
	}
	f.commit(".gitignore", "keep.txt", "edit.txt", "gone.txt", "dir/deep.txt", "tracked.log")
	f.write("edit.txt", "changed\n")
	f.write("tracked.log", "changed\n")
	f.remove("gone.txt")
	f.write("staged.txt", "staged\n")
	f.add("staged.txt")
	f.write("dir/new.txt", "new\n")
	f.write("build/out.txt", "ignored\n")
	f.write("other.log", "ignored\n")

	cmd := exec.Command(gitBinary, "status", "--porcelain=v1", "-uall", "--no-renames")
	cmd.Dir = f.dir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	var want []Change
	for _, line := range strings.Split(strings.TrimRight(string(output), "\n"), "\n") {
		code, name := line[:2], line[3:]
		switch {
		case code == "??":
			want = append(want, Change{Path: name, Status: Untracked})
		case strings.Contains(code, "D"):
			want = append(want, Change{Path: name, Status: Deleted})
		case code[0] == 'A':
			want = append(want, Change{Path: name, Status: Added})
		default:
			want = append(want, Change{Path: name, Status: Modified})
		}
	}
	sort.Slice(want, func(a, b int) bool { return want[a].Path < want[b].Path })
	assertChanges(t, status(t, f.open()), want...)
}
