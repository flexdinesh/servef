package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/git"
	"github.com/flexdinesh/servef/internal/walkthrough"
)

// Walkthroughs (decision 0047) are published by agents and read by the
// browser. Publishing is the only write: it accepts JSON only, from no
// foreign origin, so other sites in the browser cannot publish or delete.

type publishResponse struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Created bool   `json:"created"`
}

// walkthroughResponse is a stored walkthrough with its Markdown rendered.
type walkthroughResponse struct {
	ID          string                    `json:"id"`
	Project     string                    `json:"project"`
	Title       string                    `json:"title"`
	Summary     string                    `json:"summary,omitempty"`
	SummaryHTML string                    `json:"summaryHTML"`
	Steps       []walkthroughStepResponse `json:"steps"`
	CreatedAt   time.Time                 `json:"createdAt"`
	UpdatedAt   time.Time                 `json:"updatedAt"`
}

type walkthroughStepResponse struct {
	walkthrough.Step
	BodyHTML string `json:"bodyHTML"`
}

// sameOrigin reports whether a request came from servef's own page or from
// outside a browser: a present Origin must name the request's own host.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Path == "" && strings.EqualFold(parsed.Host, r.Host)
}

// writeForeignOrigin refuses a write from another site; it reports whether
// it did.
func writeForeignOrigin(w http.ResponseWriter, r *http.Request) bool {
	if sameOrigin(r) {
		return false
	}
	writeJSON(w, http.StatusForbidden, errorResponse{Error: "Walkthroughs can't be changed from another site.", Code: "foreignOrigin"})
	return true
}

func (a *App) servePublishWalkthrough(w http.ResponseWriter, r *http.Request) {
	if writeForeignOrigin(w, r) {
		return
	}
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, errorResponse{Error: "Send the walkthrough as Content-Type: application/json.", Code: "unsupportedMediaType"})
		return
	}
	draft, err := walkthrough.Decode(http.MaxBytesReader(w, r.Body, walkthrough.MaxRequestBytes))
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: fmt.Sprintf("A walkthrough is at most %d bytes.", walkthrough.MaxRequestBytes), Code: "tooLarge"})
		return
	case err != nil:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "invalidWalkthrough"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	published, err := a.walkthroughs.Publish(ctx, draft)
	var problems walkthrough.Problems
	switch {
	case errors.As(err, &problems):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: problems.Error(), Code: "invalidWalkthrough"})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't publish the walkthrough: " + err.Error()})
		return
	}

	query := url.Values{"project": {published.Walkthrough.Project}, "wt": {published.Walkthrough.ID}}
	target := (&url.URL{Scheme: "http", Host: r.Host, Path: "/view", RawQuery: query.Encode()}).String()
	if draft.Open && a.config.OpenBrowser != nil {
		go func() { _ = a.config.OpenBrowser(target) }()
	}
	status := http.StatusOK
	if published.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, publishResponse{ID: published.Walkthrough.ID, URL: target, Created: published.Created})
}

