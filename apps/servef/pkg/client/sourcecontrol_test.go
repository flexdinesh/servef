package client

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/flexdinesh/servef/pkg/contract"
)

// repoFixture is a repository whose HEAD, feature, is one commit ahead of
// main, with an uncommitted change and an untracked file.
type repoFixture struct {
	fixture
	main, feature string
}

func newRepoFixture(t *testing.T) repoFixture {
	t.Helper()
	root := t.TempDir()
	repo, err := gogit.PlainInitWithOptions(root, &gogit.PlainInitOptions{InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	commit := func(message string) string {
		t.Helper()
		if err := worktree.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
			t.Fatal(err)
		}
		hash, err := worktree.Commit(message, &gogit.CommitOptions{Author: &object.Signature{Name: "servef", Email: "servef@example.com", When: time.Unix(1700000000, 0)}})
		if err != nil {
			t.Fatal(err)
		}
		return hash.String()
	}
	write("README.md", "# One\n")
	write("lines.txt", "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\n")
	main := commit("Initial commit")
	if err := worktree.Checkout(&gogit.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("feature"), Create: true}); err != nil {
		t.Fatal(err)
	}
	write("lines.txt", "a\nB\nc\nd\ne\nf\ng\nh\ni\nj\nk\nL\nm\n")
	write("added.txt", "new\n")
	feature := commit("Feature\n\nBody.")
	write("README.md", "# Two\n")
	write("untracked.txt", "loose\n")
	return repoFixture{fixture: serveFixture(t, root), main: main, feature: feature}
}

