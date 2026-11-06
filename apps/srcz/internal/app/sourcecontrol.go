package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/flexdinesh/srcz/internal/diff"
	"github.com/flexdinesh/srcz/internal/document"
	"github.com/flexdinesh/srcz/internal/files"
	"github.com/flexdinesh/srcz/internal/git"
	"github.com/flexdinesh/srcz/pkg/contract"
)

// Source Control is repository-scoped: it lists every change in the
// project's checkout, whatever the browse depth, and reads worktree files
// only after the git package validates them as changes. History, branches,
// ranges, and compare stats are repository-scoped too; file sides are read
// from git objects only after the git package validates them as changes.

const (
	// changesLimit caps the changes one status read lists.
	changesLimit = 5000
	// statusTimeout bounds one status or history read.
	statusTimeout = 20 * time.Second
	// commitsDefaultLimit and commitsMaxLimit bound one history page.
	commitsDefaultLimit = 50
	commitsMaxLimit     = 100
	// commitFilesLimit caps the changes one commit, branch, or range lists.
	commitFilesLimit = 1000
	// refsLimit caps the branches one refs read lists.
	refsLimit = 1000
	// authorTimeLayout is RFC 3339 with a numeric offset, never Z, so the
	// author's own zone shows.
	authorTimeLayout = "2006-01-02T15:04:05-07:00"
	// DefaultStatsBudget bounds the bytes one compare-stats read takes.
	DefaultStatsBudget = 64 << 20
)

// Source control messages.
const (
	ChangeUnavailableMessage = "That change is unavailable."
	NoFileMessage            = "No file was selected."
	UnknownBaseMessage       = "That base branch doesn't exist."
	UnknownHeadMessage       = "That head branch doesn't exist."
	statusFailure            = "Couldn't read git status: "
	historyFailure           = "Couldn't read git history: "
	branchesFailure          = "Couldn't read git branches: "
)

// SourceControl reads a project's changes, diffs, history, branches, and
// compare stats. Every operation opens the repository again.
type SourceControl struct{ *state }

