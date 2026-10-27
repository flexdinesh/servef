package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/git"
)

const (
	// commitsDefaultLimit and commitsMaxLimit bound one history page.
	commitsDefaultLimit = 50
	commitsMaxLimit     = 100
	// commitFilesLimit caps the changes one commit response lists.
	commitFilesLimit = 1000
	// authorTimeLayout is RFC 3339 with a numeric offset, never Z, so the
	// author's own zone shows.
	authorTimeLayout = "2006-01-02T15:04:05-07:00"

	commitUnavailableMessage = "That commit is unavailable."
	invalidCommitMessage     = "That commit hash is invalid."
	// commitFileUnavailableMessage reports an unknown commit, or a path the
	// commit does not have.
	commitFileUnavailableMessage = "That file is unavailable at that commit."
	unknownBaseMessage           = "That base branch doesn't exist."
	// unknownBaseCode marks a 400 for a base branch /api/refs does not list.
	unknownBaseCode    = "unknownBase"
	unknownHeadMessage = "That head branch doesn't exist."
	// unknownHeadCode marks a 400 for a head that is not a local branch
	// /api/refs lists.
	unknownHeadCode = "unknownHead"
)

// writeUnknownBase writes the failure for a base branch the repository does
// not list and reports whether err was one.
func writeUnknownBase(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, git.ErrUnknownBase) {
		return false
	}
	writeJSON(w, http.StatusBadRequest, errorResponse{Error: unknownBaseMessage, Code: unknownBaseCode})
	return true
}

// writeUnknownHead writes the failure for a head branch the repository does
// not have locally and reports whether err was one.
func writeUnknownHead(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, git.ErrUnknownHead) {
		return false
	}
	writeJSON(w, http.StatusBadRequest, errorResponse{Error: unknownHeadMessage, Code: unknownHeadCode})
	return true
}

// History, like changes, is repository-scoped: commits and their files are
// listed whatever the CLI filename scope, and file sides are read from git
// objects only after the git package validates them as changes.

type commitsResponse struct {
	Repository bool             `json:"repository"`
	Branch     string           `json:"branch"`
	Base       string           `json:"base"`
	Range      git.Range        `json:"range"`
	Head       string           `json:"head"`
	Ahead      int              `json:"ahead"`
	Commits    []commitResponse `json:"commits"`
	More       bool             `json:"more"`
}

type commitResponse struct {
	Hash       string     `json:"hash"`
	Parents    []string   `json:"parents"`
	Subject    string     `json:"subject"`
	Body       string     `json:"body"`
	Author     git.Author `json:"author"`
	AuthorTime string     `json:"authorTime"`
}

type commitFilesResponse struct {
	Commit    string       `json:"commit"`
	Files     []git.Change `json:"files"`
	Truncated bool         `json:"truncated"`
}

func (a *App) serveCommits(w http.ResponseWriter, r *http.Request, projectID string, query url.Values) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	options, message := logOptions(query)
	if message != "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: message})
		return
	}
	response := commitsResponse{Range: options.Range, Commits: []commitResponse{}}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		writeJSON(w, http.StatusOK, response)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	history, err := repo.Log(ctx, options)
	switch {
	case writeUnknownBase(w, err):
		return
	case errors.Is(err, git.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: commitUnavailableMessage})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	response.Repository = true
	response.Branch = repo.Branch()
	response.Base = history.Base
	response.Ahead = history.Ahead
	response.More = history.More
	if !history.Head.IsZero() {
		response.Head = history.Head.String()
	}
	for _, commit := range history.Commits {
		response.Commits = append(response.Commits, commitResponse{
			Hash:       commit.Hash,
			Parents:    commit.Parents,
			Subject:    commit.Subject,
			Body:       commit.Body,
			Author:     commit.Author,
			AuthorTime: commit.AuthorTime.Format(authorTimeLayout),
		})
	}
	writeJSON(w, http.StatusOK, response)
}

