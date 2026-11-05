package git

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// Head is what a checkout has checked out.
type Head struct {
	// Branch is the checked-out branch's short name; empty when HEAD is
	// detached.
	Branch string
	// Commit is HEAD's full commit hash; empty on an unborn branch.
	Commit string
	// Parent is the commit's first parent's full hash; empty without one.
	Parent string
	// Subject is the first line of the commit's message.
	Subject string
}

// ReadHead reads the HEAD of the worktree whose root is the absolute,
// symlink-evaluated directory dir, a linked worktree's included. A directory
// that is not a worktree returns ErrNotRepository.
func ReadHead(dir string) (Head, error) {
	repo, err := Open(dir)
	if err != nil {
		return Head{}, err
	}
	defer func() { _ = repo.Close() }()
	ref, err := repo.storage.Reference(plumbing.HEAD)
	if err != nil {
		return Head{}, fmt.Errorf("read HEAD: %w", err)
	}
	var head Head
	if ref.Type() == plumbing.SymbolicReference && ref.Target().IsBranch() {
		head.Branch = ref.Target().Short()
	}
	resolved, err := storer.ResolveReference(repo.storage, plumbing.HEAD)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return head, nil
	}
	if err != nil {
		return Head{}, fmt.Errorf("read HEAD: %w", err)
	}
	commit, err := object.GetCommit(repo.storage, resolved.Hash())
	if err != nil {
		return Head{}, fmt.Errorf("read HEAD commit: %w", err)
	}
	head.Commit = commit.Hash.String()
	if len(commit.ParentHashes) > 0 {
		head.Parent = commit.ParentHashes[0].String()
	}
	subject, _, _ := strings.Cut(commit.Message, "\n")
	head.Subject = strings.TrimSpace(subject)
	return head, nil
}
