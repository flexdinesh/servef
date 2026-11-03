// Package walkthrough owns agent-authored walkthroughs: ordered steps through
// a project's files and diffs, each narrated in Markdown. It validates their
// structure, stores them, and publishes them through Service. It knows no
// HTTP and no git: transports adapt requests to Service, and Targets checks
// steps against the project.
package walkthrough

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits bound what one walkthrough may hold (decision 0047).
const (
	// MaxRequestBytes bounds one encoded walkthrough.
	MaxRequestBytes = 1 << 20
	// MaxSteps bounds a walkthrough's steps.
	MaxSteps = 200
	// MaxTitleLength bounds titles, in characters.
	MaxTitleLength = 200
	// MaxBodyBytes bounds each Markdown body and the summary.
	MaxBodyBytes = 32 << 10
	// MaxExpectLength bounds an expected line prefix, in characters.
	MaxExpectLength = 200
	// MaxPerProject bounds the walkthroughs stored for one project; the
	// least recently updated are evicted past it.
	MaxPerProject = 100
)

// Compare kinds name what a diff step compares, as Source Control does
// (decisions 0035 and 0040).
const (
	CompareWorking = "working"
	CompareBranch  = "branch"
	CompareCommit  = "commit"
	CompareRange   = "range"
)

// Diff sides name which side's line numbers a diff step's lines use.
const (
	SideNew = "new"
	SideOld = "old"
)

// Walkthrough is a stored, published walkthrough.
type Walkthrough struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	Steps     []Step    `json:"steps"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Summary lists a walkthrough without its steps.
type Summary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	StepCount int       `json:"stepCount"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Draft is a walkthrough as an agent submits it. Project is a project ID: an
// absolute directory within the served roots. A missing ID is generated from
// the title; an existing ID is replaced. Open asks the server to show the
// walkthrough in a browser; it is not stored.
type Draft struct {
	Project string `json:"project"`
	ID      string `json:"id,omitempty"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	Steps   []Step `json:"steps"`
	Open    bool   `json:"open,omitempty"`
}

// Step is one stop. A step with neither File nor Diff is a note.
type Step struct {
	Title string      `json:"title"`
	Body  string      `json:"body,omitempty"`
	File  *FileTarget `json:"file,omitempty"`
	Diff  *DiffTarget `json:"diff,omitempty"`
}

// Lines is an inclusive, 1-based line range.
type Lines [2]int

// UnmarshalJSON accepts exactly [start, end]; a Go array would drop extra
// numbers silently.
func (l *Lines) UnmarshalJSON(data []byte) error {
	var values []int
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if len(values) != 2 {
		return fmt.Errorf("lines must be [start, end], got %d numbers", len(values))
	}
	*l = Lines{values[0], values[1]}
	return nil
}

// FileTarget is a file in the working tree, or at commit At, optionally at
// Lines. Expect is the trimmed text the first line started with when the
// step was written.
type FileTarget struct {
	Path   string `json:"path"`
	Lines  *Lines `json:"lines,omitempty"`
	At     string `json:"at,omitempty"`
	Expect string `json:"expect,omitempty"`
}

// DiffTarget is a file's diff in a compare, optionally at Lines of Side. No
// path opens the compare's review at its first file.
type DiffTarget struct {
	Compare Compare `json:"compare"`
	Path    string  `json:"path,omitempty"`
	Side    string  `json:"side,omitempty"`
	Lines   *Lines  `json:"lines,omitempty"`
	Expect  string  `json:"expect,omitempty"`
}

// Compare names a Source Control compare: working (uncommitted changes),
// branch (Base, default the detected base, to Head, default HEAD), commit
// (Commit against its first parent), or range (From's parent to To).
type Compare struct {
	Kind   string `json:"kind"`
	Base   string `json:"base,omitempty"`
	Head   string `json:"head,omitempty"`
	Commit string `json:"commit,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
}

// Problem is one reason a draft was rejected, at a JSON field path such as
// steps[3].file.lines.
type Problem struct {
	Field   string
	Message string
}

// Problems is a rejected draft's every problem.
type Problems []Problem