// logOptions parses a history query: range defaults to branch, skip to 0,
// limit to 50 of at most 100, head to HEAD, and base to the default base
// branch. It returns a message for invalid input.
func logOptions(query url.Values) (git.LogOptions, string) {
	options := git.LogOptions{Range: git.RangeBranch, Limit: commitsDefaultLimit, Base: query.Get("base")}
	if value := query.Get("range"); value != "" {
		options.Range = git.Range(value)
		if options.Range != git.RangeBranch && options.Range != git.RangeAll {
			return options, "Range must be branch or all."
		}
	}
	if value := query.Get("skip"); value != "" {
		skip, err := strconv.Atoi(value)
		if err != nil || skip < 0 {
			return options, "Skip must be a whole number."
		}
		options.Skip = skip
	}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > commitsMaxLimit {
			return options, fmt.Sprintf("Limit must be between 1 and %d.", commitsMaxLimit)
		}
		options.Limit = limit
	}
	if value := query.Get("head"); value != "" {
		head, ok := git.ParseHash(value)
		if !ok {
			return options, invalidCommitMessage
		}
		options.Head = head
	}
	return options, ""
}

func (a *App) serveCommitFiles(w http.ResponseWriter, r *http.Request, projectID, commit string) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	hash, ok := git.ParseHash(commit)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: invalidCommitMessage})
		return
	}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: commitUnavailableMessage})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	changes, err := repo.CommitFiles(ctx, hash, commitFilesLimit)
	switch {
	case errors.Is(err, git.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: commitUnavailableMessage})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, commitFilesResponse{Commit: hash.String(), Files: changes.Files, Truncated: changes.Truncated})
}

// serveDiffs serves the diff a query names: a commit's (commit), the
// branch's (branch=1), a commit range's (from and to), or else the working
// tree's. The kinds are exclusive.
func (a *App) serveDiffs(w http.ResponseWriter, r *http.Request, projectID string, query url.Values) {
	commit, branch, ranged := query.Has("commit"), query.Has("branch"), query.Has("from") || query.Has("to")
	kinds := 0
	for _, named := range []bool{commit, branch, ranged} {
		if named {
			kinds++
		}
	}
	name, oldPath := query.Get("path"), query.Get("oldPath")
	switch {
	case kinds > 1:
		writeDiffQueryError(w, "Choose one of commit, branch, or from and to.")
	case query.Has("base") && !branch:
		writeDiffQueryError(w, "Base applies to branch diffs only.")
	case query.Has("head") && !branch:
		writeDiffQueryError(w, "Head applies to branch diffs only.")
	case commit:
		a.serveCommitDiff(w, r, projectID, query.Get("commit"), name, oldPath)
	case branch && query.Get("branch") != "1":
		writeDiffQueryError(w, "Branch must be 1.")
	case branch:
		a.serveBranchDiff(w, r, projectID, query.Get("base"), query.Get("head"), name, oldPath)
	case ranged && (!query.Has("from") || !query.Has("to")):
		writeDiffQueryError(w, "A commit range needs both from and to.")
	case ranged:
		a.serveRangeDiff(w, r, projectID, query.Get("from"), query.Get("to"), name, oldPath)
	default:
		a.serveDiff(w, r, projectID, name, oldPath)
	}
}

func writeDiffQueryError(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, errorResponse{Error: message})
}

type branchFilesResponse struct {
	Repository bool         `json:"repository"`
	Branch     string       `json:"branch"`
	Base       string       `json:"base"`
	MergeBase  string       `json:"mergeBase"`
	Head       string       `json:"head"`
	Ahead      int          `json:"ahead"`
	Files      []git.Change `json:"files"`
	Truncated  bool         `json:"truncated"`
}

type rangeFilesResponse struct {
	From      string       `json:"from"`
	To        string       `json:"to"`
	Files     []git.Change `json:"files"`
	Truncated bool         `json:"truncated"`
}

