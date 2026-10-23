package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/git"
)

const (
	// refsLimit caps the branches one refs response lists.
	refsLimit = 1000
	// compareStatsBudget bounds the bytes one compare-stats request reads.
	compareStatsBudget = 64 << 20
)

// Compares, like changes and history, are repository-scoped: refs and stats
// cover every change whatever the CLI filename scope.

type refsResponse struct {
	Repository  bool            `json:"repository"`
	Branch      string          `json:"branch"`
	DefaultBase string          `json:"defaultBase"`
	Branches    []git.BranchRef `json:"branches"`
	Truncated   bool            `json:"truncated"`
}

// serveRefs lists the branches a compare may use as its base, and the
// default one.
func (a *App) serveRefs(w http.ResponseWriter, projectID string) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	response := refsResponse{Branches: []git.BranchRef{}}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		writeJSON(w, http.StatusOK, response)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git branches: " + err.Error()})
		return
	}
	defer func() { _ = repo.Close() }()

	branches, err := repo.Branches(refsLimit)
	if err == nil {
		response.DefaultBase, err = repo.DefaultBase()
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git branches: " + err.Error()})
		return
	}
	response.Repository = true
	response.Branch = repo.Branch()
	if branches.Branches != nil {
		response.Branches = branches.Branches
	}
	response.Truncated = branches.Truncated
	writeJSON(w, http.StatusOK, response)
}

// statsStatus says whether a file's lines were counted, and why not.
type statsStatus string

const (
	statsCounted  statsStatus = "counted"
	statsBinary   statsStatus = "binary"
	statsTooLarge statsStatus = "tooLarge"
	statsBudget   statsStatus = "budget"
)

// fileStats is one compared file's line counts, present only when counted.
// Blob is the git blob hash of the newer side, or the older side for a
// deleted file; empty when that side is in the worktree and was not read.
type fileStats struct {
	Path      string      `json:"path"`
	Status    statsStatus `json:"status"`
	Additions *int        `json:"additions,omitempty"`
	Deletions *int        `json:"deletions,omitempty"`
	Blob      string      `json:"blob"`
}

type compareStatsResponse struct {
	Files []fileStats `json:"files"`
	// BudgetExceeded reports that the read budget left files uncounted.
	BudgetExceeded bool `json:"budgetExceeded"`
}

// statsSide is one side of a compared file: a git blob, whose hash is
// known, or a worktree file, hashed once read.
type statsSide struct {
	size int64
	blob string
	open func() (io.ReadCloser, error)
}

// compareSides reads a compare's change set and the sides of each change.
// original is not called for added changes, nor modified for deleted ones.
type compareSides struct {
	changes  []git.Change
	original func(change git.Change) (statsSide, error)
	modified func(change git.Change) (statsSide, error)
}

// serveCompareStats counts the lines each file of a compare adds and
// removes: compare=working (as /api/changes), branch (base; as
// /api/branch-files), commit (commit; as /api/commit-files), or range (from
// and to; as /api/range-files). Files keep the list's order and caps.
func (a *App) serveCompareStats(w http.ResponseWriter, r *http.Request, projectID string, query url.Values) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	compare := query.Get("compare")
	allowed := map[string]bool{}
	switch compare {
	case "working":
	case "branch":
		allowed["base"] = true
	case "commit":
		allowed["commit"] = true
	case "range":
		allowed["from"], allowed["to"] = true, true
	default:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Compare must be working, branch, commit, or range."})
		return
	}
	for _, name := range []string{"base", "commit", "from", "to"} {
		if query.Has(name) && !allowed[name] {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("%s%s doesn't apply to a %s compare.", strings.ToUpper(name[:1]), name[1:], compare)})
			return
		}
	}
	var hashes []git.Hash
	for _, name := range []string{"commit", "from", "to"} {
		if !allowed[name] {
			continue
		}
		hash, ok := git.ParseHash(query.Get(name))
		if !ok {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: invalidCommitMessage})
			return
		}
		hashes = append(hashes, hash)
	}

	failure := "Couldn't read git history: "
	if compare == "working" {
		failure = "Couldn't read git status: "
	}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		if len(hashes) > 0 {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: commitUnavailableMessage})
			return
		}
		writeJSON(w, http.StatusOK, compareStatsResponse{Files: []fileStats{}})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: failure + err.Error()})
		return
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	sides, err := a.compareSides(ctx, repo, project.root, compare, query.Get("base"), hashes)
	switch {
	case writeUnknownBase(w, err):
		return
	case errors.Is(err, git.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: commitUnavailableMessage})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: failure + err.Error()})
		return
	}
	response, err := a.countCompare(ctx, sides)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't count changed lines: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// compareSides lists a compare's changes as its list endpoint does and
