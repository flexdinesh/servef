package diffview

import (
	"fmt"
	"github.com/flexdinesh/servef/packages/mygo/diffs"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

type harness struct {
	t     *testing.T
	state *State
	tt    *ui.Tester
	got   []diffs.Selection
}

func newHarness(t *testing.T, d *diffs.Diff, style diffs.Style) *harness {
	return newHarnessWith(t, d, &State{Style: style})
}

// newHarnessWith shows d in state, set up before the first frame.
func newHarnessWith(t *testing.T, d *diffs.Diff, state *State) *harness {
	h := &harness{t: t, state: state}
	h.state.SetDiff(d)
	h.state.OnSelect = func(s diffs.Selection) { h.got = append(h.got, s) }
	h.state.Utility = func(c *ui.Context, s diffs.Selection) {
		ui.Text(c, "Copy "+s.Anchor()).FontSize(10)
	}
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { View(c, h.state).Grow(1) })
	}, 900, 640)
	h.tt.Frame()
	return h
}

// shot writes the frame to $DIFFS_SHOTS/<name>.png when set, to look at.
func (h *harness) shot(name string) {
	dir := os.Getenv("DIFFS_SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, h.tt.Image()); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) click(s string) {
	h.t.Helper()
	if err := h.tt.Click(s); err != nil {
		h.t.Fatalf("click %q: %v (texts %q)", s, err, h.tt.Texts())
	}
	h.tt.Frame()
}

func (h *harness) shiftClick(s string) {
	h.t.Helper()
	if err := h.tt.ClickWith(ui.Shift, s); err != nil {
		h.t.Fatalf("shift-click %q: %v", s, err)
	}
	h.tt.Frame()
}

func (h *harness) want(texts ...string) {
	h.t.Helper()
	for _, s := range texts {
		if !slices.Contains(h.tt.Texts(), s) {
			h.t.Errorf("no text %q in %q", s, h.tt.Texts())
		}
	}
}

func (h *harness) wantNot(texts ...string) {
	h.t.Helper()
	for _, s := range texts {
		if slices.Contains(h.tt.Texts(), s) {
			h.t.Errorf("text %q shows", s)
		}
	}
}

func (h *harness) key(mods ui.Modifiers, k ui.Key) {
	h.tt.Key(mods, k)
	h.tt.Frame()
}

func TestViewShowsHunksBetweenCollapsedGaps(t *testing.T) {
	h := newHarness(t, diffOf("hunks"), diffs.Split)
	h.shot("split")
	h.want("5 unmodified lines", "11 unmodified lines", "6 unmodified lines", "Expand down", "Expand", "Expand up",
		"L6", "R6", "line 6", "L10", "R10", "R11", "ten", "TEN", "L30")
	h.wantNot("line 2", "L2", "line 20")

	h.state.Style = diffs.Unified
	h.tt.Frame()
	h.shot("unified")
	h.want("L10", "R10", "R11", "R6")
	h.wantNot("L6") // unified unchanged rows number the new side
}

func TestViewExpandsGapsInPlace(t *testing.T) {
	h := newHarness(t, diffOf("hunks"), diffs.Split)
	h.click("Expand down")
	h.want("line 1", "line 5", "R1")
	h.wantNot("5 unmodified lines")
	h.click("11 unmodified lines")
	h.want("line 16", "line 25")
	h.wantNot("11 unmodified lines")
	h.shot("expanded")
	if got := h.state.Rows()[0]; got.Kind != diffs.LineRow || got.New.Line != 1 {
		t.Errorf("first row %+v", got)
	}

	// Shift expands a gap whole from any button.
	h = newHarness(t, diffOf("long"), diffs.Split)
	if err := h.tt.ClickWith(ui.Shift, "Expand down"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.wantNot("15 unmodified lines")
	h.want("line 1", "17 unmodified lines")
}

func TestViewSelectsLinesAndShowsTheUtility(t *testing.T) {
	h := newHarness(t, diffOf("hunks"), diffs.Split)
	h.click("R7")
	if want := []diffs.Selection{sel(diffs.New, 7, diffs.New, 7)}; !slices.Equal(h.got, want) {
		t.Fatalf("selections %v, want %v", h.got, want)
	}
	h.want("Copy R7")
	h.shiftClick("R12")
	s, ok := h.state.Selection()
	if !ok || s != sel(diffs.New, 7, diffs.New, 12) || s.Anchor() != "R7-R12" {
		t.Fatalf("selection %v %v", s, ok)
	}
	h.want("Copy R7-R12")
	h.wantNot("Copy R7")
	if target, ok := h.state.Diff().Target(s, diffs.Split); !ok || target.Reference("notes.txt") != "notes.txt:7-12" {
		t.Errorf("target %v", target)
	}
	h.shot("selected")
	// Across sides: from the deleted line to an added one.
	h.click("L10")
	h.shiftClick("R11")
	if s, _ := h.state.Selection(); s.Anchor() != "L10-R11" {
		t.Errorf("anchor %q", s.Anchor())
	}
	h.key(0, ui.KeyEscape)
	if _, ok := h.state.Selection(); ok {
		t.Error("Escape kept the selection")
	}
}

func TestViewSelectRevealsCollapsedLines(t *testing.T) {
	h := newHarness(t, diffOf("hunks"), diffs.Split)
	s, _ := diffs.ParseAnchor("#R20")
	h.state.Select(s)
	h.tt.Frame()
	h.want("R20", "line 20", "Copy R20")
	h.wantNot("11 unmodified lines")
}

func TestViewStepsThroughHunks(t *testing.T) {
	h := newHarness(t, diffOf("hunks"), diffs.Split)
	h.click("R7") // focus the viewer
	h.got = nil
	h.key(0, ui.KeyF7)
	h.key(0, ui.KeyF7)
	h.key(ui.Shift, ui.KeyF7)
	want := []diffs.Selection{sel(diffs.New, 27, diffs.New, 34), sel(diffs.New, 6, diffs.New, 15), sel(diffs.New, 27, diffs.New, 34)}
	if !slices.Equal(h.got, want) {
		t.Fatalf("steps %v, want %v", h.got, want)
	}
	h.want("Copy R27-R34")

	// Without a selection, a step starts from the rows in view.
	h = newHarness(t, diffOf("hunks"), diffs.Unified)
	if !h.state.StepHunk(1) {
		t.Fatal("no step")
	}
	if s, _ := h.state.Selection(); s != sel(diffs.New, 6, diffs.New, 15) {
		t.Errorf("first step %v", s)
	}
}

func TestViewMarksMissingNewlines(t *testing.T) {
	d := &diffs.Diff{Old: diffs.SplitLines("a\nb\n"), New: diffs.SplitLines("a\nc"), Hunks: []diffs.Hunk{
		hunk(1, 2, 1, 2, block(diffs.Context, 1, 1, 1, 1), block(diffs.Change, 2, 1, 2, 1)),
	}}
	h := newHarness(t, d, diffs.Unified)
	h.want("No newline at end of file")
	h.shot("no-newline")
}

func TestViewEmphasizesChangedWordsAndTakesHostColors(t *testing.T) {
	old := "\tfmt.Println(\"hello\") // go-search-token\n"
	new := "\tfmt.Println(\"hello from servef\") // go-search-token\n"
	d := &diffs.Diff{Old: diffs.SplitLines(old), New: diffs.SplitLines(new), Hunks: []diffs.Hunk{hunk(1, 1, 1, 1, block(diffs.Change, 1, 1, 1, 1))}}
	red := ui.Hex("#ff0000")
	h := newHarness(t, d, diffs.Split)
	h.state.Spans = func(side diffs.Side, line int) []Span {
		return []Span{{End: 4, Color: red}} // the tab and "fmt"
	}
	th := DefaultTheme(&ui.Theme{Background: ui.Hex("#ffffff"), Text: ui.Hex("#333333"), Success: ui.Hex("#13703f"), Danger: ui.Hex("#bd3029")})
	spans := h.state.spans(th, diffs.New, h.state.Rows()[0].New)
	var emphasized, colored []string
	for _, s := range spans {
		if s.Background == th.AddedWord {
			emphasized = append(emphasized, s.Text)
		}
		if s.Color == red {
			colored = append(colored, s.Text)
		}
	}
	if !slices.Equal(emphasized, []string{" from servef"}) {
		t.Errorf("emphasized %q", emphasized)
	}
	if !slices.Equal(colored, []string{"  fmt"}) {
		t.Errorf("colored %q (tabs expand to two columns)", colored)
	}
	h.tt.Frame()
	h.shot("words")
	if got := spanText(spans); got != "  fmt.Println(\"hello from servef\") // go-search-token" {
		t.Errorf("text %q", got)
	}
	// Off: no emphasis.
	h.state.Inline = diffs.NoInline
	for _, s := range h.state.spans(th, diffs.New, h.state.Rows()[0].New) {
		if s.Background != (ui.Color{}) {
			t.Errorf("emphasis with NoInline: %q", s.Text)
		}
	}
}

func spanText(spans []ui.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestViewBuildsOnlyTheRowsInView(t *testing.T) {
	var old, new strings.Builder
	for i := range 100000 {
		fmt.Fprintf(&old, "line %d\n", i+1)
		fmt.Fprintf(&new, "line %d\n", i+1)
	}
	d := &diffs.Diff{Old: diffs.SplitLines(old.String()), New: diffs.SplitLines(new.String() + "added\n"), Hunks: []diffs.Hunk{
		hunk(99997, 4, 99997, 5, block(diffs.Context, 99997, 4, 99997, 4), block(diffs.Change, 100001, 0, 100001, 1)),
	}, Gaps: []diffs.Gap{gap(0, 1, 1, 99996)}}
	h := newHarness(t, d, diffs.Unified)
	h.state.ExpandUnchanged = true
	h.tt.Frame()
	if n := len(h.tt.Texts()); n > 400 {
		t.Errorf("%d texts built for 100001 rows", n)
	}
	h.want("line 1")
	h.state.StepHunk(1)
	h.tt.Frame()
	h.tt.Frame()
	h.want("Copy R99997-R100001", "added")
	h.wantNot("line 1")
}