// serveBranchFiles lists the branch's net changes: the merge base with the
// base branch, base or else the default, against the head branch, or else
// HEAD. It is read live, never pinned.
func (a *App) serveBranchFiles(w http.ResponseWriter, r *http.Request, projectID, base, head string) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	response := branchFilesResponse{Files: []git.Change{}}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		writeJSON(w, http.StatusOK, response)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	compared, changes, err := repo.BranchFiles(ctx, base, head, commitFilesLimit)
	switch {
	case writeUnknownBase(w, err), writeUnknownHead(w, err):
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	response.Repository = true
	response.Branch = head
	if head == "" {
		response.Branch = repo.Branch()
	}
	response.Base = compared.Base
	response.MergeBase = hashString(compared.MergeBase)
	response.Head = hashString(compared.Head)
	response.Ahead = compared.Ahead
	response.Files = changes.Files
	response.Truncated = changes.Truncated
	writeJSON(w, http.StatusOK, response)
}

// hashString is a full hash, or empty for zero.
func hashString(hash git.Hash) string {
	if hash.IsZero() {
		return ""
	}
	return hash.String()
}

type branchStackResponse struct {
	Repository bool                 `json:"repository"`
	Branch     string               `json:"branch"`
	Base       string               `json:"base"`
	MergeBase  string               `json:"mergeBase"`
	Head       string               `json:"head"`
	Layers     []stackLayerResponse `json:"layers"`
	Truncated  bool                 `json:"truncated"`
}

type stackLayerResponse struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Head    string   `json:"head"`
	Parent  string   `json:"parent"`
	Ahead   int      `json:"ahead"`
	Current bool     `json:"current"`
}

// serveBranchStack lists the local branches stacked with HEAD on the base
// branch, base or else the default. It is read live, never pinned.
func (a *App) serveBranchStack(w http.ResponseWriter, r *http.Request, projectID, base string) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	response := branchStackResponse{Layers: []stackLayerResponse{}}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		writeJSON(w, http.StatusOK, response)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	stack, err := repo.Stack(ctx, base)
	switch {
	case writeUnknownBase(w, err):
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
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
		response.Layers = append(response.Layers, stackLayerResponse{
			Name:    layer.Name,
			Aliases: aliases,
			Head:    layer.Head.String(),
			Parent:  layer.Parent,
			Ahead:   layer.Ahead,
			Current: layer.Current,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

// serveRangeFiles lists a commit range's net changes: from's first parent,
// from being the oldest commit, against to, the newest.
func (a *App) serveRangeFiles(w http.ResponseWriter, r *http.Request, projectID, from, to string) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	oldest, oldestOK := git.ParseHash(from)
	newest, newestOK := git.ParseHash(to)
	if !oldestOK || !newestOK {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: invalidCommitMessage})
		return
	}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: commitUnavailableMessage})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	changes, err := repo.RangeFiles(ctx, oldest, newest, commitFilesLimit)
	switch {
	case errors.Is(err, git.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: commitUnavailableMessage})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rangeFilesResponse{From: oldest.String(), To: newest.String(), Files: changes.Files, Truncated: changes.Truncated})
}

// openHistoryDiff validates a history diff request in order (project, path,
// then each commit hash) and opens the repository. It writes the failure
// and returns false when the request cannot proceed.
func (a *App) openHistoryDiff(w http.ResponseWriter, projectID, name string, commits ...string) (*git.Repository, []git.Hash, bool) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return nil, nil, false
	}
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "No file was selected."})
		return nil, nil, false
	}
	hashes := make([]git.Hash, 0, len(commits))
	for _, commit := range commits {
		hash, ok := git.ParseHash(commit)
		if !ok {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: invalidCommitMessage})
			return nil, nil, false
		}
		hashes = append(hashes, hash)
	}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: changeUnavailableMessage})
		return nil, nil, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return nil, nil, false
	}
	return repo, hashes, true
}

