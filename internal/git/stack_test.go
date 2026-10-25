package git

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

// stackGraph is main at m1 and a stack of branches on it: island (i1),
// then history (h1, HEAD), then rail (r1). side forks from i1 and other
// from m1; old sits on m1.
type stackGraph struct {
	m1, i1, h1, r1, s1, o1 plumbing.Hash
}

func newStackGraph(f *fixture) stackGraph {
	var g stackGraph
	g.m1 = f.at(f.tree(map[string]file{"base.txt": regular("base")}), "m1", 100)
	g.i1 = f.at(f.tree(map[string]file{"base.txt": regular("base"), "island.txt": regular("i")}), "i1", 200, g.m1)
	g.h1 = f.at(f.tree(map[string]file{"base.txt": regular("base"), "island.txt": regular("i"), "history.txt": regular("h")}), "h1", 300, g.i1)
	g.r1 = f.at(f.tree(map[string]file{"base.txt": regular("rail"), "island.txt": regular("i"), "history.txt": regular("h")}), "r1", 400, g.h1)
	g.s1 = f.empty("s1", 250, g.i1)
	g.o1 = f.empty("o1", 150, g.m1)
	f.ref("refs/heads/main", g.m1)
	f.ref("refs/heads/island", g.i1)
	f.ref("refs/heads/history", g.h1)
	f.ref("refs/heads/rail", g.r1)
	f.ref("refs/heads/side", g.s1)
	f.ref("refs/heads/other", g.o1)
	f.ref("refs/heads/old", g.m1)
	f.ref("refs/remotes/origin/rail", g.r1)
	f.symbolic("HEAD", "refs/heads/history")
	return g
}

func stackOf(t *testing.T, repo *Repository, base string) Stack {
	t.Helper()
	stack, err := repo.Stack(context.Background(), base)
	if err != nil {
		t.Fatalf("Stack(%q) error = %v", base, err)
	}
	return stack
}

func layer(name string, head plumbing.Hash, parent string, ahead int, aliases ...string) StackLayer {
	if aliases == nil {
		aliases = []string{}
	}
	return StackLayer{Name: name, Aliases: aliases, Head: Hash(head), Parent: parent, Ahead: ahead}
}

func current(l StackLayer) StackLayer {
	l.Current = true
	return l
}

