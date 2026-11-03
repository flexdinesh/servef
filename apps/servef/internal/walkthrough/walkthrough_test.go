package walkthrough

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func absoluteProject(t *testing.T) string {
	t.Helper()
	project, err := filepath.Abs("project")
	if err != nil {
		t.Fatal(err)
	}
	return project
}

func TestDecodeRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	for name, input := range map[string]string{
		"unknown field":      `{"project":"/p","title":"t","steps":[{"title":"s","fiel":{}}]}`,
		"trailing data":      `{"project":"/p","title":"t","steps":[]} {}`,
		"not an object":      `[]`,
		"wrong type":         `{"project":"/p","title":"t","steps":[{"title":"s","file":{"path":"a","lines":"1-2"}}]}`,
		"three line numbers": `{"project":"/p","title":"t","steps":[{"title":"s","file":{"path":"a","lines":[1,2,3]}}]}`,
	} {
		if _, err := Decode(strings.NewReader(input)); !errors.Is(err, ErrInvalidJSON) {
			t.Errorf("%s: Decode error = %v, want ErrInvalidJSON", name, err)
		}
	}
	draft, err := Decode(strings.NewReader(`{"project":"/p","title":"t","open":true,"steps":[{"title":"s","diff":{"compare":{"kind":"commit","commit":"abcd"},"path":"a.go","lines":[2,4]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !draft.Open || draft.Steps[0].Diff == nil || *draft.Steps[0].Diff.Lines != (Lines{2, 4}) {
		t.Fatalf("draft = %+v", draft)
	}
}

func TestNormalizeTrimsAndDefaults(t *testing.T) {
	project := absoluteProject(t)
	draft, err := Draft{
		Project: " " + project + " ",
		Title:   "  How login works ",
		Steps: []Step{
			{Title: " Intro "},
			{Title: "File", File: &FileTarget{Path: " a.go ", At: " ABCDEF1 "}},
			{Title: "Diff", Diff: &DiffTarget{Compare: Compare{Kind: "range", From: "ABCD", To: "ef01"}, Path: "a.go"}},
		},
	}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if draft.Project != project || draft.Title != "How login works" || draft.Steps[0].Title != "Intro" {
		t.Errorf("trimmed draft = %+v", draft)
	}
	if file := draft.Steps[1].File; file.Path != "a.go" || file.At != "abcdef1" {
		t.Errorf("file = %+v", file)
	}
	if diff := draft.Steps[2].Diff; diff.Side != SideNew || diff.Compare.From != "abcd" {
		t.Errorf("diff = %+v", diff)
	}
}

func TestNormalizeReportsEveryProblemWithItsField(t *testing.T) {
	lines := func(start, end int) *Lines { return &Lines{start, end} }
	_, err := Draft{
		Project: "relative",
		ID:      "Bad ID",
		Steps: []Step{
			{},
			{Title: "both", File: &FileTarget{Path: "a"}, Diff: &DiffTarget{Compare: Compare{Kind: CompareWorking}}},
			{Title: "escape", File: &FileTarget{Path: "../a", Lines: lines(5, 2), At: "xyz"}},
			{Title: "absolute", File: &FileTarget{Path: "/etc/passwd", Lines: lines(0, 2)}},
			{Title: "unclean", File: &FileTarget{Path: "a/./b", Expect: "x"}},
			{Title: "kind", Diff: &DiffTarget{Compare: Compare{Kind: "stash"}}},
			{Title: "commit", Diff: &DiffTarget{Compare: Compare{Kind: CompareCommit, Base: "main"}, Side: "left", Lines: lines(1, 1)}},
			{Title: "range", Diff: &DiffTarget{Compare: Compare{Kind: CompareRange, From: "abcd", To: "main"}, Path: "a", Expect: "two\nlines", Lines: lines(1, 1)}},
		},
	}.Normalize()
	var problems Problems
	if !errors.As(err, &problems) {
		t.Fatalf("error = %v, want Problems", err)
	}
	fields := map[string]bool{}
	for _, problem := range problems {
		fields[problem.Field] = true
	}
	for _, want := range []string{
		"project", "id", "title",
		"steps[0].title",
		"steps[1]",
		"steps[2].file.path", "steps[2].file.lines", "steps[2].file.at",
		"steps[3].file.path", "steps[3].file.lines",
		"steps[4].file.path", "steps[4].file.expect",
		"steps[5].diff.compare.kind",
		"steps[6].diff.compare.base", "steps[6].diff.compare.commit", "steps[6].diff.side", "steps[6].diff.lines",
		"steps[7].diff.compare.to", "steps[7].diff.expect",
	} {
		if !fields[want] {
			t.Errorf("no problem for %s in %v", want, problems)
		}
	}
	if !strings.Contains(err.Error(), "steps[2].file.lines: end 2 is before start 5") {
		t.Errorf("message = %q", err)
	}
}

func TestNormalizeEnforcesLimits(t *testing.T) {
	project := absoluteProject(t)
	steps := make([]Step, MaxSteps+1)
	for i := range steps {
		steps[i] = Step{Title: "s"}
	}
	long := strings.Repeat("x", MaxBodyBytes+1)
	_, err := Draft{Project: project, Title: strings.Repeat("t", MaxTitleLength+1), Summary: long, Steps: steps}.Normalize()
	for _, want := range []string{"title: is longer than 200", "summary: is longer than", "steps: has 201 steps"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want %q", err, want)
		}
	}
	_, err = Draft{Project: project, Title: "t", Steps: []Step{{Title: "s", Body: long}}}.Normalize()
	if err == nil || !strings.Contains(err.Error(), "steps[0].body") {
		t.Errorf("long body error = %v", err)
	}
	if _, err := (Draft{Project: project, Title: "t"}).Normalize(); err == nil || !strings.Contains(err.Error(), "needs at least one step") {
		t.Errorf("no steps error = %v", err)
	}
}

func TestExpectPrefixTrimsAndCaps(t *testing.T) {
	if got := ExpectPrefix([]byte("\t  func Login() {\r")); got != "func Login() {" {
		t.Errorf("ExpectPrefix = %q", got)
	}
	long := ExpectPrefix([]byte(strings.Repeat("é", MaxExpectLength+10)))
	if len([]rune(long)) != MaxExpectLength {
		t.Errorf("long prefix has %d characters", len([]rune(long)))
	}
}

func TestSlug(t *testing.T) {
	for title, want := range map[string]string{
		"How Login Works!":        "how-login-works",
		"  --PR #42: auth-- ":     "pr-42-auth",
		"日本語":                     "",
		strings.Repeat("ab ", 40): strings.TrimSuffix(strings.Repeat("ab-", 16), "-"),
	} {
		if got := slug(title); got != want {
			t.Errorf("slug(%q) = %q, want %q", title, got, want)
		}
	}
}
