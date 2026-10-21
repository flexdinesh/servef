package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-git/go-git/v5/plumbing"

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
)

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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
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
// limit to 50 of at most 100, and head to HEAD. It returns a message for
// invalid input.
func logOptions(query url.Values) (git.LogOptions, string) {
	options := git.LogOptions{Range: git.RangeBranch, Limit: commitsDefaultLimit}
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
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

// serveCommitDiff pairs a commit's first parent's side (Original) with the
// commit's (Modified) for one validated change. Without oldPath, a renamed
// path reports its rename, since commit diff tabs remember only the path.
func (a *App) serveCommitDiff(w http.ResponseWriter, r *http.Request, projectID, commit, name, oldPath string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "No file was selected."})
		return
	}
	hash, ok := git.ParseHash(commit)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: invalidCommitMessage})
		return
	}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: changeUnavailableMessage})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	change, err := repo.CommitLookup(ctx, hash, name, oldPath)
	switch {
	case errors.Is(err, git.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: changeUnavailableMessage})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git history: " + err.Error()})
		return
	}

	fileType := a.config.Types.Lookup(change.Path).Type()
	response := diffResponse{Path: change.Path, OldPath: change.OldPath, Status: change.Status, DocumentType: fileType}
	short := hash.String()[:7]
	if change.Status != git.Added {
		original := change.Path
		if change.OldPath != "" {
			original = change.OldPath
		}
		response.Original, err = readCommitSide(repo.ParentBlob, hash, original, fileType)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("Could not read %s from the parent of %s: %v", original, short, err)})
			return
		}
	}
	if change.Status != git.Deleted {
		response.Modified, err = readCommitSide(repo.CommitBlob, hash, change.Path, fileType)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("Could not read %s from %s: %v", change.Path, short, err)})
			return
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func readCommitSide(blob func(plumbing.Hash, string) (git.Blob, error), hash plumbing.Hash, name string, fileType document.Type) (*diffSide, error) {
	side, err := blob(hash, name)
	if err != nil {
		return nil, err
	}
	return readDiffSide(side.Size, side.Open, fileType)
}
