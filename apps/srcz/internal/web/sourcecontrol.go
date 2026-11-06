package web

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/flexdinesh/srcz/pkg/contract"
)

// The Source Control endpoints adapt internal/app's SourceControl to HTTP.
// Queries name optional values by presence; this adapter checks what a
// typed request cannot carry (flags whose only value is 1, mutually
// exclusive parameters, and parameters a compare does not take) in the
// order the endpoints have always checked them.

func (a *App) serveChanges(w http.ResponseWriter, r *http.Request, projectID string) {
	response, err := a.app.SourceControl.Changes(r.Context(), projectID)
	writeResult(w, response, err)
}

// serveDiffs serves the diff a query names: a commit's (commit), the
// branch's (branch=1), a commit range's (from and to), or else the working
// tree's. The kinds are exclusive; ignoreWhitespace=1 compares lines
// without their leading and trailing whitespace.
func (a *App) serveDiffs(w http.ResponseWriter, r *http.Request, query url.Values) {
	commit, branch, ranged := query.Has("commit"), query.Has("branch"), query.Has("from") || query.Has("to")
	kinds := 0
	for _, named := range []bool{commit, branch, ranged} {
		if named {
			kinds++
		}
	}
	request := contract.DiffRequest{
		Project: query.Get("project"), Path: query.Get("path"), OldPath: query.Get("oldPath"),
		Commit: query.Get("commit"), Base: query.Get("base"), Head: query.Get("head"),
		From: query.Get("from"), To: query.Get("to"),
	}
	switch {
	case kinds > 1:
		writeQueryError(w, "Choose one of commit, branch, or from and to.")
		return
	case query.Has("base") && !branch:
		writeQueryError(w, "Base applies to branch diffs only.")
		return
	case query.Has("head") && !branch:
		writeQueryError(w, "Head applies to branch diffs only.")
		return
	case query.Has("uncommitted") && !branch:
		writeQueryError(w, "Uncommitted applies to branch diffs only.")
		return
	case commit:
		request.Kind = contract.DiffCommit
	case branch && query.Get("branch") != "1":
		writeQueryError(w, "Branch must be 1.")
		return
	case branch:
		request.Kind = contract.DiffBranch
		uncommitted, message := uncommittedQuery(query)
		if message != "" {
			writeQueryError(w, message)
			return
		}
		request.Uncommitted = uncommitted
	case ranged && (!query.Has("from") || !query.Has("to")):
		writeQueryError(w, "A commit range needs both from and to.")
		return
	case ranged:
		request.Kind = contract.DiffRange
	}
	if query.Has("ignoreWhitespace") {
		if query.Get("ignoreWhitespace") != "1" {
			writeQueryError(w, "IgnoreWhitespace must be 1.")
			return
		}
		request.IgnoreWhitespace = true
	}
	response, err := a.app.SourceControl.Diff(r.Context(), request)
	writeResult(w, response, err)
}

// uncommittedQuery reads uncommitted=1, which adds the working tree to
// HEAD's branch compare (decision 0051). It returns a message for invalid
// input: another value, or a head branch, whose worktree is not checked out.
func uncommittedQuery(query url.Values) (bool, string) {
	switch {
	case !query.Has("uncommitted"):
		return false, ""
	case query.Get("uncommitted") != "1":
		return false, "Uncommitted must be 1."
	case query.Get("head") != "":
		return false, "Uncommitted applies to HEAD's branch only."
	}
	return true, ""
}

// serveCommits pages history: range defaults to branch, skip to 0, limit to
// 50 of at most 100, head to HEAD, and base to the default base branch.
func (a *App) serveCommits(w http.ResponseWriter, r *http.Request, query url.Values) {
	request := contract.CommitsRequest{
		Project: query.Get("project"), Range: contract.CommitRange(query.Get("range")),
		Head: query.Get("head"), Base: query.Get("base"),
	}
	// Values that are not whole numbers, or a limit of 0, are as invalid as
	// those out of range; -1 is out of range for both.
	if query.Get("skip") != "" {
		skip, err := strconv.Atoi(query.Get("skip"))
		if err != nil {
			skip = -1
		}
		request.Skip = skip
	}
	if query.Get("limit") != "" {
		limit, err := strconv.Atoi(query.Get("limit"))
		if err != nil || limit == 0 {
			limit = -1
		}
		request.Limit = limit
	}
	response, err := a.app.SourceControl.Commits(r.Context(), request)
	writeResult(w, response, err)
}

func (a *App) serveCommitFiles(w http.ResponseWriter, r *http.Request, query url.Values) {
	response, err := a.app.SourceControl.CommitFiles(r.Context(), query.Get("project"), query.Get("commit"))
	writeResult(w, response, err)
}

// serveBranchFiles lists the branch's net changes; uncommitted=1 adds the
// working tree to HEAD's branch.
func (a *App) serveBranchFiles(w http.ResponseWriter, r *http.Request, query url.Values) {
	uncommitted, message := uncommittedQuery(query)
	if message != "" {
		writeQueryError(w, message)
		return
	}
	response, err := a.app.SourceControl.BranchFiles(r.Context(), contract.BranchFilesRequest{
		Project: query.Get("project"), Base: query.Get("base"), Head: query.Get("head"), Uncommitted: uncommitted,
	})
	writeResult(w, response, err)
}

func (a *App) serveBranchStack(w http.ResponseWriter, r *http.Request, query url.Values) {
	response, err := a.app.SourceControl.BranchStack(r.Context(), query.Get("project"), query.Get("base"))
	writeResult(w, response, err)
}

func (a *App) serveRangeFiles(w http.ResponseWriter, r *http.Request, query url.Values) {
	response, err := a.app.SourceControl.RangeFiles(r.Context(), query.Get("project"), query.Get("from"), query.Get("to"))
	writeResult(w, response, err)
}

func (a *App) serveRefs(w http.ResponseWriter, query url.Values) {
	response, err := a.app.SourceControl.Refs(query.Get("project"))
	writeResult(w, response, err)
}

// serveCompareStats counts the lines each file of a compare adds and
// removes: compare=working, branch (base, head, uncommitted), commit
// (commit), or range (from and to).
func (a *App) serveCompareStats(w http.ResponseWriter, r *http.Request, query url.Values) {
	request := contract.CompareStatsRequest{
		Project: query.Get("project"), Compare: contract.CompareKind(query.Get("compare")),
		Base: query.Get("base"), Head: query.Get("head"), Uncommitted: query.Has("uncommitted"),
		Commit: query.Get("commit"), From: query.Get("from"), To: query.Get("to"),
	}
	// uncommitted's only value is 1; another fails once the compare is
	// known to take it.
	if query.Has("uncommitted") && query.Get("uncommitted") != "1" && request.Compare == contract.CompareBranch {
		if _, err := a.app.Projects.Resolve(request.Project); err != nil {
			writeError(w, err)
			return
		}
		writeQueryError(w, "Uncommitted must be 1.")
		return
	}
	response, err := a.app.SourceControl.CompareStats(r.Context(), request)
	writeResult(w, response, err)
}

// writeResult writes an operation's response, or its error.
func writeResult(w http.ResponseWriter, response any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func writeQueryError(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, errorResponse{Error: message})
}
