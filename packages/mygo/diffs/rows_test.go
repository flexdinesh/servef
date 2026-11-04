package diffs

import (
	"fmt"
	"strings"
	"testing"
)

// sketch draws rows as text: "3|3" unchanged, "-10|" deleted, "|+10"
// added, "~5 v" a separator hiding 5 lines with its buttons, "\ L" a
// no-newline mark.
func sketch(rows []Row) string {
	cell := func(c Cell, side string) string {
		switch c.Kind {
		case Deleted:
			return fmt.Sprintf("-%d", c.Line)
		case Added:
			return fmt.Sprintf("+%d", c.Line)
		case Unchanged:
			return fmt.Sprint(c.Line)
		}
		return ""
	}
	var out []string
	for _, r := range rows {
		switch r.Kind {
		case SeparatorRow:
			b := ""
			for _, on := range []struct {
				on bool
				s  string
			}{{r.Buttons.Up, "^"}, {r.Buttons.Down, "v"}, {r.Buttons.Both, "b"}, {r.Buttons.All, "a"}} {
				if on.on {
					b += on.s
				}
			}
			out = append(out, fmt.Sprintf("~%d %s", r.Hidden, b))
		case NoNewlineRow:
			s := `\`
			if r.NoNewlineOld {
				s += " L"
			}
			if r.NoNewlineNew {
				s += " R"
			}
			out = append(out, s)
		default:
			out = append(out, cell(r.Old, "L")+"|"+cell(r.New, "R"))
		}
	}
	return strings.Join(out, " ")
}

func TestGapsCollapseIntoSeparators(t *testing.T) {
	d := diffOf("hunks")
	rows := d.Layout(Split, nil, false)
	want := "~5 v 6|6 7|7 8|8 9|9 -10|+10 |+11 11|12 12|13 13|14 14|15 ~11 b 26|27 27|28 28|29 29|30 -30| 31|31 32|32 33|33 34|34 ~6 ^"
	if got := sketch(rows); got != want {
		t.Errorf("split rows\n got %s\nwant %s", got, want)
	}
	unified := sketch(d.Layout(Unified, nil, false))
	if !strings.Contains(unified, "9|9 -10| |+10 |+11 11|12") {
		t.Errorf("unified rows list deletions before additions: %s", unified)
	}
	for _, r := range rows {
		if r.Kind == LineRow && r.Old.Kind == Deleted && r.Old.Line == 10 && r.Old.Pair != 10 {
			t.Errorf("deleted 10 pairs with %d, want new 10", r.Old.Pair)
		}
		if r.Kind == LineRow && r.New.Kind == Added && r.New.Line == 11 && r.New.Pair != 0 {
			t.Errorf("added 11 pairs with %d, want none", r.New.Pair)
		}
	}
}

func TestGapsExpandInPlace(t *testing.T) {
	d := diffOf("hunks")
	e := Expansion{}
	// The first gap reveals from its bottom, over the first hunk.
	e.Expand(0, Down, 2)
	got := sketch(d.Layout(Split, e, false))
	if !strings.HasPrefix(got, "~3 v 4|4 5|5 6|6") {
		t.Errorf("first gap down 2: %s", got)
	}
	// A gap between hunks reveals from its top and its bottom.
	e.Expand(1, Up, 3)
	e.Expand(1, Down, 1)
	got = sketch(d.Layout(Split, e, false))
	if !strings.Contains(got, "14|15 15|16 16|17 17|18 ~7 b 25|26 26|27") {
		t.Errorf("middle gap up 3, down 1: %s", got)
	}
	// Revealing more than a gap holds shows it whole, without a separator.
	e.Expand(2, Up, ExpansionLines)
	got = sketch(d.Layout(Split, e, false))
	if !strings.HasSuffix(got, "34|34 35|35 36|36 37|37 38|38 39|39 40|40") {
		t.Errorf("last gap up: %s", got)
	}
	// The last gap reveals from its top only.
	e = Expansion{}
	e.Expand(2, Down, 3)
	if got := sketch(d.Layout(Split, e, false)); !strings.HasSuffix(got, "~6 ^") {
		t.Errorf("last gap down: %s", got)
	}
	// Expanding all shows every line.
	all := d.Layout(Unified, nil, true)
	for _, r := range all {
		if r.Kind == SeparatorRow {
			t.Fatalf("all expanded left a separator: %s", sketch(all))
		}
	}
	if n := len(all); n != 40+2 /* added lines */ {
		t.Errorf("all expanded unified rows = %d, want 42", n)
	}
}

func TestShortGapsShowAndLongGapsChunk(t *testing.T) {
	lines := func(n int) string {
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, "line %d\n", i+1)
		}
		return b.String()
	}
	// One unchanged line between hunks always shows.
	d := &Diff{Old: SplitLines(lines(5)), New: SplitLines(lines(5)), Hunks: []Hunk{
		{1, 2, 1, 2, []Block{{Context, 1, 2, 1, 2}}}, {4, 2, 4, 2, []Block{{Context, 4, 2, 4, 2}}},
	}, Gaps: []Gap{{1, 3, 3, 1}}}
	if got := sketch(d.Layout(Split, nil, false)); got != "1|1 2|2 3|3 4|4 5|5" {
		t.Errorf("one-line gap: %s", got)
	}
	// A gap over ExpansionLines offers Up, Down, and All.
	d = &Diff{Old: SplitLines(lines(300)), New: SplitLines(lines(300)), Hunks: []Hunk{
		{1, 1, 1, 1, []Block{{Context, 1, 1, 1, 1}}}, {300, 1, 300, 1, []Block{{Context, 300, 1, 300, 1}}},
	}, Gaps: []Gap{{1, 2, 2, 298}}}
	if got := sketch(d.Layout(Split, nil, false)); got != "1|1 ~298 ^va 300|300" {
		t.Errorf("long gap: %s", got)
	}
	e := Expansion{}
	e.Expand(1, Up, ExpansionLines)
	rows := d.Layout(Split, e, false)
	if rows[101].Kind != SeparatorRow || rows[101].Hidden != 198 || rows[100].Old.Line != 101 {
		t.Errorf("long gap up %d: row 100 %v, row 101 %v", ExpansionLines, rows[100], rows[101])
	}
}

func TestNoNewlineMarks(t *testing.T) {
	// The new side's last line lost its newline in a change.
	d := &Diff{Old: SplitLines("a\nb\n"), New: SplitLines("a\nc"), Hunks: []Hunk{
		{1, 2, 1, 2, []Block{{Context, 1, 1, 1, 1}, {Change, 2, 1, 2, 1}}},
	}}
	if got := sketch(d.Layout(Split, nil, false)); got != `1|1 -2|+2 \ R` {
		t.Errorf("split: %s", got)
	}
	if got := sketch(d.Layout(Unified, nil, false)); got != `1|1 -2| |+2 \ R` {
		t.Errorf("unified: %s", got)
	}
	// Both sides end without a newline in unchanged lines: one mark.
	d = &Diff{Old: SplitLines("x\na\nz"), New: SplitLines("y\na\nz"), Hunks: []Hunk{
		{1, 3, 1, 3, []Block{{Change, 1, 1, 1, 1}, {Context, 2, 2, 2, 2}}},
	}}
	if got := sketch(d.Layout(Unified, nil, false)); got != `-1| |+1 2|2 3|3 \ L R` {
		t.Errorf("unified context: %s", got)
	}
	// Deletions ending the old side, unified: the mark follows them.
	d = &Diff{Old: SplitLines("a\nb"), New: SplitLines("a\nc\n"), Hunks: []Hunk{
		{1, 2, 1, 2, []Block{{Context, 1, 1, 1, 1}, {Change, 2, 1, 2, 1}}},
	}}
	if got := sketch(d.Layout(Unified, nil, false)); got != `1|1 -2| \ L |+2` {
		t.Errorf("unified old end: %s", got)
	}
}