func assertStack(t *testing.T, label string, got, want Stack) {
	t.Helper()
	if want.Layers == nil {
		want.Layers = []StackLayer{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %+v\nwant %+v", label, got, want)
	}
}

func TestBranchCompareWithHeadBranch(t *testing.T) {
	f := newFixture(t)
	g := newStackGraph(f)
	repo := f.open()
	ctx := context.Background()

	// A layer against the base: everything the stack adds up to it.
	compared, err := repo.BranchCompare(ctx, "", "rail")
	if want := (BranchRange{Base: "main", MergeBase: Hash(g.m1), Head: Hash(g.r1), Ahead: 3}); err != nil || compared != want {
		t.Fatalf("rail vs main = %+v, %v; want %+v", compared, err, want)
	}
	// A layer against its parent branch: only its own commit.
	compared, files, err := repo.BranchFiles(ctx, "history", "rail", 0)
	if want := (BranchRange{Base: "history", MergeBase: Hash(g.h1), Head: Hash(g.r1), Ahead: 1}); err != nil || compared != want {
		t.Fatalf("rail vs history = %+v, %v; want %+v", compared, err, want)
	}
	assertChanges(t, files.Files, Change{Path: "base.txt", Status: Modified})
	if _, change, err := repo.BranchLookup(ctx, "history", "rail", "base.txt", ""); err != nil || change != (Change{Path: "base.txt", Status: Modified}) {
		t.Fatalf("BranchLookup(base.txt) = %+v, %v", change, err)
	}
	if _, _, err := repo.BranchLookup(ctx, "history", "rail", "history.txt", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("BranchLookup(history.txt) error = %v, want ErrUnavailable", err)
	}
	// Naming HEAD's branch answers as HEAD does.
	named, err := repo.BranchCompare(ctx, "", "history")
	if err != nil || named != compareBranch(t, repo) {
		t.Fatalf("history = %+v, %v; HEAD = %+v", named, err, compareBranch(t, repo))
	}
	// A head the base already holds is nothing ahead.
	if behind, err := repo.BranchCompare(ctx, "rail", "island"); err != nil || behind != (BranchRange{Base: "rail", Head: Hash(g.i1)}) {
		t.Fatalf("island vs rail = %+v, %v", behind, err)
	}

	for _, head := range []string{"missing", "origin/rail", "refs/heads/rail", "../rail", "rail/", "HEAD", "-"} {
		if _, err := repo.BranchCompare(ctx, "", head); !errors.Is(err, ErrUnknownHead) {
			t.Errorf("BranchCompare(head %q) error = %v, want ErrUnknownHead", head, err)
		}
		if _, _, err := repo.BranchFiles(ctx, "", head, 0); !errors.Is(err, ErrUnknownHead) {
			t.Errorf("BranchFiles(head %q) error = %v, want ErrUnknownHead", head, err)
		}
		if _, _, err := repo.BranchLookup(ctx, "", head, "base.txt", ""); !errors.Is(err, ErrUnknownHead) {
			t.Errorf("BranchLookup(head %q) error = %v, want ErrUnknownHead", head, err)
		}
	}
	if _, err := repo.BranchCompare(ctx, "missing", "missing"); !errors.Is(err, ErrUnknownBase) {
		t.Fatalf("unknown base and head error = %v, want ErrUnknownBase", err)
	}
}

func TestStackListsLayersBelowAndAboveHead(t *testing.T) {
	f := newFixture(t)
	g := newStackGraph(f)
	repo := f.open()

	// HEAD on the middle layer: side forks below HEAD without holding it,
	// other forks from the base, and old is on the base; none is a layer.
	assertStack(t, "middle", stackOf(t, repo, ""), Stack{
		Base: "main", MergeBase: Hash(g.m1), Head: Hash(g.h1), Branch: "history",
		Layers: []StackLayer{
			layer("rail", g.r1, "history", 1),
			current(layer("history", g.h1, "island", 1)),
			layer("island", g.i1, "main", 1),
		},
	})
	if named := stackOf(t, repo, "main"); !reflect.DeepEqual(named, stackOf(t, repo, "")) {
		t.Fatalf("base main = %+v", named)
	}

	// HEAD on the bottom layer: every other layer is above it, side now too.
	f.symbolic("HEAD", "refs/heads/island")
	assertStack(t, "bottom", stackOf(t, repo, ""), Stack{
		Base: "main", MergeBase: Hash(g.m1), Head: Hash(g.i1), Branch: "island",
		Layers: []StackLayer{
			layer("rail", g.r1, "history", 1),
			layer("history", g.h1, "island", 1),
			layer("side", g.s1, "island", 1),
			current(layer("island", g.i1, "main", 1)),
		},
	})

	// HEAD on the top layer: every other layer is below it.
	f.symbolic("HEAD", "refs/heads/rail")
	assertStack(t, "top", stackOf(t, repo, ""), Stack{
		Base: "main", MergeBase: Hash(g.m1), Head: Hash(g.r1), Branch: "rail",
		Layers: []StackLayer{
			current(layer("rail", g.r1, "history", 1)),
			layer("history", g.h1, "island", 1),
			layer("island", g.i1, "main", 1),
		},
	})

	// Against island, island is the base and history the first layer.
	f.symbolic("HEAD", "refs/heads/history")
	assertStack(t, "base island", stackOf(t, repo, "island"), Stack{
		Base: "island", MergeBase: Hash(g.i1), Head: Hash(g.h1), Branch: "history",
		Layers: []StackLayer{
			layer("rail", g.r1, "history", 1),
			current(layer("history", g.h1, "island", 1)),
		},
	})
}

func TestStackListsSiblingsAboveHead(t *testing.T) {
	f := newFixture(t)
	g := newStackGraph(f)
	a := f.empty("a", 500, g.h1)
	b := f.empty("b", 510, f.empty("b0", 505, g.h1))
	f.ref("refs/heads/b", b)
	f.ref("refs/heads/a", a)

	assertStack(t, "siblings", stackOf(t, f.open(), ""), Stack{
		Base: "main", MergeBase: Hash(g.m1), Head: Hash(g.h1), Branch: "history",
		Layers: []StackLayer{
			layer("b", b, "history", 2),
			layer("a", a, "history", 1),
			layer("rail", g.r1, "history", 1),
			current(layer("history", g.h1, "island", 1)),
			layer("island", g.i1, "main", 1),
		},
	})
}

func TestStackMergesBranchesAtOneTip(t *testing.T) {
	f := newFixture(t)
	g := newStackGraph(f)
	f.ref("refs/heads/island-copy", g.i1)
	f.ref("refs/heads/an-island", g.i1)
	f.ref("refs/heads/history-copy", g.h1)
	f.ref("refs/heads/a-history", g.h1)

	assertStack(t, "aliases", stackOf(t, f.open(), ""), Stack{
		Base: "main", MergeBase: Hash(g.m1), Head: Hash(g.h1), Branch: "history",
		Layers: []StackLayer{
			layer("rail", g.r1, "history", 1),
			current(layer("history", g.h1, "an-island", 1, "a-history", "history-copy")),
			layer("an-island", g.i1, "main", 1, "island", "island-copy"),
		},
	})
}

func TestStackOnDetachedHead(t *testing.T) {
	f := newFixture(t)
	g := newStackGraph(f)
	f.ref("HEAD", g.h1)
	repo := f.open()

	// The detached layer takes the branch at its tip as an alias and is never
	// a parent.
	assertStack(t, "detached at history", stackOf(t, repo, ""), Stack{
		Base: "main", MergeBase: Hash(g.m1), Head: Hash(g.h1),
		Layers: []StackLayer{
			layer("rail", g.r1, "island", 2),
			current(layer("", g.h1, "island", 1, "history")),
			layer("island", g.i1, "main", 1),
		},
	})

	// Detached between two layers, the layer above skips it as a parent.
	between := f.empty("between", 350, g.h1)
	rail := f.empty("rail", 400, between)
	f.ref("refs/heads/rail", rail)
	f.ref("HEAD", between)
	assertStack(t, "detached between", stackOf(t, repo, ""), Stack{
		Base: "main", MergeBase: Hash(g.m1), Head: Hash(between),
		Layers: []StackLayer{
			layer("rail", rail, "history", 2),
			current(layer("", between, "history", 1)),
			layer("history", g.h1, "island", 1),
			layer("island", g.i1, "main", 1),
		},
	})
}

func TestStackWithNothingAhead(t *testing.T) {
	f := newFixture(t)
	g := newStackGraph(f)
	repo := f.open()

	f.symbolic("HEAD", "refs/heads/old")
	assertStack(t, "on base", stackOf(t, repo, ""), Stack{Base: "main", Head: Hash(g.m1), Branch: "old"})
	f.ref("refs/heads/main", g.i1)
	assertStack(t, "behind base", stackOf(t, repo, ""), Stack{Base: "main", Head: Hash(g.m1), Branch: "old"})

	f.symbolic("HEAD", "refs/heads/unborn")
	assertStack(t, "unborn", stackOf(t, repo, ""), Stack{Base: "main", Branch: "unborn"})

	if err := f.repo.Storer.RemoveReference("refs/heads/main"); err != nil {
		t.Fatal(err)
	}
	f.symbolic("HEAD", "refs/heads/history")
	assertStack(t, "no base", stackOf(t, repo, ""), Stack{Head: Hash(g.h1), Branch: "history"})
}

func TestStackOfUnrelatedHistories(t *testing.T) {
	f := newFixture(t)
	m1 := f.empty("m1", 100)
	u1 := f.empty("u1", 200)
	u2 := f.empty("u2", 300, u1)
	v1 := f.empty("v1", 400, u2)
	f.ref("refs/heads/main", m1)
	f.ref("refs/heads/feature", u2)
	f.ref("refs/heads/lower", u1)
	f.ref("refs/heads/upper", v1)
	f.symbolic("HEAD", "refs/heads/feature")

	assertStack(t, "unrelated", stackOf(t, f.open(), ""), Stack{
		Base: "main", Head: Hash(u2), Branch: "feature",
		Layers: []StackLayer{
			layer("upper", v1, "feature", 1),
			current(layer("feature", u2, "lower", 1)),
			layer("lower", u1, "main", 1),
		},
	})
}

func TestStackIsBounded(t *testing.T) {
	f := newFixture(t)
	g := newStackGraph(f)
	repo := f.open()
	ctx := context.Background()

	defer func(limit int) { stackCandidateLimit = limit }(stackCandidateLimit)
	// Sorted, the first two candidates are island and old.
	stackCandidateLimit = 2
	assertStack(t, "candidate limit", stackOf(t, repo, ""), Stack{
		Base: "main", MergeBase: Hash(g.m1), Head: Hash(g.h1), Branch: "history", Truncated: true,
		Layers: []StackLayer{
			current(layer("history", g.h1, "island", 1)),
			layer("island", g.i1, "main", 1),
		},
	})
	stackCandidateLimit = 100

	defer func(branch, walk int) { branchLimit, walkLimit = branch, walk }(branchLimit, walkLimit)
	// rail's range is past the branch limit; candidates after it are skipped.
	branchLimit = 2
	assertStack(t, "branch limit", stackOf(t, repo, ""), Stack{
		Base: "main", MergeBase: Hash(g.m1), Head: Hash(g.h1), Branch: "history", Truncated: true,
		Layers: []StackLayer{
			current(layer("history", g.h1, "island", 1)),
			layer("island", g.i1, "main", 1),
		},
	})
	// HEAD's own range past the bounds fails as BranchCompare does.
	branchLimit = 1
	if _, err := repo.Stack(ctx, ""); !errors.Is(err, ErrRangeTooLarge) || err.Error() != "the branch has too many commits to compare with main" {
		t.Fatalf("HEAD past bounds error = %v", err)
	}
	branchLimit = 10_000

	if _, err := repo.Stack(ctx, "missing"); !errors.Is(err, ErrUnknownBase) {
		t.Fatalf("unknown base error = %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.Stack(cancelled, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}
}
