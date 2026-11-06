package git

import (
	"errors"
	"path/filepath"
	"testing"
)

func readHead(t *testing.T, dir string) Head {
	t.Helper()
	head, err := ReadHead(dir)
	if err != nil {
		t.Fatalf("ReadHead() error = %v", err)
	}
	return head
}

func TestReadHeadOnBranch(t *testing.T) {
	f := newFixture(t)
	first := f.empty("First\n", 1700000000)
	second := f.empty("Second subject\n\nBody line.\n", 1700000100, first)
	f.ref("refs/heads/master", second)

	want := Head{Branch: "master", Commit: second.String(), Parent: first.String(), Subject: "Second subject"}
	if got := readHead(t, f.dir); got != want {
		t.Fatalf("ReadHead() = %+v, want %+v", got, want)
	}

	f.ref("refs/heads/feature", first)
	f.symbolic("HEAD", "refs/heads/feature")
	want = Head{Branch: "feature", Commit: first.String(), Subject: "First"}
	if got := readHead(t, f.dir); got != want {
		t.Fatalf("after switch ReadHead() = %+v, want %+v", got, want)
	}
}

func TestReadHeadDetached(t *testing.T) {
	f := newFixture(t)
	commit := f.empty("Detached\n", 1700000000)
	f.ref("refs/heads/master", commit)
	f.ref("HEAD", commit)

	want := Head{Commit: commit.String(), Subject: "Detached"}
	if got := readHead(t, f.dir); got != want {
		t.Fatalf("ReadHead() = %+v, want %+v", got, want)
	}
}

func TestReadHeadUnborn(t *testing.T) {
	f := newFixture(t)
	if got, want := readHead(t, f.dir), (Head{Branch: "master"}); got != want {
		t.Fatalf("ReadHead() = %+v, want %+v", got, want)
	}
}

func TestReadHeadInLinkedWorktree(t *testing.T) {
	f := newFixture(t)
	main := f.empty("Main\n", 1700000000)
	feature := f.empty("Feature work\n", 1700000100, main)
	f.ref("refs/heads/master", main)

	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(parent, "feature")
	admin := filepath.Join(f.dir, ".git", "worktrees", "feature")
	writeFile(t, filepath.Join(admin, "HEAD"), "ref: refs/heads/feature\n")
	writeFile(t, filepath.Join(admin, "commondir"), "../..\n")
	writeFile(t, filepath.Join(admin, "gitdir"), filepath.Join(worktree, ".git")+"\n")
	writeFile(t, filepath.Join(f.dir, ".git", "refs", "heads", "feature"), feature.String()+"\n")
	writeFile(t, filepath.Join(worktree, ".git"), "gitdir: "+admin+"\n")

	want := Head{Branch: "feature", Commit: feature.String(), Parent: main.String(), Subject: "Feature work"}
	if got := readHead(t, worktree); got != want {
		t.Fatalf("worktree ReadHead() = %+v, want %+v", got, want)
	}
	want = Head{Branch: "master", Commit: main.String(), Subject: "Main"}
	if got := readHead(t, f.dir); got != want {
		t.Fatalf("main ReadHead() = %+v, want %+v", got, want)
	}
}

func TestReadHeadRejectsFolders(t *testing.T) {
	isolate(t)
	if _, err := ReadHead(t.TempDir()); !errors.Is(err, ErrNotRepository) {
		t.Fatalf("ReadHead(folder) error = %v, want ErrNotRepository", err)
	}
}