func (a *App) serveWalkthroughs(w http.ResponseWriter, projectID string) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	summaries, err := a.walkthroughs.List(project.root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't list walkthroughs: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (a *App) serveWalkthrough(w http.ResponseWriter, projectID, id string) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	stored, err := a.walkthroughs.Get(project.root, id)
	if errors.Is(err, walkthrough.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "That walkthrough is unavailable.", Code: "walkthroughUnavailable"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read the walkthrough: " + err.Error()})
		return
	}
	// Narration links resolve from the project root, like a root README's.
	context := document.Context{Project: stored.Project, Supports: a.config.Filenames.Match}
	render := func(source string) string {
		if source == "" {
			return ""
		}
		rendered, err := document.RenderMarkdown([]byte(source), context)
		if err != nil {
			return ""
		}
		return rendered
	}
	response := walkthroughResponse{ID: stored.ID, Project: stored.Project, Title: stored.Title, Summary: stored.Summary, SummaryHTML: render(stored.Summary), Steps: make([]walkthroughStepResponse, len(stored.Steps)), CreatedAt: stored.CreatedAt, UpdatedAt: stored.UpdatedAt}
	for i, step := range stored.Steps {
		response.Steps[i] = walkthroughStepResponse{Step: step, BodyHTML: render(step.Body)}
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) serveDeleteWalkthrough(w http.ResponseWriter, r *http.Request, projectID, id string) {
	if writeForeignOrigin(w, r) {
		return
	}
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	err := a.walkthroughs.Delete(project.root, id)
	if errors.Is(err, walkthrough.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "That walkthrough is unavailable.", Code: "walkthroughUnavailable"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't delete the walkthrough: " + err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// walkthroughTargets checks walkthrough steps against a served project.
type walkthroughTargets struct{ app *App }

// Check resolves the project, then checks every step's target as the
// browser will open it: working-tree files through the scanned index (the
// CLI scope), commit files and diffs through git. Abbreviated hashes become
// full; missing Expect prefixes are recorded for targets that can drift
// (the working tree and branches).
func (t walkthroughTargets) Check(ctx context.Context, projectID string, steps []walkthrough.Step) (string, []walkthrough.Step, error) {
	project, status, message := t.app.project(projectID)
	if status != http.StatusOK {
		return "", nil, walkthrough.Problems{{Field: "project", Message: message}}
	}
	checker := &stepChecker{app: t.app, ctx: ctx, root: project.root, compares: map[walkthrough.Compare]compareResult{}}
	defer checker.close()
	checked := make([]walkthrough.Step, len(steps))
	for i, step := range steps {
		field := fmt.Sprintf("steps[%d]", i)
		var err error
		switch {
		case step.File != nil:
			file := *step.File
			err = checker.file(field+".file", &file)
			step.File = &file
		case step.Diff != nil:
			diff := *step.Diff
			err = checker.diff(field+".diff", &diff)
			step.Diff = &diff
		}
		if err != nil {
			return "", nil, err
		}
		checked[i] = step
	}
	if len(checker.problems) > 0 {
		return "", nil, checker.problems
	}
	return project.root, checked, nil
}

type compareResult struct {
	sides compareSides
	// problem rejects every step of the compare; its field is relative to
	// the step's diff.
	problem *walkthrough.Problem
}

// stepChecker checks one draft's steps, scanning the project and opening
// its repository at most once.
type stepChecker struct {
	app      *App
	ctx      context.Context
	root     string
	index    *files.Index
	repo     *git.Repository
	opened   bool
	compares map[walkthrough.Compare]compareResult
	problems walkthrough.Problems
}

func (c *stepChecker) close() {
	if c.repo != nil {
		_ = c.repo.Close()
	}
}

func (c *stepChecker) problem(field, format string, args ...any) {
	c.problems = append(c.problems, walkthrough.Problem{Field: field, Message: fmt.Sprintf(format, args...)})
}

// repository opens the project's repository, or reports a problem at field
// when it has none.
func (c *stepChecker) repository(field string) (*git.Repository, error) {
	if !c.opened {
		c.opened = true
		repo, err := git.Open(c.root)
		if err != nil && !errors.Is(err, git.ErrNotRepository) {
			return nil, err
		}
		c.repo = repo
	}
	if c.repo == nil {
		c.problem(field, "needs a git repository; the project has none")
	}
	return c.repo, nil
}

// commit resolves an abbreviated hash, reporting a problem at field when no
// single commit matches.
func (c *stepChecker) commit(field, value string) (git.Hash, bool, error) {
	repo, err := c.repository(field)
	if err != nil || repo == nil {
		return git.Hash{}, false, err
	}
	hash, err := repo.ResolveCommit(value)
	switch {
	case errors.Is(err, git.ErrUnavailable):
		c.problem(field, "no commit matches %s", value)
	case errors.Is(err, git.ErrAmbiguousCommit):
		c.problem(field, "%s matches several commits; use more digits", value)
	case err != nil:
		return git.Hash{}, false, err
	default:
		return hash, true, nil
	}
	return git.Hash{}, false, nil
}

func (c *stepChecker) file(field string, file *walkthrough.FileTarget) error {
	fileType := c.app.config.Types.Lookup(file.Path).Type()
	if file.At != "" {
		hash, ok, err := c.commit(field+".at", file.At)
		if err != nil || !ok {
			return err
		}
		file.At = hash.String()
		blob, err := c.repo.CommitBlob(hash, file.Path)
		if errors.Is(err, git.ErrUnavailable) {
			c.problem(field+".path", "is not a file in commit %s", file.At[:7])
			return nil
		}
		if err != nil {
			return err
		}
		if file.Lines == nil {
			return nil
		}
		return c.lines(field, file.Lines, fileType, blob.Size, document.DisplayLimit, blob.Open, nil)
	}

	if c.index == nil {
		index, err := c.app.scan(c.root)
		if err != nil {
			return err
		}
		c.index = index
	}
	resolved, err := c.index.Resolve(file.Path)
	if err != nil {
		c.problem(field+".path", "is not a file servef shows in the project (missing, excluded, too deep, or outside --filename)")
		return nil
	}
	if file.Lines == nil {
		return nil
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	open := func() (io.ReadCloser, error) { return c.app.openFile(resolved) }
	return c.lines(field, file.Lines, fileType, info.Size(), document.DisplayLimit, open, &file.Expect)
}

func (c *stepChecker) diff(field string, diff *walkthrough.DiffTarget) error {
	repo, err := c.repository(field + ".compare")
	if err != nil || repo == nil {
		return err
	}
	compare := diff.Compare
	var hashes []git.Hash
	for _, value := range []struct {
		name   string
		target *string
	}{{"commit", &compare.Commit}, {"from", &compare.From}, {"to", &compare.To}} {
		if *value.target == "" {
			continue
		}
		hash, ok, err := c.commit(field+".compare."+value.name, *value.target)
		if err != nil || !ok {
			return err
		}
		*value.target = hash.String()
		hashes = append(hashes, hash)
	}
	diff.Compare = compare

	result, ok := c.compares[compare]
	if !ok {
		sides, err := c.app.compareSides(c.ctx, repo, c.root, compare.Kind, compare.Base, compare.Head, hashes)
		switch {
		case errors.Is(err, git.ErrUnknownBase):
			result.problem = &walkthrough.Problem{Field: ".compare.base", Message: "is not a branch servef lists"}
		case errors.Is(err, git.ErrUnknownHead):
			result.problem = &walkthrough.Problem{Field: ".compare.head", Message: "is not a local branch"}
		case errors.Is(err, git.ErrUnavailable):
			result.problem = &walkthrough.Problem{Field: ".compare", Message: "names an unavailable commit"}
		case errors.Is(err, git.ErrRangeTooLarge):
			result.problem = &walkthrough.Problem{Field: ".compare", Message: err.Error()}
		case err != nil:
			return err
		}
		result.sides = sides
		c.compares[compare] = result
	}
	if result.problem != nil {
		c.problem(field+result.problem.Field, "%s", result.problem.Message)
		return nil
	}
	if diff.Path == "" {
		return nil
	}

	var change *git.Change
	for i, candidate := range result.sides.changes {
		if candidate.Path == diff.Path || candidate.OldPath == diff.Path {
			change = &result.sides.changes[i]
			break
		}
	}
	if change == nil {
		c.problem(field+".path", "is not changed in this compare")
		return nil
	}
	diff.Path = change.Path
	if diff.Lines == nil {
		return nil
	}
	side := result.sides.modified
	switch {
	case diff.Side == walkthrough.SideOld && (change.Status == git.Added || change.Status == git.Untracked):
		c.problem(field+".side", "is old, but the file is %s and has no old side", change.Status)
		return nil
	case diff.Side == walkthrough.SideNew && change.Status == git.Deleted:
		c.problem(field+".side", "is new, but the file is deleted and has no new side")
		return nil
	case diff.Side == walkthrough.SideOld:
		side = result.sides.original
	}
	read, err := side(*change)
	if err != nil {
		return err
	}
	name := change.Path
	if diff.Side == walkthrough.SideOld && change.OldPath != "" {
		name = change.OldPath
	}
	fileType := c.app.config.Types.Lookup(name).Type()
	var expect *string
	if compare.Kind == walkthrough.CompareWorking || compare.Kind == walkthrough.CompareBranch {
		expect = &diff.Expect
	}
	// Diffs show sides up to PreviewLimit (decision 0004).
	return c.lines(field, diff.Lines, fileType, read.size, document.PreviewLimit, read.open, expect)
}

// lines checks that lines fall within a text file of size bytes read from
// open, and records the first line's prefix into expect when expect points
// to an empty one.
func (c *stepChecker) lines(field string, lines *walkthrough.Lines, fileType document.Type, size, limit int64, open func() (io.ReadCloser, error), expect *string) error {
	if !fileType.HasTextSource() || size > limit {
		c.problem(field+".lines", "can't be shown: the file is not shown as text lines (no text source, or over %d MiB)", limit>>20)
		return nil
	}
	reader, err := open()
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	text, binary, err := readText(reader, limit)
	if err != nil {
		return err
	}
	if binary {
		c.problem(field+".lines", "can't be shown: the file is binary")
		return nil
	}
	count := bytes.Count(text, []byte("\n"))
	if len(text) > 0 && !bytes.HasSuffix(text, []byte("\n")) {
		count++
	}
	if lines[1] > count {
		c.problem(field+".lines", "end %d is past the file's last line, %d", lines[1], count)
		return nil
	}
	if expect != nil && *expect == "" {
		line := text
		for range lines[0] - 1 {
			_, line, _ = bytes.Cut(line, []byte("\n"))
		}
		first, _, _ := bytes.Cut(line, []byte("\n"))
		*expect = walkthrough.ExpectPrefix(first)
	}
	return nil
}