// says where each change's sides are read from.
func (a *App) compareSides(ctx context.Context, repo *git.Repository, root, compare, base string, hashes []git.Hash) (compareSides, error) {
	switch compare {
	case "working":
		changes, err := repo.Status(ctx, git.StatusOptions{Limit: changesLimit})
		if err != nil {
			return compareSides{}, err
		}
		entries, err := repo.Entries(changes.Files)
		if err != nil {
			return compareSides{}, err
		}
		byPath := make(map[string]git.Entry, len(entries))
		for _, entry := range entries {
			byPath[entry.Path] = entry
		}
		return compareSides{
			changes:  changes.Files,
			original: blobSide(repo.HeadBlob, true),
			modified: func(change git.Change) (statsSide, error) { return a.worktreeStatsSide(root, byPath[change.Path]) },
		}, nil
	case "branch":
		compared, changes, err := repo.BranchFiles(ctx, base, commitFilesLimit)
		if err != nil {
			return compareSides{}, err
		}
		return compareSides{
			changes:  changes.Files,
			original: blobSide(func(name string) (git.Blob, error) { return repo.CommitBlob(compared.MergeBase, name) }, true),
			modified: blobSide(func(name string) (git.Blob, error) { return repo.CommitBlob(compared.Head, name) }, false),
		}, nil
	case "commit":
		hash := hashes[0]
		changes, err := repo.CommitFiles(ctx, hash, commitFilesLimit)
		if err != nil {
			return compareSides{}, err
		}
		return compareSides{
			changes:  changes.Files,
			original: blobSide(func(name string) (git.Blob, error) { return repo.ParentBlob(hash, name) }, true),
			modified: blobSide(func(name string) (git.Blob, error) { return repo.CommitBlob(hash, name) }, false),
		}, nil
	default:
		from, to := hashes[0], hashes[1]
		changes, err := repo.RangeFiles(ctx, from, to, commitFilesLimit)
		if err != nil {
			return compareSides{}, err
		}
		return compareSides{
			changes:  changes.Files,
			original: blobSide(func(name string) (git.Blob, error) { return repo.ParentBlob(from, name) }, true),
			modified: blobSide(func(name string) (git.Blob, error) { return repo.CommitBlob(to, name) }, false),
		}, nil
	}
}

// blobSide reads a change's side from git objects: the original side at a
// rename's old path, the modified side at its path.
func blobSide(read func(name string) (git.Blob, error), original bool) func(git.Change) (statsSide, error) {
	return func(change git.Change) (statsSide, error) {
		name := change.Path
		if original && change.OldPath != "" {
			name = change.OldPath
		}
		blob, err := read(name)
		if err != nil {
			return statsSide{}, fmt.Errorf("read %s: %w", name, err)
		}
		return statsSide{size: blob.Size, blob: blob.Hash(), open: blob.Open}, nil
	}
}

// worktreeStatsSide is a validated change's worktree side: a symlink's
// target text, or the root-confined regular file.
func (a *App) worktreeStatsSide(root string, entry git.Entry) (statsSide, error) {
	if entry.Symlink {
		link := []byte(entry.Link)
		return statsSide{size: int64(len(link)), open: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(link)), nil
		}}, nil
	}
	resolved, err := files.ResolveFile(root, entry.Path)
	if err != nil {
		return statsSide{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return statsSide{}, err
	}
	return statsSide{size: info.Size(), open: func() (io.ReadCloser, error) { return a.openFile(resolved) }}, nil
}

// countCompare counts each change's lines under the 0004 limits: a side over
// PreviewLimit is tooLarge and never read; a type without a text source or a
// side with a NUL in its sniffed prefix is binary. Reads stop at the
// request's budget: the first change that does not fit, and every change
// after it, report budget.
func (a *App) countCompare(ctx context.Context, sides compareSides) (compareStatsResponse, error) {
	response := compareStatsResponse{Files: make([]fileStats, 0, len(sides.changes))}
	left := a.statsBudget
	for _, change := range sides.changes {
		if err := ctx.Err(); err != nil {
			return compareStatsResponse{}, err
		}
		var original, modified *statsSide
		if change.Status != git.Added && change.Status != git.Untracked {
			side, err := sides.original(change)
			if err != nil {
				return compareStatsResponse{}, err
			}
			original = &side
		}
		if change.Status != git.Deleted {
			side, err := sides.modified(change)
			if err != nil {
				return compareStatsResponse{}, fmt.Errorf("read %s: %w", change.Path, err)
			}
			modified = &side
		}
		newer := modified
		if newer == nil {
			newer = original
		}
		present := make([]*statsSide, 0, 2)
		var need int64
		tooLarge := false
		for _, side := range []*statsSide{original, modified} {
			if side != nil {
				present = append(present, side)
				need += side.size
				tooLarge = tooLarge || side.size > document.PreviewLimit
			}
		}

		stats := fileStats{Path: change.Path}
		switch {
		case response.BudgetExceeded:
			stats.Status = statsBudget
		case tooLarge:
			stats.Status = statsTooLarge
		case need > left:
			response.BudgetExceeded = true
			stats.Status = statsBudget
		default:
			contents := map[*statsSide][]byte{}
			binary := !a.config.Types.Lookup(change.Path).Type().HasTextSource()
			for _, side := range present {
				content, err := readStatsSide(side)
				if err != nil {
					return compareStatsResponse{}, fmt.Errorf("read %s: %w", change.Path, err)
				}
				left -= int64(len(content))
				if len(content) > document.PreviewLimit {
					// The file grew past the limit after it was sized.
					tooLarge = true
					continue
				}
				if side.blob == "" {
					side.blob = git.BlobHash(content)
				}
				binary = binary || document.IsBinary(content)
				contents[side] = content
			}
			switch {
			case tooLarge:
				stats.Status = statsTooLarge
			case binary:
				stats.Status = statsBinary
			default:
				added, deleted := git.LineChanges(contents[original], contents[modified])
				stats.Status, stats.Additions, stats.Deletions = statsCounted, &added, &deleted
			}
		}
		stats.Blob = newer.blob
		response.Files = append(response.Files, stats)
	}
	return response, nil
}

// readStatsSide reads a side, stopping one byte past PreviewLimit.
func readStatsSide(side *statsSide) ([]byte, error) {
	reader, err := side.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(io.LimitReader(reader, document.PreviewLimit+1))
}
