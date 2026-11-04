package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/git"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/walkthrough"
	"github.com/flexdinesh/servef/pkg/contract"
)

// walkthroughTargets checks walkthrough steps against projects within roots.
type walkthroughTargets struct {
	roots    *projects.RootSet
	depth    int
	types    *document.Registry
	openFile func(string) (fs.File, error)
}

// NewWalkthroughTargets returns the checks POST /api/walkthroughs applies:
// projects resolve within roots; working-tree files through a scan within
// depth; commit files and diffs through git. servef walkthrough publish
// shares them without a server (decision 0065).
func NewWalkthroughTargets(roots *projects.RootSet, depth int, types *document.Registry) walkthrough.Targets {
	return walkthroughTargets{roots: roots, depth: depth, types: types, openFile: func(name string) (fs.File, error) { return os.Open(name) }}
}

// Check resolves the project, then checks every step's target as the
// browser will open it: working-tree files through the scanned index
// (within Depth), commit files and diffs through git. Abbreviated hashes become
// full; missing Expect prefixes are recorded for targets that can drift
// (the working tree and branches).
func (t walkthroughTargets) Check(ctx context.Context, projectID string, steps []walkthrough.Step) (string, []walkthrough.Step, error) {
	project, err := ResolveProject(t.roots.List(), projectID)
	if err != nil {
		return "", nil, walkthrough.Problems{{Field: "project", Message: AsError(err).Message}}
	}
	checker := &stepChecker{targets: t, ctx: ctx, root: project.Root, compares: map[walkthrough.Compare]compareResult{}}
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
	return project.Root, checked, nil
}

type compareResult struct {
	sides CompareSides
	// problem rejects every step of the compare; its field is relative to
	// the step's diff.
	problem *walkthrough.Problem
}

// stepChecker checks one draft's steps, scanning the project and opening
// its repository at most once.
type stepChecker struct {
	targets  walkthroughTargets
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
	fileType := c.targets.types.Lookup(file.Path).Type()
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
		index, err := files.Scan(c.root, c.targets.depth, func(string) bool { return true })
		if err != nil {
			return err
		}
		c.index = index
	}
	resolved, err := c.index.Resolve(file.Path)
	if err != nil {
		c.problem(field+".path", "is not a file servef shows in the project (missing, excluded, or too deep)")
		return nil
	}
	if file.Lines == nil {
		return nil
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	open := func() (io.ReadCloser, error) { return c.targets.openFile(resolved) }
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
		sides, err := ReadCompareSides(c.ctx, repo, c.root, c.targets.openFile, contract.CompareKind(compare.Kind), compare.Base, compare.Head, false, hashes)
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
	for i, candidate := range result.sides.Changes {
		if candidate.Path == diff.Path || candidate.OldPath == diff.Path {
			change = &result.sides.Changes[i]
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
	side := result.sides.Modified
	switch {
	case diff.Side == walkthrough.SideOld && (change.Status == git.Added || change.Status == git.Untracked):
		c.problem(field+".side", "is old, but the file is %s and has no old side", change.Status)
		return nil
	case diff.Side == walkthrough.SideNew && change.Status == git.Deleted:
		c.problem(field+".side", "is new, but the file is deleted and has no new side")
		return nil
	case diff.Side == walkthrough.SideOld:
		side = result.sides.Original
	}
	read, err := side(*change)
	if err != nil {
		return err
	}
	name := change.Path
	if diff.Side == walkthrough.SideOld && change.OldPath != "" {
		name = change.OldPath
	}
	fileType := c.targets.types.Lookup(name).Type()
	var expect *string
	if compare.Kind == walkthrough.CompareWorking || compare.Kind == walkthrough.CompareBranch {
		expect = &diff.Expect
	}
	// Diffs show sides up to PreviewLimit (decision 0004).
	return c.lines(field, diff.Lines, fileType, read.Size, document.PreviewLimit, read.Open, expect)
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
	text, binary, err := document.ReadText(reader, limit)
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
