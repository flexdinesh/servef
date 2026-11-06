package git

import (
	"context"
	"errors"
	"testing"
)

func branchWorkingFiles(t *testing.T, repo *Repository, base string) (BranchWorking, []Change) {
	t.Helper()
	compared, changes, err := repo.BranchWorkingFiles(context.Background(), base, StatusOptions{})
	if err != nil {
		t.Fatalf("BranchWorkingFiles(%q) error = %v", base, err)
	}
	if changes.Truncated {
		t.Fatal("BranchWorkingFiles() truncated without a limit")
	}
	return compared, changes.Files
}

func TestBranchWorkingFilesCompareMergeBaseWithWorkingTree(t *testing.T) {
	f := newFixture(t)
	f.write("shared.txt", "v0")
	f.write("gone.txt", "g")
	f.write("same.txt", "s")
	f.write("revert.txt", "r0")
	f.write("moved.txt", "m")
	m1 := f.commit("shared.txt", "gone.txt", "same.txt", "revert.txt", "moved.txt")
	f.ref("refs/heads/main", m1)

	// Committed on the branch.
	f.write("shared.txt", "v1")
	f.write("revert.txt", "r1")
	f.write("feature.txt", "f")
	f1 := f.commit("shared.txt", "revert.txt", "feature.txt")

	// Uncommitted on top: revert.txt returns to the merge base's content.
	f.write("revert.txt", "r0")
	f.write("feature.txt", "f2")
	f.write("wip.txt", "w")
	f.write("staged.txt", "s2")
	f.add("staged.txt")
	f.remove("gone.txt")
	f.remove("moved.txt")
	f.write("renamed.txt", "m")
	repo := f.open()

	compared, changes := branchWorkingFiles(t, repo, "main")
	want := BranchWorking{BranchRange: BranchRange{Base: "main", MergeBase: Hash(m1), Head: Hash(f1), Ahead: 1}, Reference: Hash(m1)}
	if compared != want {
		t.Fatalf("range = %+v, want %+v", compared, want)
	}
	assertChanges(t, changes,
		Change{Path: "feature.txt", Status: Added},
		Change{Path: "gone.txt", Status: Deleted},
		Change{Path: "renamed.txt", OldPath: "moved.txt", Status: Renamed},
		Change{Path: "shared.txt", Status: Modified},
		Change{Path: "staged.txt", Status: Added},
		Change{Path: "wip.txt", Status: Untracked},
	)
	// Against HEAD, the reverted file is an uncommitted change.
	if !hasChange(status(t, repo), Change{Path: "revert.txt", Status: Modified}) {
		t.Fatal("Status does not list revert.txt as modified")
	}

	ctx := context.Background()
	for _, tc := range []struct {
		name, oldPath string
		want          Change
	}{
		{"shared.txt", "", Change{Path: "shared.txt", Status: Modified}},
		{"feature.txt", "", Change{Path: "feature.txt", Status: Added}},
		{"wip.txt", "", Change{Path: "wip.txt", Status: Untracked}},
		{"gone.txt", "", Change{Path: "gone.txt", Status: Deleted}},
		{"renamed.txt", "moved.txt", Change{Path: "renamed.txt", OldPath: "moved.txt", Status: Renamed}},
	} {
		_, entry, err := repo.BranchWorkingLookup(ctx, "main", tc.name, tc.oldPath)
		if err != nil || entry.Change != tc.want {
			t.Errorf("BranchWorkingLookup(%s) = %+v, %v; want %+v", tc.name, entry.Change, err, tc.want)
		}
	}
	for _, name := range []string{"revert.txt", "same.txt"} {
		if _, entry, err := repo.BranchWorkingLookup(ctx, "main", name, ""); !errors.Is(err, ErrUnchanged) {
			t.Errorf("BranchWorkingLookup(%s) = %+v, %v; want ErrUnchanged", name, entry, err)
		}
	}
	for _, name := range []string{"missing.txt", "../shared.txt", ".git/HEAD"} {
		if _, entry, err := repo.BranchWorkingLookup(ctx, "main", name, ""); !errors.Is(err, ErrUnavailable) {
			t.Errorf("BranchWorkingLookup(%s) = %+v, %v; want ErrUnavailable", name, entry, err)
		}
	}
	if _, _, err := repo.BranchWorkingFiles(ctx, "nope", StatusOptions{}); !errors.Is(err, ErrUnknownBase) {
		t.Fatalf("unknown base error = %v", err)
	}
	if _, _, err := repo.BranchWorkingLookup(ctx, "nope", "shared.txt", ""); !errors.Is(err, ErrUnknownBase) {
		t.Fatalf("unknown base lookup error = %v", err)
	}
}

func TestBranchWorkingFilesWithNothingAheadMatchStatus(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "a")
	f.write("b.txt", "b")
	tip := f.commit("a.txt", "b.txt")
	f.write("a.txt", "a2")
	f.write("c.txt", "c")
	f.remove("b.txt")
	repo := f.open()

	// Without a base, and with HEAD on the base, Reference is HEAD.
	for _, base := range []string{"", "main"} {
		if base != "" {
			f.ref("refs/heads/main", tip)
		}
		compared, changes := branchWorkingFiles(t, repo, base)
		if compared.Ahead != 0 || compared.Reference != Hash(tip) {
			t.Fatalf("base %q range = %+v, want Reference %s", base, compared, tip)
		}
		assertChanges(t, changes, status(t, repo)...)
	}
}

func TestBranchWorkingFilesWithUnrelatedHistoriesStartFromAnEmptyTree(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "a")
	head := f.commit("a.txt")
	f.write("b.txt", "b")
	f.ref("refs/heads/main", f.empty("unrelated", 100))
	repo := f.open()

	compared, changes := branchWorkingFiles(t, repo, "main")
	if compared.Ahead != 1 || !compared.MergeBase.IsZero() || !compared.Reference.IsZero() || compared.Head != Hash(head) {
		t.Fatalf("range = %+v", compared)
	}
	assertChanges(t, changes,
		Change{Path: "a.txt", Status: Added},
		Change{Path: "b.txt", Status: Untracked},
	)
}

func hasChange(changes []Change, want Change) bool {
	for _, change := range changes {
		if change == want {
			return true
		}
	}
	return false
}