// Changes lists the working tree's changes since HEAD; a folder project has
// none.
func (s *SourceControl) Changes(ctx context.Context, projectID string) (contract.ChangesResponse, error) {
	project, err := s.project(projectID)
	if err != nil {
		return contract.ChangesResponse{}, err
	}
	response := contract.ChangesResponse{Files: []contract.ChangedFile{}, Warnings: []string{}}
	repo, err := git.Open(project.Root)
	if errors.Is(err, git.ErrNotRepository) {
		return response, nil
	}
	if err != nil {
		return contract.ChangesResponse{}, fail(contract.KindInternal, statusFailure+err.Error())
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	changes, err := repo.Status(ctx, git.StatusOptions{Limit: changesLimit})
	if err != nil {
		return contract.ChangesResponse{}, fail(contract.KindInternal, statusFailure+err.Error())
	}
	response.Repository = true
	response.Branch = repo.Branch()
	response.Files = changedFiles(changes.Files)
	response.Truncated = changes.Truncated
	response.Warnings = warningStrings(changes.Warnings)
	return response, nil
}

// Commits pages the history of HEAD, or of request.Head, against the base
// branch.
func (s *SourceControl) Commits(ctx context.Context, request contract.CommitsRequest) (contract.CommitsResponse, error) {
	project, err := s.project(request.Project)
	if err != nil {
		return contract.CommitsResponse{}, err
	}
	options, err := logOptions(request)
	if err != nil {
		return contract.CommitsResponse{}, err
	}
	response := contract.CommitsResponse{Range: contract.CommitRange(options.Range), Commits: []contract.Commit{}}
	repo, err := git.Open(project.Root)
	if errors.Is(err, git.ErrNotRepository) {
		return response, nil
	}
	if err != nil {
		return contract.CommitsResponse{}, fail(contract.KindInternal, historyFailure+err.Error())
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	history, err := repo.Log(ctx, options)
	switch {
	case errors.Is(err, git.ErrUnknownBase):
		return contract.CommitsResponse{}, unknownBase()
	case errors.Is(err, git.ErrUnavailable):
		return contract.CommitsResponse{}, fail(contract.KindNotFound, CommitUnavailableMessage)
	case err != nil:
		return contract.CommitsResponse{}, fail(contract.KindInternal, historyFailure+err.Error())
	}
	response.Repository = true
	response.Branch = repo.Branch()
	response.Base = history.Base
	response.Ahead = history.Ahead
	response.More = history.More
	response.Head = hashString(history.Head)
	for _, commit := range history.Commits {
		response.Commits = append(response.Commits, contract.Commit{
			Hash:       commit.Hash,
			Parents:    commit.Parents,
			Subject:    commit.Subject,
			Body:       commit.Body,
			Author:     contract.Author{Name: commit.Author.Name, Email: commit.Author.Email},
			AuthorTime: commit.AuthorTime.Format(authorTimeLayout),
		})
	}
	return response, nil
}

// logOptions checks a history request: range branch or all, skip of at
// least 0, limit 0 (50) or from 1 to 100, and a full head hash.
func logOptions(request contract.CommitsRequest) (git.LogOptions, error) {
	options := git.LogOptions{Range: git.RangeBranch, Skip: request.Skip, Limit: commitsDefaultLimit, Base: request.Base}
	switch request.Range {
	case "":
	case contract.RangeBranch, contract.RangeAll:
		options.Range = git.Range(request.Range)
	default:
		return options, fail(contract.KindInvalid, "Range must be branch or all.")
	}
	if request.Skip < 0 {
		return options, fail(contract.KindInvalid, "Skip must be a whole number.")
	}
	if request.Limit != 0 {
		if request.Limit < 1 || request.Limit > commitsMaxLimit {
			return options, fail(contract.KindInvalid, fmt.Sprintf("Limit must be between 1 and %d.", commitsMaxLimit))
		}
		options.Limit = request.Limit
	}
	if request.Head != "" {
		head, ok := git.ParseHash(request.Head)
		if !ok {
			return options, fail(contract.KindInvalid, InvalidCommitMessage)
		}
		options.Head = head
	}
	return options, nil
}

// CommitFiles lists a commit's changes against its first parent.
func (s *SourceControl) CommitFiles(ctx context.Context, projectID, commit string) (contract.CommitFilesResponse, error) {
	project, err := s.project(projectID)
	if err != nil {
		return contract.CommitFilesResponse{}, err
	}
	hash, ok := git.ParseHash(commit)
	if !ok {
		return contract.CommitFilesResponse{}, fail(contract.KindInvalid, InvalidCommitMessage)
	}
	repo, err := openHistory(project.Root)
	if err != nil {
		return contract.CommitFilesResponse{}, err
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	changes, err := repo.CommitFiles(ctx, hash, commitFilesLimit)
	if err != nil {
		return contract.CommitFilesResponse{}, historyError(err)
	}
	return contract.CommitFilesResponse{Commit: hash.String(), Files: changedFiles(changes.Files), Truncated: changes.Truncated}, nil
}

// BranchFiles lists the branch's net changes: the merge base with the base
// branch, request.Base or else the default, against request.Head, or else
// HEAD; with Uncommitted, HEAD's against the working tree. It is read live,
// never pinned.
func (s *SourceControl) BranchFiles(ctx context.Context, request contract.BranchFilesRequest) (contract.BranchFilesResponse, error) {
	if request.Uncommitted && request.Head != "" {
		return contract.BranchFilesResponse{}, uncommittedHead()
	}
	project, err := s.project(request.Project)
	if err != nil {
		return contract.BranchFilesResponse{}, err
	}
	response := contract.BranchFilesResponse{Files: []contract.ChangedFile{}, Warnings: []string{}}
	repo, err := git.Open(project.Root)
	if errors.Is(err, git.ErrNotRepository) {
		return response, nil
	}
	if err != nil {
		return contract.BranchFilesResponse{}, fail(contract.KindInternal, historyFailure+err.Error())
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	var compared git.BranchRange
	var changes git.CommitChanges
	if request.Uncommitted {
		var working git.BranchWorking
		var listed git.Changes
		working, listed, err = repo.BranchWorkingFiles(ctx, request.Base, git.StatusOptions{Limit: changesLimit})
		compared = working.BranchRange
		changes = git.CommitChanges{Files: listed.Files, Truncated: listed.Truncated}
		response.Warnings = warningStrings(listed.Warnings)
	} else {
		compared, changes, err = repo.BranchFiles(ctx, request.Base, request.Head, commitFilesLimit)
	}
	if err != nil {
		if failure := branchError(err); failure != nil {
			return contract.BranchFilesResponse{}, failure
		}
		return contract.BranchFilesResponse{}, fail(contract.KindInternal, historyFailure+err.Error())
	}
	response.Repository = true
	response.Branch = request.Head
	if request.Head == "" {
		response.Branch = repo.Branch()
	}
	response.Base = compared.Base
	response.MergeBase = hashString(compared.MergeBase)
	response.Head = hashString(compared.Head)
	response.Ahead = compared.Ahead
	response.Files = changedFiles(changes.Files)
	response.Truncated = changes.Truncated
	return response, nil
}

// BranchStack lists the local branches stacked with HEAD on the base
// branch, base or else the default. It is read live, never pinned.
func (s *SourceControl) BranchStack(ctx context.Context, projectID, base string) (contract.StackResponse, error) {
	project, err := s.project(projectID)
	if err != nil {
		return contract.StackResponse{}, err
	}
	response := contract.StackResponse{Layers: []contract.StackLayer{}}
	repo, err := git.Open(project.Root)
	if errors.Is(err, git.ErrNotRepository) {
		return response, nil
	}
	if err != nil {
		return contract.StackResponse{}, fail(contract.KindInternal, historyFailure+err.Error())
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	stack, err := repo.Stack(ctx, base)
	switch {
	case errors.Is(err, git.ErrUnknownBase):
		return contract.StackResponse{}, unknownBase()
	case err != nil:
		return contract.StackResponse{}, fail(contract.KindInternal, historyFailure+err.Error())
	}
	response.Repository = true
	response.Branch = stack.Branch
	response.Base = stack.Base
	response.MergeBase = hashString(stack.MergeBase)
	response.Head = hashString(stack.Head)
	response.Truncated = stack.Truncated
	for _, layer := range stack.Layers {
		aliases := layer.Aliases
		if aliases == nil {
			aliases = []string{}
		}
		response.Layers = append(response.Layers, contract.StackLayer{
			Name:    layer.Name,
			Aliases: aliases,
			Head:    layer.Head.String(),
			Parent:  layer.Parent,
			Ahead:   layer.Ahead,
			Current: layer.Current,
		})
	}
	return response, nil
}

// RangeFiles lists a commit range's net changes: from's first parent, from
// being the oldest commit, against to, the newest.
func (s *SourceControl) RangeFiles(ctx context.Context, projectID, from, to string) (contract.RangeFilesResponse, error) {
	project, err := s.project(projectID)
	if err != nil {
		return contract.RangeFilesResponse{}, err
	}
	oldest, oldestOK := git.ParseHash(from)
	newest, newestOK := git.ParseHash(to)
	if !oldestOK || !newestOK {
		return contract.RangeFilesResponse{}, fail(contract.KindInvalid, InvalidCommitMessage)
	}
	repo, err := openHistory(project.Root)
	if err != nil {
		return contract.RangeFilesResponse{}, err
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	changes, err := repo.RangeFiles(ctx, oldest, newest, commitFilesLimit)
	if err != nil {
		return contract.RangeFilesResponse{}, historyError(err)
	}
	return contract.RangeFilesResponse{From: oldest.String(), To: newest.String(), Files: changedFiles(changes.Files), Truncated: changes.Truncated}, nil
}

// Refs lists the branches a compare may use as its base, and the default
// one.
func (s *SourceControl) Refs(projectID string) (contract.RefsResponse, error) {
	project, err := s.project(projectID)
	if err != nil {
		return contract.RefsResponse{}, err
	}
	response := contract.RefsResponse{Branches: []contract.BranchRef{}}
	repo, err := git.Open(project.Root)
	if errors.Is(err, git.ErrNotRepository) {
		return response, nil
	}
	if err != nil {
		return contract.RefsResponse{}, fail(contract.KindInternal, branchesFailure+err.Error())
	}
	defer func() { _ = repo.Close() }()

	branches, err := repo.Branches(refsLimit)
	if err == nil {
		response.DefaultBase, err = repo.DefaultBase()
	}
	if err != nil {
		return contract.RefsResponse{}, fail(contract.KindInternal, branchesFailure+err.Error())
	}
	response.Repository = true
	response.Branch = repo.Branch()
	for _, branch := range branches.Branches {
		response.Branches = append(response.Branches, contract.BranchRef{Name: branch.Name, Remote: branch.Remote})
	}
	response.Truncated = branches.Truncated
	return response, nil
}

func (s *SourceControl) project(id string) (Workspace, error) {
	return ResolveProject(s.config.Roots.List(), id)
}

// openHistory opens a repository whose commits a request names: a folder
// has none.
func openHistory(root string) (*git.Repository, error) {
	repo, err := git.Open(root)
	if errors.Is(err, git.ErrNotRepository) {
		return nil, fail(contract.KindNotFound, CommitUnavailableMessage)
	}
	if err != nil {
		return nil, fail(contract.KindInternal, historyFailure+err.Error())
	}
	return repo, nil
}

// historyError is a failed commit or range read.
func historyError(err error) error {
	if errors.Is(err, git.ErrUnavailable) {
		return fail(contract.KindNotFound, CommitUnavailableMessage)
	}
	return fail(contract.KindInternal, historyFailure+err.Error())
}

// branchError is the failure for a base or head branch the repository does
// not list, or nil for any other error.
func branchError(err error) error {
	switch {
	case errors.Is(err, git.ErrUnknownBase):
		return unknownBase()
	case errors.Is(err, git.ErrUnknownHead):
		return &Error{Kind: contract.KindInvalid, Code: contract.CodeUnknownHead, Message: UnknownHeadMessage}
	}
	return nil
}

func unknownBase() *Error {
	return &Error{Kind: contract.KindInvalid, Code: contract.CodeUnknownBase, Message: UnknownBaseMessage}
}

func uncommittedHead() *Error {
	return fail(contract.KindInvalid, "Uncommitted applies to HEAD's branch only.")
}

// hashString is a full hash, or empty for zero.
func hashString(hash git.Hash) string {
	if hash.IsZero() {
		return ""
	}
	return hash.String()
}

func changedFiles(changes []git.Change) []contract.ChangedFile {
	result := make([]contract.ChangedFile, len(changes))
	for i, change := range changes {
		result[i] = contract.ChangedFile{Path: change.Path, OldPath: change.OldPath, Status: contract.ChangeStatus(change.Status)}
	}
	return result
}

// worktreeSource opens a validated change's worktree file: a symlink's
// target text, or the root-confined regular file. It returns the size and
// an opener.
func worktreeSource(root string, openFile func(string) (fs.File, error), entry git.Entry) (int64, func() (io.ReadCloser, error), error) {
	if entry.Symlink {
		link := []byte(entry.Link)
		return int64(len(link)), func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(link)), nil }, nil
	}
	resolved, err := files.ResolveFile(root, entry.Path)
	if err != nil {
		return 0, nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return 0, nil, err
	}
	return info.Size(), func() (io.ReadCloser, error) { return openFile(resolved) }, nil
}

// lineDiff is the contract form of a line diff.
func lineDiff(result diff.Result, ignoreWhitespace bool) *contract.LineDiff {
	converted := &contract.LineDiff{
		IgnoreWhitespace: ignoreWhitespace,
		Additions:        result.Additions,
		Deletions:        result.Deletions,
		Hunks:            make([]contract.DiffHunk, len(result.Hunks)),
		Gaps:             make([]contract.DiffGap, len(result.Gaps)),
	}
	for i, hunk := range result.Hunks {
		blocks := make([]contract.DiffBlock, len(hunk.Blocks))
		for j, block := range hunk.Blocks {
			kind := contract.DiffBlockContext
			if block.Change {
				kind = contract.DiffBlockChange
			}
			blocks[j] = contract.DiffBlock{Kind: kind, OldStart: block.OldStart, OldCount: block.OldCount, NewStart: block.NewStart, NewCount: block.NewCount}
		}
		converted.Hunks[i] = contract.DiffHunk{
			OldStart: hunk.OldStart, OldCount: hunk.OldCount,
			NewStart: hunk.NewStart, NewCount: hunk.NewCount,
			Additions: hunk.Additions, Deletions: hunk.Deletions,
			Blocks: blocks,
		}
	}
	for i, gap := range result.Gaps {
		converted.Gaps[i] = contract.DiffGap{Hunk: gap.Hunk, OldStart: gap.OldStart, NewStart: gap.NewStart, Lines: gap.Lines}
	}
	return converted
}

// documentTypeOf is the contract type of a path's family.
func (s *state) documentTypeOf(name string) (document.Type, contract.FileType) {
	fileType := s.config.Types.Lookup(name).Type()
	return fileType, fileTypeContract(fileType)
}