func (p Problems) Error() string {
	messages := make([]string, len(p))
	for i, problem := range p {
		messages[i] = problem.Field + ": " + problem.Message
	}
	return "invalid walkthrough: " + strings.Join(messages, "; ")
}

// ErrInvalidJSON wraps a draft that is not one strict JSON object.
var ErrInvalidJSON = errors.New("invalid walkthrough JSON")

// Decode reads one JSON draft strictly: unknown fields and trailing data are
// rejected, so a misspelled field is reported rather than ignored.
func Decode(r io.Reader) (Draft, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var draft Draft
	if err := decoder.Decode(&draft); err != nil {
		return Draft{}, fmt.Errorf("%w: %w", ErrInvalidJSON, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Draft{}, fmt.Errorf("%w: data after the walkthrough object", ErrInvalidJSON)
	}
	return draft, nil
}

var (
	idPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	hashPattern = regexp.MustCompile(`^[0-9a-f]{4,40}$`)
)

// Normalize returns the draft with text trimmed, hashes lowercased, and
// diff sides defaulted, then checks its structure. Targets are not checked
// against the project.
func (d Draft) Normalize() (Draft, error) {
	d.Project = strings.TrimSpace(d.Project)
	d.ID = strings.TrimSpace(d.ID)
	d.Title = strings.TrimSpace(d.Title)
	steps := make([]Step, len(d.Steps))
	for i, step := range d.Steps {
		steps[i] = normalizeStep(step)
	}
	d.Steps = steps

	var problems Problems
	add := func(field, format string, args ...any) {
		problems = append(problems, Problem{Field: field, Message: fmt.Sprintf(format, args...)})
	}
	if d.Project == "" {
		add("project", "is required: the project's absolute directory")
	} else if !filepath.IsAbs(d.Project) {
		add("project", "must be an absolute directory")
	}
	if d.ID != "" && !idPattern.MatchString(d.ID) {
		add("id", "must be 1–64 lowercase letters, digits, or hyphens, starting with a letter or digit")
	}
	checkText(add, "title", d.Title, true)
	if len(d.Summary) > MaxBodyBytes {
		add("summary", "is longer than %d bytes", MaxBodyBytes)
	}
	switch {
	case len(d.Steps) == 0:
		add("steps", "needs at least one step")
	case len(d.Steps) > MaxSteps:
		add("steps", "has %d steps; at most %d are allowed", len(d.Steps), MaxSteps)
	}
	for i, step := range d.Steps {
		checkStep(add, fmt.Sprintf("steps[%d]", i), step)
	}
	if len(problems) > 0 {
		return d, problems
	}
	return d, nil
}

func normalizeStep(step Step) Step {
	step.Title = strings.TrimSpace(step.Title)
	if step.File != nil {
		file := *step.File
		file.Path = strings.TrimSpace(file.Path)
		file.At = strings.ToLower(strings.TrimSpace(file.At))
		file.Expect = strings.TrimSpace(file.Expect)
		step.File = &file
	}
	if step.Diff != nil {
		diff := *step.Diff
		diff.Path = strings.TrimSpace(diff.Path)
		diff.Expect = strings.TrimSpace(diff.Expect)
		if diff.Side == "" {
			diff.Side = SideNew
		}
		diff.Compare.Kind = strings.TrimSpace(diff.Compare.Kind)
		diff.Compare.Base = strings.TrimSpace(diff.Compare.Base)
		diff.Compare.Head = strings.TrimSpace(diff.Compare.Head)
		diff.Compare.Commit = strings.ToLower(strings.TrimSpace(diff.Compare.Commit))
		diff.Compare.From = strings.ToLower(strings.TrimSpace(diff.Compare.From))
		diff.Compare.To = strings.ToLower(strings.TrimSpace(diff.Compare.To))
		step.Diff = &diff
	}
	return step
}

type addProblem func(field, format string, args ...any)

func checkText(add addProblem, field, value string, required bool) {
	switch {
	case required && value == "":
		add(field, "is required")
	case utf8.RuneCountInString(value) > MaxTitleLength:
		add(field, "is longer than %d characters", MaxTitleLength)
	}
}

func checkStep(add addProblem, field string, step Step) {
	checkText(add, field+".title", step.Title, true)
	if len(step.Body) > MaxBodyBytes {
		add(field+".body", "is longer than %d bytes", MaxBodyBytes)
	}
	switch {
	case step.File != nil && step.Diff != nil:
		add(field, "has both file and diff; a step targets one or neither (a note)")
	case step.File != nil:
		file := step.File
		checkPath(add, field+".file.path", file.Path, true)
		checkLines(add, field+".file.lines", file.Lines)
		if file.At != "" && !hashPattern.MatchString(file.At) {
			add(field+".file.at", "must be a commit hash of 4–40 hexadecimal digits")
		}
		checkExpect(add, field+".file", file.Expect, file.Lines)
	case step.Diff != nil:
		diff := step.Diff
		checkCompare(add, field+".diff.compare", diff.Compare)
		checkPath(add, field+".diff.path", diff.Path, false)
		if diff.Side != SideNew && diff.Side != SideOld {
			add(field+".diff.side", `must be "new" or "old"`)
		}
		checkLines(add, field+".diff.lines", diff.Lines)
		if diff.Lines != nil && diff.Path == "" {
			add(field+".diff.lines", "needs a path")
		}
		checkExpect(add, field+".diff", diff.Expect, diff.Lines)
	}
}

func checkPath(add addProblem, field, name string, required bool) {
	if name == "" {
		if required {
			add(field, "is required")
		}
		return
	}
	if strings.Contains(name, `\`) || path.IsAbs(name) || path.Clean(name) != name || name == "." || name == ".." || strings.HasPrefix(name, "../") {
		add(field, "must be a clean, project-relative path with forward slashes")
	}
}

func checkLines(add addProblem, field string, lines *Lines) {
	if lines == nil {
		return
	}
	switch {
	case lines[0] < 1:
		add(field, "start must be at least 1")
	case lines[1] < lines[0]:
		add(field, "end %d is before start %d", lines[1], lines[0])
	}
}

func checkExpect(add addProblem, field, expect string, lines *Lines) {
	if expect == "" {
		return
	}
	if lines == nil {
		add(field+".expect", "needs lines")
	}
	if strings.ContainsAny(expect, "\r\n") || utf8.RuneCountInString(expect) > MaxExpectLength {
		add(field+".expect", "must be one line of at most %d characters", MaxExpectLength)
	}
}

func checkCompare(add addProblem, field string, compare Compare) {
	allowed := map[string]bool{}
	switch compare.Kind {
	case CompareWorking:
	case CompareBranch:
		allowed["base"], allowed["head"] = true, true
	case CompareCommit:
		allowed["commit"] = true
	case CompareRange:
		allowed["from"], allowed["to"] = true, true
	default:
		add(field+".kind", `must be "working", "branch", "commit", or "range"`)
		return
	}
	for _, value := range []struct {
		name, value string
		hash        bool
	}{{"base", compare.Base, false}, {"head", compare.Head, false}, {"commit", compare.Commit, true}, {"from", compare.From, true}, {"to", compare.To, true}} {
		switch {
		case value.value != "" && !allowed[value.name]:
			add(field+"."+value.name, "doesn't apply to a %s compare", compare.Kind)
		case value.hash && allowed[value.name] && value.value == "":
			add(field+"."+value.name, "is required for a %s compare", compare.Kind)
		case value.hash && value.value != "" && !hashPattern.MatchString(value.value):
			add(field+"."+value.name, "must be a commit hash of 4–40 hexadecimal digits")
		}
	}
}

// ExpectPrefix returns the trimmed text a line starts with, cut to
// MaxExpectLength characters, as a step's Expect records it.
func ExpectPrefix(line []byte) string {
	text := strings.TrimSpace(string(bytes.TrimRight(line, "\r")))
	if utf8.RuneCountInString(text) <= MaxExpectLength {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:MaxExpectLength]))
}

// slug derives an ID prefix from a title: lowercase letters and digits
// joined by single hyphens, at most 48 characters.
func slug(title string) string {
	var builder strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if hyphen && builder.Len() > 0 {
				builder.WriteByte('-')
			}
			hyphen = false
			builder.WriteRune(r)
		default:
			hyphen = true
		}
		if builder.Len() > 48 {
			break
		}
	}
	value := builder.String()
	return strings.Trim(value[:min(len(value), 48)], "-")
}
