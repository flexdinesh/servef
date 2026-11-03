package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/flexdinesh/servef/internal/diff"
	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/git"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Diff reads one changed file's sides under the 0004 limits and, when both
// are text, their line diff (decision 0083): the working tree's against
// HEAD, a commit's against its first parent, a branch's against its merge
// base (with Uncommitted, the working tree against HEAD's branch's base),
// or a commit range's. A path the compare does not change is not found
// with CodeUnchanged where the compare is live.
func (s *SourceControl) Diff(ctx context.Context, request contract.DiffRequest) (contract.DiffResponse, error) {
	if request.Kind != contract.DiffBranch {
		switch {
		case request.Base != "":
			return contract.DiffResponse{}, fail(contract.KindInvalid, "Base applies to branch diffs only.")
		case request.Head != "":
			return contract.DiffResponse{}, fail(contract.KindInvalid, "Head applies to branch diffs only.")
		case request.Uncommitted:
			return contract.DiffResponse{}, fail(contract.KindInvalid, "Uncommitted applies to branch diffs only.")
		}
	}
	switch request.Kind {
	case contract.DiffWorking:
		return s.workingDiff(ctx, request)
	case contract.DiffCommit:
		return s.commitDiff(ctx, request)
	case contract.DiffBranch:
		if request.Uncommitted {
			if request.Head != "" {
				return contract.DiffResponse{}, uncommittedHead()
			}
			return s.branchWorkingDiff(ctx, request)
		}
		return s.branchDiff(ctx, request)
	case contract.DiffRange:
		return s.rangeDiff(ctx, request)
	}
	return contract.DiffResponse{}, fail(contract.KindInvalid, "Choose one of commit, branch, or from and to.")
}

// openDiff validates a diff request in order (project, path, then each
// commit hash) and opens the repository; failure names the read in errors.
func (s *SourceControl) openDiff(request contract.DiffRequest, failure string, commits ...string) (Workspace, *git.Repository, []git.Hash, error) {
	project, err := s.project(request.Project)
	if err != nil {
		return Workspace{}, nil, nil, err
	}
	if request.Path == "" {
		return Workspace{}, nil, nil, fail(contract.KindInvalid, NoFileMessage)
	}
	hashes := make([]git.Hash, 0, len(commits))
	for _, commit := range commits {
		hash, ok := git.ParseHash(commit)
		if !ok {
			return Workspace{}, nil, nil, fail(contract.KindInvalid, InvalidCommitMessage)
		}
		hashes = append(hashes, hash)
	}
	repo, err := git.Open(project.Root)
	if errors.Is(err, git.ErrNotRepository) {
		return Workspace{}, nil, nil, fail(contract.KindNotFound, ChangeUnavailableMessage)
	}
	if err != nil {
		return Workspace{}, nil, nil, fail(contract.KindInternal, failure+err.Error())
	}
	return project, repo, hashes, nil
}

func (s *SourceControl) workingDiff(ctx context.Context, request contract.DiffRequest) (contract.DiffResponse, error) {
	project, repo, _, err := s.openDiff(request, statusFailure)
	if err != nil {
		return contract.DiffResponse{}, err
	}
	defer func() { _ = repo.Close() }()

	entry, err := repo.Lookup(ctx, request.Path, request.OldPath)
	switch {
	case errors.Is(err, git.ErrUnchanged):
		return contract.DiffResponse{}, &Error{Kind: contract.KindNotFound, Code: contract.CodeUnchanged, Message: fmt.Sprintf("No changes to %s since the last commit.", request.Path)}
	case errors.Is(err, git.ErrUnavailable):
		return contract.DiffResponse{}, fail(contract.KindNotFound, ChangeUnavailableMessage)
	case err != nil:
		return contract.DiffResponse{}, fail(contract.KindInternal, statusFailure+err.Error())
	}
	return s.worktreeDiff(project.Root, entry, "the last commit", repo.HeadBlob, request.IgnoreWhitespace)
}

