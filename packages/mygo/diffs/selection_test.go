package diffs

import "testing"

// These mirror apps/web/src/features/source-control/diff-line-selection.test.ts
// and diff-hunks.test.ts on the same line diffs.

func sel(startSide Side, start int, endSide Side, end int) Selection {
	return Selection{Point{startSide, start}, Point{endSide, end}}
}

func TestAnchorsReadAndWriteGitHubSides(t *testing.T) {
	for hash, want := range map[string]Selection{
		"#R4":    sel(New, 4, New, 4),
		"#L6-L4": sel(Old, 4, Old, 6),
		"#R4-6":  sel(New, 4, New, 6),
		"#L5-R8": sel(Old, 5, New, 8),
	} {
		if got, ok := ParseAnchor(hash); !ok || got != want {
			t.Errorf("ParseAnchor(%q) = %v, %v; want %v", hash, got, ok, want)
		}
	}
	for _, hash := range []string{"", "#", "#L0", "#X1", "#R1-", "#R1-X2", "#12", "#R99999999999999999999"} {
		if got, ok := ParseAnchor(hash); ok {
			t.Errorf("ParseAnchor(%q) = %v; want none", hash, got)
		}
	}
	for want, s := range map[string]Selection{
		"R4":    sel(New, 4, New, 4),
		"L4-L6": sel(Old, 6, Old, 4),
		"L5-R5": sel(Old, 5, New, 5),
	} {
		if got := s.Anchor(); got != want {
			t.Errorf("%v.Anchor() = %q; want %q", s, got, want)
		}
	}
}

func target(t *testing.T, d *Diff, s Selection, style Style) (Target, bool) {
	t.Helper()
	return d.Target(s, style)
}

func wantTarget(t *testing.T, d *Diff, s Selection, style Style, want Target) {
	t.Helper()
	if got, ok := target(t, d, s, style); !ok || got != want {
		t.Errorf("Target(%v, %d) = %v, %v; want %v", s, style, got, ok, want)
	}
}

func TestOneSideKeepsItsLineNumbers(t *testing.T) {
	d := diffOf("edited")
	for _, style := range []Style{Unified, Split} {
		wantTarget(t, d, sel(New, 3, New, 1), style, Target{New, 1, 3})
		wantTarget(t, d, sel(Old, 2, Old, 2), style, Target{Old, 2, 2})
		wantTarget(t, d, sel(Old, 3, Old, 5), style, Target{Old, 3, 5})
	}
}

func TestRangeAcrossSidesCoversNewLinesBetween(t *testing.T) {
	d := diffOf("edited")
	for _, style := range []Style{Unified, Split} {
		wantTarget(t, d, sel(Old, 2, New, 3), style, Target{New, 2, 3})
		wantTarget(t, d, sel(New, 3, Old, 2), style, Target{New, 2, 3})
		end := 2
		if style == Unified {
			end = 1
		}
		wantTarget(t, d, sel(New, 1, Old, 2), style, Target{New, 1, end})
		wantTarget(t, d, sel(Old, 2, New, 4), style, Target{New, 2, 4})
	}
	shrunk := diffOf("shrunk")
	wantTarget(t, shrunk, sel(New, 1, Old, 3), Unified, Target{New, 1, 1})
	wantTarget(t, shrunk, sel(New, 1, Old, 3), Split, Target{New, 1, 2})
	wantTarget(t, shrunk, sel(Old, 3, New, 3), Unified, Target{New, 2, 3})
	wantTarget(t, shrunk, sel(Old, 3, New, 3), Split, Target{New, 3, 3})
}

func TestLinesOutsideHunksKeepNumbers(t *testing.T) {
	d := diffOf("long")
	wantTarget(t, d, sel(New, 31, New, 31), Split, Target{New, 31, 31})
	wantTarget(t, d, sel(Old, 30, Old, 30), Split, Target{Old, 30, 30})
	wantTarget(t, d, sel(Old, 5, New, 20), Split, Target{New, 5, 20})
	wantTarget(t, d, sel(New, 20, Old, 30), Split, Target{New, 20, 31})
	for _, s := range []Selection{sel(New, 42, New, 42), sel(Old, 41, Old, 41)} {
		if got, ok := d.Target(s, Unified); ok {
			t.Errorf("Target(%v) = %v past the file's end", s, got)
		}
	}
	wantTarget(t, d, sel(New, 41, New, 41), Unified, Target{New, 41, 41})
	if got := (Target{New, 5, 20}).Reference("notes.txt"); got != "notes.txt:5-20" {
		t.Errorf("Reference = %q", got)
	}
	if got := (Target{Old, 7, 7}).Reference("a.go"); got != "a.go:7" {
		t.Errorf("Reference = %q", got)
	}
}