// serveCommitDiff pairs a commit's first parent's side (Original) with the
// commit's (Modified) for one validated change. Without oldPath, a renamed
// path reports its rename, since commit diff tabs remember only the path.
func (a *App) serveCommitDiff(w http.ResponseWriter, r *http.Request, projectID, commit, name, oldPath string) {
	repo, hashes, ok := a.openHistoryDiff(w, projectID, name, commit)
	if !ok {
		return
	}
	defer func() { _ = repo.Close() }()
	hash := hashes[0]

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	change, err := repo.CommitLookup(ctx, hash, name, oldPath)
	if !writeLookupError(w, err) {
		return
	}
	short := hash.String()[:7]
	a.writeHistoryDiff(w, change,
		historySide{"the parent of " + short, func(name string) (git.Blob, error) { return repo.ParentBlob(hash, name) }},
		historySide{short, func(name string) (git.Blob, error) { return repo.CommitBlob(hash, name) }})
}

// serveRangeDiff pairs the first parent of from, the range's oldest commit
// (Original), with to, its newest (Modified), for one validated change.
func (a *App) serveRangeDiff(w http.ResponseWriter, r *http.Request, projectID, from, to, name, oldPath string) {
	repo, hashes, ok := a.openHistoryDiff(w, projectID, name, from, to)
	if !ok {
		return
	}
	defer func() { _ = repo.Close() }()
	oldest, newest := hashes[0], hashes[1]

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	change, err := repo.RangeLookup(ctx, oldest, newest, name, oldPath)
	if !writeLookupError(w, err) {
		return
	}
	a.writeHistoryDiff(w, change,
		historySide{"the parent of " + oldest.String()[:7], func(name string) (git.Blob, error) { return repo.ParentBlob(oldest, name) }},
		historySide{newest.String()[:7], func(name string) (git.Blob, error) { return repo.CommitBlob(newest, name) }})
}

// serveBranchDiff pairs the merge base (Original) with the head branch, or
// else HEAD, (Modified) for one change of the branch against base, or else
// the default base branch, resolved again on every request. A path outside
// the branch's changes, including when nothing is ahead, is unchanged, as in
// the working tree diff.
func (a *App) serveBranchDiff(w http.ResponseWriter, r *http.Request, projectID, base, head, name, oldPath string) {
	repo, _, ok := a.openHistoryDiff(w, projectID, name)
	if !ok {
		return
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	compared, change, err := repo.BranchLookup(ctx, base, head, name, oldPath)
	switch {
	case writeUnknownBase(w, err), writeUnknownHead(w, err):
		return
	case errors.Is(err, git.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: fmt.Sprintf("No changes to %s on this branch.", name), Code: unchangedCode})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	a.writeHistoryDiff(w, change,
		historySide{"the merge base " + compared.MergeBase.String()[:7], func(name string) (git.Blob, error) { return repo.CommitBlob(compared.MergeBase, name) }},
		historySide{compared.Head.String()[:7], func(name string) (git.Blob, error) { return repo.CommitBlob(compared.Head, name) }})
}

// writeLookupError writes a failed history lookup and reports whether err
// was nil.
func writeLookupError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, git.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: changeUnavailableMessage})
		return false
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return false
	}
	return true
}

// historySide reads one diff side from git objects; label names it in
// errors.
type historySide struct {
	label string
	blob  func(name string) (git.Blob, error)
}

// writeHistoryDiff reads a validated change's sides, Original from the
// rename's old path when there is one, under the 0004 limits.
func (a *App) writeHistoryDiff(w http.ResponseWriter, change git.Change, original, modified historySide) {
	fileType := a.config.Types.Lookup(change.Path).Type()
	response := diffResponse{Path: change.Path, OldPath: change.OldPath, Status: change.Status, DocumentType: fileType}
	var err error
	if change.Status != git.Added {
		name := change.Path
		if change.OldPath != "" {
			name = change.OldPath
		}
		response.Original, err = readCommitSide(original.blob, name, fileType)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("Could not read %s from %s: %v", name, original.label, err)})
			return
		}
	}
	if change.Status != git.Deleted {
		response.Modified, err = readCommitSide(modified.blob, change.Path, fileType)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("Could not read %s from %s: %v", change.Path, modified.label, err)})
			return
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func readCommitSide(blob func(string) (git.Blob, error), name string, fileType document.Type) (*diffSide, error) {
	side, err := blob(name)
	if err != nil {
		return nil, err
	}
	return readDiffSide(side.Size, side.Open, fileType)
}
