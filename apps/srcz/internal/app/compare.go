package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/flexdinesh/srcz/internal/diff"
	"github.com/flexdinesh/srcz/internal/document"
	"github.com/flexdinesh/srcz/internal/git"
	"github.com/flexdinesh/srcz/pkg/contract"
)

// ChangeSide is one side of a compared file: a git blob, whose hash is
// known, or a worktree file, hashed once read.
type ChangeSide struct {
	Size int64
	Blob string
	Open func() (io.ReadCloser, error)
}

// CompareSides is a compare's change set and where each change's sides are
// read from. Original is not called for added changes, nor Modified for
// deleted ones.
type CompareSides struct {
	Changes  []git.Change
	Original func(change git.Change) (ChangeSide, error)
	Modified func(change git.Change) (ChangeSide, error)
}

// compareParameters lists, per compare, the request fields it takes.
var compareParameters = map[contract.CompareKind][]string{
	contract.CompareWorking: nil,
	contract.CompareBranch:  {"base", "head", "uncommitted"},
	contract.CompareCommit:  {"commit"},
	contract.CompareRange:   {"from", "to"},
}

// CompareStats counts the lines each file of a compare adds and removes:
// working (as Changes), branch (as BranchFiles), commit (as CommitFiles),
// or range (as RangeFiles). Files keep the list's order and caps.
func (s *SourceControl) CompareStats(ctx context.Context, request contract.CompareStatsRequest) (contract.CompareStatsResponse, error) {
	project, err := s.project(request.Project)
	if err != nil {
		return contract.CompareStatsResponse{}, err
	}
	allowed, ok := compareParameters[request.Compare]
	if !ok {
		return contract.CompareStatsResponse{}, fail(contract.KindInvalid, "Compare must be working, branch, commit, or range.")
	}
	given := map[string]bool{
		"base": request.Base != "", "head": request.Head != "", "uncommitted": request.Uncommitted,
		"commit": request.Commit != "", "from": request.From != "", "to": request.To != "",
	}
	for _, name := range []string{"base", "head", "uncommitted", "commit", "from", "to"} {
		if given[name] && !slices.Contains(allowed, name) {
			return contract.CompareStatsResponse{}, fail(contract.KindInvalid, fmt.Sprintf("%s%s doesn't apply to a %s compare.", strings.ToUpper(name[:1]), name[1:], request.Compare))
		}
	}
	if request.Uncommitted && request.Head != "" {
		return contract.CompareStatsResponse{}, uncommittedHead()
	}
	var hashes []git.Hash
	for _, named := range []struct{ name, value string }{{"commit", request.Commit}, {"from", request.From}, {"to", request.To}} {
		if !slices.Contains(allowed, named.name) {
			continue
		}
		hash, ok := git.ParseHash(named.value)
		if !ok {
			return contract.CompareStatsResponse{}, fail(contract.KindInvalid, InvalidCommitMessage)
		}
		hashes = append(hashes, hash)
	}

	failure := historyFailure
	if request.Compare == contract.CompareWorking || request.Uncommitted {
		failure = statusFailure
	}
	repo, err := git.Open(project.Root)
	if errors.Is(err, git.ErrNotRepository) {
		if len(hashes) > 0 {
			return contract.CompareStatsResponse{}, fail(contract.KindNotFound, CommitUnavailableMessage)
		}
		return contract.CompareStatsResponse{Files: []contract.FileStats{}}, nil
	}
	if err != nil {
		return contract.CompareStatsResponse{}, fail(contract.KindInternal, failure+err.Error())
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	sides, err := ReadCompareSides(ctx, repo, project.Root, s.config.OpenFile, request.Compare, request.Base, request.Head, request.Uncommitted, hashes)
	if err != nil {
		if failure := branchError(err); failure != nil {
			return contract.CompareStatsResponse{}, failure
		}
		if errors.Is(err, git.ErrUnavailable) {
			return contract.CompareStatsResponse{}, fail(contract.KindNotFound, CommitUnavailableMessage)
		}
		return contract.CompareStatsResponse{}, fail(contract.KindInternal, failure+err.Error())
	}
	response, err := s.countCompare(ctx, sides)
	if err != nil {
		return contract.CompareStatsResponse{}, fail(contract.KindInternal, "Couldn't count changed lines: "+err.Error())
	}
	return response, nil
}

// ReadCompareSides lists a compare's changes as its list operation does and
// says where each change's sides are read from: hashes are the commit, or
// from and to, the compare names; worktree files open through openFile,
// root-confined. Compare stats and walkthrough targets (decision 0047) read
// sides through it.
func ReadCompareSides(ctx context.Context, repo *git.Repository, root string, openFile func(string) (fs.File, error), compare contract.CompareKind, base, head string, uncommitted bool, hashes []git.Hash) (CompareSides, error) {
	switch {
	case compare == contract.CompareWorking:
		changes, err := repo.Status(ctx, git.StatusOptions{Limit: changesLimit})
		if err != nil {
			return CompareSides{}, err
		}
		return worktreeSides(repo, root, openFile, changes.Files, repo.HeadBlob)
	case compare == contract.CompareBranch && uncommitted:
		compared, changes, err := repo.BranchWorkingFiles(ctx, base, git.StatusOptions{Limit: changesLimit})
		if err != nil {
			return CompareSides{}, err
		}
		return worktreeSides(repo, root, openFile, changes.Files, func(name string) (git.Blob, error) { return repo.CommitBlob(compared.Reference, name) })
	case compare == contract.CompareBranch:
		compared, changes, err := repo.BranchFiles(ctx, base, head, commitFilesLimit)
		if err != nil {
			return CompareSides{}, err
		}
		return CompareSides{
			Changes:  changes.Files,
			Original: blobSide(func(name string) (git.Blob, error) { return repo.CommitBlob(compared.MergeBase, name) }, true),
			Modified: blobSide(func(name string) (git.Blob, error) { return repo.CommitBlob(compared.Head, name) }, false),
		}, nil
	case compare == contract.CompareCommit:
		hash := hashes[0]
		changes, err := repo.CommitFiles(ctx, hash, commitFilesLimit)
		if err != nil {
			return CompareSides{}, err
		}
		return CompareSides{
			Changes:  changes.Files,
			Original: blobSide(func(name string) (git.Blob, error) { return repo.ParentBlob(hash, name) }, true),
			Modified: blobSide(func(name string) (git.Blob, error) { return repo.CommitBlob(hash, name) }, false),
		}, nil
	default:
		from, to := hashes[0], hashes[1]
		changes, err := repo.RangeFiles(ctx, from, to, commitFilesLimit)
		if err != nil {
			return CompareSides{}, err
		}
		return CompareSides{
			Changes:  changes.Files,
			Original: blobSide(func(name string) (git.Blob, error) { return repo.ParentBlob(from, name) }, true),
			Modified: blobSide(func(name string) (git.Blob, error) { return repo.CommitBlob(to, name) }, false),
		}, nil
	}
}

// worktreeSides reads changes listed against the worktree: original sides
// through blob, modified sides from the worktree.
func worktreeSides(repo *git.Repository, root string, openFile func(string) (fs.File, error), changes []git.Change, blob func(name string) (git.Blob, error)) (CompareSides, error) {
	entries, err := repo.Entries(changes)
	if err != nil {
		return CompareSides{}, err
	}
	byPath := make(map[string]git.Entry, len(entries))
	for _, entry := range entries {
		byPath[entry.Path] = entry
	}
	return CompareSides{
		Changes:  changes,
		Original: blobSide(blob, true),
		Modified: func(change git.Change) (ChangeSide, error) {
			size, open, err := worktreeSource(root, openFile, byPath[change.Path])
			if err != nil {
				return ChangeSide{}, err
			}
			return ChangeSide{Size: size, Open: open}, nil
		},
	}, nil
}

// blobSide reads a change's side from git objects: the original side at a
// rename's old path, the modified side at its path.
func blobSide(read func(name string) (git.Blob, error), original bool) func(git.Change) (ChangeSide, error) {
	return func(change git.Change) (ChangeSide, error) {
		name := change.Path
		if original && change.OldPath != "" {
			name = change.OldPath
		}
		blob, err := read(name)
		if err != nil {
			return ChangeSide{}, fmt.Errorf("read %s: %w", name, err)
		}
		return ChangeSide{Size: blob.Size, Blob: blob.Hash(), Open: blob.Open}, nil
	}
}

// statsBudget is the bytes one compare-stats read may take.
func (s *SourceControl) statsBudget() int64 {
	if s.config.StatsBudget > 0 {
		return s.config.StatsBudget
	}
	return DefaultStatsBudget
}

// countCompare counts each change's lines under the 0004 limits: a side over
// PreviewLimit is tooLarge and never read; a type without a text source or a
// side with a NUL in its sniffed prefix is binary. Reads stop at the
// request's budget: the first change that does not fit, and every change
// after it, report budget.
func (s *SourceControl) countCompare(ctx context.Context, sides CompareSides) (contract.CompareStatsResponse, error) {
	response := contract.CompareStatsResponse{Files: make([]contract.FileStats, 0, len(sides.Changes))}
	left := s.statsBudget()
	for _, change := range sides.Changes {
		if err := ctx.Err(); err != nil {
			return contract.CompareStatsResponse{}, err
		}
		var original, modified *ChangeSide
		if change.Status != git.Added && change.Status != git.Untracked {
			side, err := sides.Original(change)
			if err != nil {
				return contract.CompareStatsResponse{}, err
			}
			original = &side
		}
		if change.Status != git.Deleted {
			side, err := sides.Modified(change)
			if err != nil {
				return contract.CompareStatsResponse{}, fmt.Errorf("read %s: %w", change.Path, err)
			}
			modified = &side
		}
		newer := modified
		if newer == nil {
			newer = original
		}
		present := make([]*ChangeSide, 0, 2)
		var need int64
		tooLarge := false
		for _, side := range []*ChangeSide{original, modified} {
			if side != nil {
				present = append(present, side)
				need += side.Size
				tooLarge = tooLarge || side.Size > document.PreviewLimit
			}
		}

		stats := contract.FileStats{Path: change.Path}
		switch {
		case response.BudgetExceeded:
			stats.Status = contract.StatsBudget
		case tooLarge:
			stats.Status = contract.StatsTooLarge
		case need > left:
			response.BudgetExceeded = true
			stats.Status = contract.StatsBudget
		default:
			contents := map[*ChangeSide][]byte{}
			binary := !s.config.Types.Lookup(change.Path).Type().HasTextSource()
			for _, side := range present {
				content, err := readStatsSide(side)
				if err != nil {
					return contract.CompareStatsResponse{}, fmt.Errorf("read %s: %w", change.Path, err)
				}
				left -= int64(len(content))
				if len(content) > document.PreviewLimit {
					// The file grew past the limit after it was sized.
					tooLarge = true
					continue
				}
				if side.Blob == "" {
					side.Blob = git.BlobHash(content)
				}
				binary = binary || document.IsBinary(content)
				contents[side] = content
			}
			switch {
			case tooLarge:
				stats.Status = contract.StatsTooLarge
			case binary:
				stats.Status = contract.StatsBinary
			default:
				added, deleted := diff.Count(contents[original], contents[modified])
				stats.Status, stats.Additions, stats.Deletions = contract.StatsCounted, &added, &deleted
			}
		}
		stats.Blob = newer.Blob
		response.Files = append(response.Files, stats)
	}
	return response, nil
}

// readStatsSide reads a side, stopping one byte past PreviewLimit.
func readStatsSide(side *ChangeSide) ([]byte, error) {
	reader, err := side.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(io.LimitReader(reader, document.PreviewLimit+1))
}