func TestSourceControlParityAcrossTransports(t *testing.T) {
	f := newRepoFixture(t)
	ctx := context.Background()
	missing := filepath.Join(f.root, "gone")
	cases := []struct {
		name string
		call func(Client) outcome
		kind contract.ErrorKind
		code string
	}{
		{"Changes", func(c Client) outcome { return result(c.Changes(ctx, f.root)) }, "", ""},
		{"Changes without a project", func(c Client) outcome { return result(c.Changes(ctx, "")) }, contract.KindInvalid, ""},
		{"Changes of a missing project", func(c Client) outcome { return result(c.Changes(ctx, missing)) }, contract.KindNotFound, ""},
		{"Diff working", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Path: "README.md"}))
		}, "", ""},
		{"Diff untracked", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Path: "untracked.txt"}))
		}, "", ""},
		{"Diff ignoring whitespace", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Path: "README.md", IgnoreWhitespace: true}))
		}, "", ""},
		{"Diff unchanged", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Path: "lines.txt"}))
		}, contract.KindNotFound, contract.CodeUnchanged},
		{"Diff commit", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Kind: contract.DiffCommit, Commit: f.feature, Path: "lines.txt"}))
		}, "", ""},
		{"Diff bad commit", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Kind: contract.DiffCommit, Commit: "HEAD", Path: "lines.txt"}))
		}, contract.KindInvalid, ""},
		{"Diff branch", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Kind: contract.DiffBranch, Base: "main", Path: "lines.txt"}))
		}, "", ""},
		{"Diff branch with uncommitted", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Kind: contract.DiffBranch, Base: "main", Uncommitted: true, Path: "README.md"}))
		}, "", ""},
		{"Diff unknown base", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Kind: contract.DiffBranch, Base: "nope", Path: "lines.txt"}))
		}, contract.KindInvalid, contract.CodeUnknownBase},
		{"Diff base on a working diff", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Base: "main", Path: "README.md"}))
		}, contract.KindInvalid, ""},
		{"Diff range", func(c Client) outcome {
			return result(c.Diff(ctx, contract.DiffRequest{Project: f.root, Kind: contract.DiffRange, From: f.feature, To: f.feature, Path: "added.txt"}))
		}, "", ""},
		{"Commits", func(c Client) outcome {
			return result(c.Commits(ctx, contract.CommitsRequest{Project: f.root, Range: contract.RangeAll}))
		}, "", ""},
		{"Commits on the branch", func(c Client) outcome {
			return result(c.Commits(ctx, contract.CommitsRequest{Project: f.root, Base: "main", Limit: 1}))
		}, "", ""},
		{"Commits from a bad head", func(c Client) outcome {
			return result(c.Commits(ctx, contract.CommitsRequest{Project: f.root, Head: "main"}))
		}, contract.KindInvalid, ""},
		{"CommitFiles", func(c Client) outcome { return result(c.CommitFiles(ctx, f.root, f.feature)) }, "", ""},
		{"CommitFiles of an unknown commit", func(c Client) outcome {
			return result(c.CommitFiles(ctx, f.root, "0123456789012345678901234567890123456789"))
		}, contract.KindNotFound, ""},
		{"BranchFiles", func(c Client) outcome {
			return result(c.BranchFiles(ctx, contract.BranchFilesRequest{Project: f.root, Base: "main"}))
		}, "", ""},
		{"BranchFiles with uncommitted", func(c Client) outcome {
			return result(c.BranchFiles(ctx, contract.BranchFilesRequest{Project: f.root, Uncommitted: true}))
		}, "", ""},
		{"BranchFiles of an unknown head", func(c Client) outcome {
			return result(c.BranchFiles(ctx, contract.BranchFilesRequest{Project: f.root, Head: "nope"}))
		}, contract.KindInvalid, contract.CodeUnknownHead},
		{"BranchFiles with head and uncommitted", func(c Client) outcome {
			return result(c.BranchFiles(ctx, contract.BranchFilesRequest{Project: f.root, Head: "feature", Uncommitted: true}))
		}, contract.KindInvalid, ""},
		{"RangeFiles", func(c Client) outcome { return result(c.RangeFiles(ctx, f.root, f.main, f.feature)) }, "", ""},
		{"Refs", func(c Client) outcome { return result(c.Refs(ctx, f.root)) }, "", ""},
		{"BranchStack", func(c Client) outcome { return result(c.BranchStack(ctx, f.root, "")) }, "", ""},
		{"BranchStack on an unknown base", func(c Client) outcome { return result(c.BranchStack(ctx, f.root, "nope")) }, contract.KindInvalid, contract.CodeUnknownBase},
		{"CompareStats working", func(c Client) outcome {
			return result(c.CompareStats(ctx, contract.CompareStatsRequest{Project: f.root, Compare: contract.CompareWorking}))
		}, "", ""},
		{"CompareStats branch", func(c Client) outcome {
			return result(c.CompareStats(ctx, contract.CompareStatsRequest{Project: f.root, Compare: contract.CompareBranch, Base: "main", Uncommitted: true}))
		}, "", ""},
		{"CompareStats commit", func(c Client) outcome {
			return result(c.CompareStats(ctx, contract.CompareStatsRequest{Project: f.root, Compare: contract.CompareCommit, Commit: f.feature}))
		}, "", ""},
		{"CompareStats with a stray parameter", func(c Client) outcome {
			return result(c.CompareStats(ctx, contract.CompareStatsRequest{Project: f.root, Compare: contract.CompareWorking, Commit: f.feature}))
		}, contract.KindInvalid, ""},
		{"CompareStats of an unknown compare", func(c Client) outcome {
			return result(c.CompareStats(ctx, contract.CompareStatsRequest{Project: f.root, Compare: "nope"}))
		}, contract.KindInvalid, ""},
	}
	outcomes := map[string]outcome{}
	for _, tc := range cases {
		got := f.same(t, tc.name, tc.call)
		outcomes[tc.name] = got
		if kindOf(got.Err) != tc.kind {
			t.Errorf("%s error = %v, want kind %q", tc.name, got.Err, tc.kind)
		}
		if code := codeOf(got.Err); code != tc.code {
			t.Errorf("%s code = %q, want %q", tc.name, code, tc.code)
		}
	}

	branch := outcomes["Diff branch"]
	response, ok := branch.Value.(contract.DiffResponse)
	if !ok || response.LineDiff == nil {
		t.Fatalf("Diff branch = %+v, want a line diff", branch)
	}
	if lines := response.LineDiff; lines.Additions != 2 || lines.Deletions != 2 || len(lines.Hunks) != 2 || len(lines.Gaps) != 1 {
		t.Errorf("Diff branch line diff = %+v, want two hunks of one change each around one gap", lines)
	}
	stats := outcomes["CompareStats commit"]
	counted, ok := stats.Value.(contract.CompareStatsResponse)
	if !ok || len(counted.Files) != 2 {
		t.Fatalf("CompareStats commit = %+v, want two files", stats)
	}
	for _, file := range counted.Files {
		if file.Status != contract.StatsCounted || file.Additions == nil || file.Deletions == nil {
			t.Errorf("CompareStats commit file = %+v, want counted", file)
		}
	}
}

func codeOf(err error) string {
	var failure *Error
	if errors.As(err, &failure) {
		return failure.Code
	}
	return ""
}