// branchWorkingDiff pairs Reference, the merge base when HEAD is ahead of
// base, or else the default base branch (Original), with the worktree
// (Modified) for one change of HEAD's branch with uncommitted changes,
// resolved again on every request. A path outside its changes is
// unchanged, as in the branch diff.
func (s *SourceControl) branchWorkingDiff(ctx context.Context, request contract.DiffRequest) (contract.DiffResponse, error) {
	project, repo, _, err := s.openDiff(request, statusFailure)
	if err != nil {
		return contract.DiffResponse{}, err
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	compared, entry, err := repo.BranchWorkingLookup(ctx, request.Base, request.Path, request.OldPath)
	switch {
	case errors.Is(err, git.ErrUnknownBase):
		return contract.DiffResponse{}, unknownBase()
	case errors.Is(err, git.ErrUnchanged), errors.Is(err, git.ErrUnavailable):
		return contract.DiffResponse{}, unchangedOnBranch(request.Path)
	case err != nil:
		return contract.DiffResponse{}, fail(contract.KindInternal, statusFailure+err.Error())
	}
	label := "the last commit"
	if compared.Reference != compared.Head {
		label = "the merge base " + compared.Reference.String()[:7]
	}
	return s.worktreeDiff(project.Root, entry, label, func(name string) (git.Blob, error) { return repo.CommitBlob(compared.Reference, name) }, request.IgnoreWhitespace)
}

// commitDiff pairs a commit's first parent's side (Original) with the
// commit's (Modified) for one validated change. Without OldPath, a renamed
// path reports its rename, since commit diff tabs remember only the path.
func (s *SourceControl) commitDiff(ctx context.Context, request contract.DiffRequest) (contract.DiffResponse, error) {
	_, repo, hashes, err := s.openDiff(request, historyFailure, request.Commit)
	if err != nil {
		return contract.DiffResponse{}, err
	}
	defer func() { _ = repo.Close() }()
	hash := hashes[0]

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	change, err := repo.CommitLookup(ctx, hash, request.Path, request.OldPath)
	if err != nil {
		return contract.DiffResponse{}, lookupError(err)
	}
	short := hash.String()[:7]
	return s.historyDiff(change,
		historySide{"the parent of " + short, func(name string) (git.Blob, error) { return repo.ParentBlob(hash, name) }},
		historySide{short, func(name string) (git.Blob, error) { return repo.CommitBlob(hash, name) }},
		request.IgnoreWhitespace)
}

// rangeDiff pairs the first parent of From, the range's oldest commit
// (Original), with To, its newest (Modified), for one validated change.
func (s *SourceControl) rangeDiff(ctx context.Context, request contract.DiffRequest) (contract.DiffResponse, error) {
	_, repo, hashes, err := s.openDiff(request, historyFailure, request.From, request.To)
	if err != nil {
		return contract.DiffResponse{}, err
	}
	defer func() { _ = repo.Close() }()
	oldest, newest := hashes[0], hashes[1]

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	change, err := repo.RangeLookup(ctx, oldest, newest, request.Path, request.OldPath)
	if err != nil {
		return contract.DiffResponse{}, lookupError(err)
	}
	return s.historyDiff(change,
		historySide{"the parent of " + oldest.String()[:7], func(name string) (git.Blob, error) { return repo.ParentBlob(oldest, name) }},
		historySide{newest.String()[:7], func(name string) (git.Blob, error) { return repo.CommitBlob(newest, name) }},
		request.IgnoreWhitespace)
}

// branchDiff pairs the merge base (Original) with the head branch, or else
// HEAD, (Modified) for one change of the branch against base, or else the
// default base branch, resolved again on every request. A path outside the
// branch's changes, including when nothing is ahead, is unchanged, as in
// the working tree diff.
func (s *SourceControl) branchDiff(ctx context.Context, request contract.DiffRequest) (contract.DiffResponse, error) {
	_, repo, _, err := s.openDiff(request, historyFailure)
	if err != nil {
		return contract.DiffResponse{}, err
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	compared, change, err := repo.BranchLookup(ctx, request.Base, request.Head, request.Path, request.OldPath)
	if err != nil {
		if failure := branchError(err); failure != nil {
			return contract.DiffResponse{}, failure
		}
		if errors.Is(err, git.ErrUnavailable) {
			return contract.DiffResponse{}, unchangedOnBranch(request.Path)
		}
		return contract.DiffResponse{}, fail(contract.KindInternal, historyFailure+err.Error())
	}
	return s.historyDiff(change,
		historySide{"the merge base " + compared.MergeBase.String()[:7], func(name string) (git.Blob, error) { return repo.CommitBlob(compared.MergeBase, name) }},
		historySide{compared.Head.String()[:7], func(name string) (git.Blob, error) { return repo.CommitBlob(compared.Head, name) }},
		request.IgnoreWhitespace)
}

func unchangedOnBranch(name string) *Error {
	return &Error{Kind: contract.KindNotFound, Code: contract.CodeUnchanged, Message: fmt.Sprintf("No changes to %s on this branch.", name)}
}

// lookupError is a failed history lookup.
func lookupError(err error) error {
	if errors.Is(err, git.ErrUnavailable) {
		return fail(contract.KindNotFound, ChangeUnavailableMessage)
	}
	return fail(contract.KindInternal, historyFailure+err.Error())
}

// sideText is a read diff side and its text, nil unless the side is text
// within the limits.
type sideText struct {
	side *contract.DiffSide
	text []byte
}

// worktreeDiff reads a validated change's sides under the 0004 limits:
// Original through blob, from the rename's old path when there is one, and
// Modified from the worktree. label names Original's commit in errors.
func (s *SourceControl) worktreeDiff(root string, entry git.Entry, label string, blob func(name string) (git.Blob, error), ignoreWhitespace bool) (contract.DiffResponse, error) {
	fileType, documentType := s.documentTypeOf(entry.Path)
	var original, modified *sideText
	if entry.Status != git.Added && entry.Status != git.Untracked {
		name := entry.Path
		if entry.OldPath != "" {
			name = entry.OldPath
		}
		side, err := blob(name)
		if err == nil {
			original, err = readDiffSide(side.Size, side.Open, fileType)
		}
		if err != nil {
			return contract.DiffResponse{}, fail(contract.KindInternal, fmt.Sprintf("Could not read %s from %s: %v", name, label, err))
		}
	}
	if entry.Status != git.Deleted {
		size, open, err := worktreeSource(root, s.config.OpenFile, entry)
		if err == nil {
			modified, err = readDiffSide(size, open, fileType)
		}
		if err != nil {
			return contract.DiffResponse{}, fail(contract.KindInternal, fmt.Sprintf("Could not read %s: %v", entry.Path, err))
		}
	}
	return diffResponse(entry.Change, documentType, original, modified, ignoreWhitespace), nil
}

// historySide reads one diff side from git objects; label names it in
// errors.
type historySide struct {
	label string
	blob  func(name string) (git.Blob, error)
}

// historyDiff reads a validated change's sides, Original from the rename's
// old path when there is one, under the 0004 limits.
func (s *SourceControl) historyDiff(change git.Change, originalSide, modifiedSide historySide, ignoreWhitespace bool) (contract.DiffResponse, error) {
	fileType, documentType := s.documentTypeOf(change.Path)
	var original, modified *sideText
	if change.Status != git.Added {
		name := change.Path
		if change.OldPath != "" {
			name = change.OldPath
		}
		side, err := originalSide.blob(name)
		if err == nil {
			original, err = readDiffSide(side.Size, side.Open, fileType)
		}
		if err != nil {
			return contract.DiffResponse{}, fail(contract.KindInternal, fmt.Sprintf("Could not read %s from %s: %v", name, originalSide.label, err))
		}
	}
	if change.Status != git.Deleted {
		side, err := modifiedSide.blob(change.Path)
		if err == nil {
			modified, err = readDiffSide(side.Size, side.Open, fileType)
		}
		if err != nil {
			return contract.DiffResponse{}, fail(contract.KindInternal, fmt.Sprintf("Could not read %s from %s: %v", change.Path, modifiedSide.label, err))
		}
	}
	return diffResponse(change, documentType, original, modified, ignoreWhitespace), nil
}

// diffResponse pairs the sides and, when every present side is text,
// diffs them; an absent side is empty.
func diffResponse(change git.Change, documentType contract.FileType, original, modified *sideText, ignoreWhitespace bool) contract.DiffResponse {
	response := contract.DiffResponse{Path: change.Path, OldPath: change.OldPath, Status: contract.ChangeStatus(change.Status), DocumentType: documentType}
	text := true
	var oldText, newText []byte
	if original != nil {
		response.Original = original.side
		oldText = original.text
		text = text && original.side.Content != nil
	}
	if modified != nil {
		response.Modified = modified.side
		newText = modified.text
		text = text && modified.side.Content != nil
	}
	if text {
		response.LineDiff = lineDiff(diff.Lines(oldText, newText, diff.Options{IgnoreWhitespace: ignoreWhitespace}), ignoreWhitespace)
	}
	return response
}

// readDiffSide applies the 0004 limits to one side of size bytes. Sides over
// DisplayLimit and types without a text source are never opened; binary and
// over-PreviewLimit sides read only the sniffed prefix.
func readDiffSide(size int64, open func() (io.ReadCloser, error), fileType document.Type) (*sideText, error) {
	side := &contract.DiffSide{Size: size, TooLarge: size > document.PreviewLimit}
	if !fileType.HasTextSource() {
		side.Binary = true
		return &sideText{side: side}, nil
	}
	if size > document.DisplayLimit {
		return &sideText{side: side}, nil
	}
	reader, err := open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	// Too-large sides only need the prefix to report binary.
	limit := int64(document.PreviewLimit)
	if side.TooLarge {
		limit = document.SniffLength
	}
	text, binary, err := document.ReadText(reader, limit)
	if err != nil {
		return nil, err
	}
	if binary {
		side.Binary = true
		return &sideText{side: side}, nil
	}
	if side.TooLarge {
		return &sideText{side: side}, nil
	}
	content := string(text)
	side.Content = &content
	return &sideText{side: side, text: text}, nil
}
