package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBranchesListLocalThenRemoteWithoutSymbolicRefs(t *testing.T) {
	f := newFixture(t)
	root := f.empty("root", 100)
	for _, name := range []string{
		"refs/heads/main", "refs/heads/feature/b", "refs/heads/a",
		"refs/remotes/origin/main", "refs/remotes/upstream/dev", "refs/remotes/origin/feature/b",
		"refs/tags/v1",
	} {
		f.ref(name, root)
	}
	f.symbolic("refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	f.symbolic("HEAD", "refs/heads/feature/b")
	repo := f.open()

	got, err := repo.Branches(0)
	if err != nil {
		t.Fatal(err)
	}
	want := BranchList{Branches: []BranchRef{
		{Name: "a"}, {Name: "feature/b"}, {Name: "main"},
		{Name: "origin/feature/b", Remote: true}, {Name: "origin/main", Remote: true}, {Name: "upstream/dev", Remote: true},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Branches = %+v\nwant %+v", got, want)
	}
	if got, err := repo.Branches(4); err != nil || !got.Truncated || !reflect.DeepEqual(got.Branches, want.Branches[:4]) {
		t.Fatalf("Branches(4) = %+v, %v", got, err)
	}
	if base, err := repo.DefaultBase(); err != nil || base != "origin/main" {
		t.Fatalf("DefaultBase = %q, %v", base, err)
	}
	if branch := repo.Branch(); branch != "feature/b" {
		t.Fatalf("Branch = %q", branch)
	}
}

func TestBranchesInEmptyRepository(t *testing.T) {
	f := newFixture(t)
	repo := f.open()
	if got, err := repo.Branches(0); err != nil || len(got.Branches) != 0 || got.Truncated {
		t.Fatalf("Branches = %+v, %v", got, err)
	}
	if base, err := repo.DefaultBase(); err != nil || base != "" {
		t.Fatalf("DefaultBase = %q, %v", base, err)
	}
}

func TestBaseOverridesTheDefaultBase(t *testing.T) {
	f := newFixture(t)
	g := newBranchGraph(f)
	f.ref("refs/remotes/origin/old", g.m1)
	f.ref("refs/remotes/origin/main", g.m3)
	f.symbolic("refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	// A local branch wins over a remote-tracking one of the same name.
	f.ref("refs/remotes/main", g.m1)
	repo := f.open()
	ctx := context.Background()

	byDefault := logOf(t, repo, LogOptions{Range: RangeBranch})
	if byDefault.Base != "origin/main" || byDefault.Ahead != 4 {
		t.Fatalf("default = Base %q, Ahead %d", byDefault.Base, byDefault.Ahead)
	}
	for _, base := range []string{"main", "origin/main"} {
		named := logOf(t, repo, LogOptions{Range: RangeBranch, Base: base})
		if named.Base != base || !reflect.DeepEqual(named.Commits, byDefault.Commits) || named.Ahead != byDefault.Ahead {
			t.Fatalf("%s = %+v, default = %+v", base, named, byDefault)
		}
	}

	// Against m1 the merged main commits count as the branch's.
	old := logOf(t, repo, LogOptions{Range: RangeBranch, Base: "origin/old"})
	assertHashes(t, "origin/old", hashes(old), g.f4, g.f3, g.m3, g.f2, g.m2, g.f1)
	if old.Base != "origin/old" || old.Ahead != 6 {
		t.Fatalf("origin/old = Base %q, Ahead %d", old.Base, old.Ahead)
	}
	if all := logOf(t, repo, LogOptions{Range: RangeAll, Base: "origin/old"}); all.Base != "origin/old" || all.Ahead != 6 || len(all.Commits) != 7 {
		t.Fatalf("all against origin/old = Base %q, Ahead %d, %d commits", all.Base, all.Ahead, len(all.Commits))
	}
	compared, err := repo.BranchCompare(ctx, "origin/old", "")
	if want := (BranchRange{Base: "origin/old", MergeBase: Hash(g.m1), Head: Hash(g.f4), Ahead: 6}); err != nil || compared != want {
		t.Fatalf("BranchCompare(origin/old) = %+v, %v; want %+v", compared, err, want)
	}
	if compared, _, err := repo.BranchFiles(ctx, "main", "", 0); err != nil || compared.Base != "main" || compared.MergeBase != Hash(g.m3) {
		t.Fatalf("BranchFiles(main) = %+v, %v", compared, err)
	}

	for _, base := range []string{"missing", "origin/HEAD", "refs/heads/main", "../main", "main/", "origin/../main", "HEAD", "-"} {
		if _, err := repo.Log(ctx, LogOptions{Range: RangeBranch, Base: base}); !errors.Is(err, ErrUnknownBase) {
			t.Errorf("Log(Base %q) error = %v, want ErrUnknownBase", base, err)
		}
		if _, err := repo.BranchCompare(ctx, base, ""); !errors.Is(err, ErrUnknownBase) {
			t.Errorf("BranchCompare(%q) error = %v, want ErrUnknownBase", base, err)
		}
		if _, _, err := repo.BranchFiles(ctx, base, "", 0); !errors.Is(err, ErrUnknownBase) {
			t.Errorf("BranchFiles(%q) error = %v, want ErrUnknownBase", base, err)
		}
		if _, _, err := repo.BranchLookup(ctx, base, "", "a.txt", ""); !errors.Is(err, ErrUnknownBase) {
			t.Errorf("BranchLookup(%q) error = %v, want ErrUnknownBase", base, err)
		}
	}
}

func TestBlobHashesMatchGitObjects(t *testing.T) {
	f := newFixture(t)
	content := "# Doc\n"
	commit := f.at(f.tree(map[string]file{"doc.md": regular(content)}), "doc", 100)
	f.ref("refs/heads/master", commit)
	blob, err := f.open().CommitBlob(Hash(commit), "doc.md")
	if err != nil {
		t.Fatal(err)
	}
	// git hash-object of "# Doc\n".
	if want := "14d14195d2c8afc11488df19468b71be3db1acb8"; blob.Hash() != want || BlobHash([]byte(content)) != want {
		t.Fatalf("blob hash = %s, BlobHash = %s, want %s", blob.Hash(), BlobHash([]byte(content)), want)
	}
}

func TestEntriesReportWorktreeSymlinks(t *testing.T) {
	f := newFixture(t)
	f.write("file.md", "# File\n")
	if err := os.Symlink("file.md", filepath.Join(f.dir, "link.md")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	repo := f.open()
	entries, err := repo.Entries([]Change{
		{Path: "file.md", Status: Untracked},
		{Path: "link.md", Status: Untracked},
		{Path: "gone.md", Status: Deleted},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Change: Change{Path: "file.md", Status: Untracked}},
		{Change: Change{Path: "link.md", Status: Untracked}, Symlink: true, Link: "file.md"},
		{Change: Change{Path: "gone.md", Status: Deleted}},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("Entries = %+v\nwant %+v", entries, want)
	}
}