func TestAddedAndDeletedFilesHaveOneSide(t *testing.T) {
	added, deleted := diffOf("added"), diffOf("deleted")
	wantTarget(t, added, sel(New, 1, New, 2), Split, Target{New, 1, 2})
	if _, ok := added.Target(sel(Old, 1, Old, 1), Split); ok {
		t.Error("an added file's old side selected lines")
	}
	wantTarget(t, deleted, sel(Old, 2, Old, 2), Unified, Target{Old, 2, 2})
	if _, ok := deleted.Target(sel(New, 1, New, 1), Unified); ok {
		t.Error("a deleted file's new side selected lines")
	}
}

func wantHunk(t *testing.T, d *Diff, i int, style Style, want Selection) {
	t.Helper()
	if got, ok := d.HunkSelection(i, style); !ok || got != want {
		t.Errorf("HunkSelection(%d, %d) = %v, %v; want %v", i, style, got, ok, want)
	}
}

func TestHunkSelectsItsRowsAsShown(t *testing.T) {
	d := diffOf("hunks")
	for _, style := range []Style{Unified, Split} {
		wantHunk(t, d, 0, style, sel(New, 6, New, 15))
		wantHunk(t, d, 1, style, sel(New, 27, New, 34))
		wantHunk(t, diffOf("tail"), 0, style, sel(New, 1, Old, 5))
	}
	if _, ok := d.HunkSelection(2, Unified); ok {
		t.Error("a hunk past the last selected rows")
	}
	wantHunk(t, diffOf("deleted3"), 0, Unified, sel(Old, 1, Old, 3))
	wantHunk(t, diffOf("added3"), 0, Split, sel(New, 1, New, 3))
	head := diffOf("head")
	wantHunk(t, head, 0, Unified, sel(Old, 1, New, 5))
	wantHunk(t, head, 0, Split, sel(New, 1, New, 5))
	s, _ := head.HunkSelection(0, Unified)
	wantTarget(t, head, s, Unified, Target{New, 1, 5})
}

func TestCursorsAndSteps(t *testing.T) {
	d := diffOf("hunks")
	for p, want := range map[Point]Cursor{
		{New, 3}: {true, 0}, {New, 7}: {false, 0}, {New, 20}: {true, 1}, {Old, 30}: {false, 1}, {New, 38}: {true, 2},
	} {
		if got := d.CursorAt(p); got != want {
			t.Errorf("CursorAt(%v) = %v; want %v", p, got, want)
		}
	}
	type step struct {
		count     int
		cursor    Cursor
		direction int
		want      int
		ok        bool
	}
	for _, s := range []step{
		{2, Cursor{false, 0}, 1, 1, true}, {2, Cursor{false, 1}, 1, 0, true}, {2, Cursor{false, 0}, -1, 1, true},
		{2, Cursor{true, 1}, 1, 1, true}, {2, Cursor{true, 1}, -1, 0, true}, {2, Cursor{true, 0}, -1, 1, true},
		{2, Cursor{true, 2}, 1, 0, true}, {0, Cursor{true, 0}, 1, 0, false},
	} {
		if got, ok := AdjacentHunk(s.count, s.cursor, s.direction); got != s.want || ok != s.ok {
			t.Errorf("AdjacentHunk(%v) = %d, %v", s, got, ok)
		}
	}
	for _, s := range []step{
		{2, Cursor{false, 0}, 1, 1, true}, {2, Cursor{false, 1}, 1, 2, false}, {2, Cursor{false, 0}, -1, -1, false},
		{2, Cursor{true, 1}, 1, 1, true}, {2, Cursor{true, 1}, -1, 0, true}, {2, Cursor{true, 2}, 1, 2, false},
	} {
		if got, ok := HunkInFile(s.count, s.cursor, s.direction); ok != s.ok || ok && got != s.want {
			t.Errorf("HunkInFile(%v) = %d, %v", s, got, ok)
		}
	}
}

func TestGapsHoldTheLinesTheyCollapse(t *testing.T) {
	d := diffOf("hunks")
	for p, want := range map[Point][2]int{{New, 2}: {0, 2}, {New, 20}: {1, 20}, {Old, 20}: {1, 21}, {New, 40}: {2, 40}} {
		if gap, line, ok := d.GapAt(p); !ok || [2]int{gap, line} != want {
			t.Errorf("GapAt(%v) = %d, %d, %v; want %v", p, gap, line, ok, want)
		}
	}
	for _, p := range []Point{{New, 6}, {New, 15}, {New, 34}, {New, 41}, {Old, 30}} {
		if _, _, ok := d.GapAt(p); ok {
			t.Errorf("GapAt(%v) found a gap", p)
		}
	}
}
