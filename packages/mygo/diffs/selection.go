package diffs

import (
	"fmt"
	"regexp"
	"strconv"
)

// Point is a line (from 1) of a side.
type Point struct {
	Side Side
	Line int
}

// Selection is a run of lines from Start to End in the order they were
// chosen; its ends may lie on different sides.
type Selection struct {
	Start, End Point
}

var anchorPattern = regexp.MustCompile(`^#?([LR])([1-9]\d*)(?:-([LR])?([1-9]\d*))?$`)

func anchorSide(letter string) Side {
	if letter == "L" {
		return Old
	}
	return New
}

func sideLetter(s Side) string {
	if s == Old {
		return "L"
	}
	return "R"
}

// ParseAnchor reads GitHub diff fragments: L is the old side, R the new
// one. #R12, #R12-R20 (#R12-20 too), #L5, and #L5-R8 across sides.
func ParseAnchor(hash string) (Selection, bool) {
	m := anchorPattern.FindStringSubmatch(hash)
	if m == nil {
		return Selection{}, false
	}
	side, endSide := anchorSide(m[1]), anchorSide(m[1])
	if m[3] != "" {
		endSide = anchorSide(m[3])
	}
	first, err := strconv.Atoi(m[2])
	if err != nil {
		return Selection{}, false
	}
	second := first
	if m[4] != "" {
		if second, err = strconv.Atoi(m[4]); err != nil {
			return Selection{}, false
		}
	}
	if side != endSide {
		return Selection{Point{side, first}, Point{endSide, second}}, true
	}
	return Selection{Point{side, min(first, second)}, Point{side, max(first, second)}}, true
}

// Anchor formats the selection as a fragment without the leading #. One
// side reads top down; a range across sides keeps its order.
func (s Selection) Anchor() string {
	if s.Start.Side != s.End.Side {
		return fmt.Sprintf("%s%d-%s%d", sideLetter(s.Start.Side), s.Start.Line, sideLetter(s.End.Side), s.End.Line)
	}
	start, end := min(s.Start.Line, s.End.Line), max(s.Start.Line, s.End.Line)
	if start == end {
		return fmt.Sprintf("%s%d", sideLetter(s.Start.Side), start)
	}
	return fmt.Sprintf("%s%d-%s%d", sideLetter(s.Start.Side), start, sideLetter(s.Start.Side), end)
}

// Target is the lines a selection addresses: Start to End of one side's
// file.
type Target struct {
	Side       Side
	Start, End int
}

// Reference reads path:12 or path:12-20.
func (t Target) Reference(path string) string {
	if t.Start == t.End {
		return fmt.Sprintf("%s:%d", path, t.Start)
	}
	return fmt.Sprintf("%s:%d-%d", path, t.Start, t.End)
}

// span is a run of rows a diff can show: unchanged lines (both sides
// equal) or one change of deletions then additions.
type span struct {
	change               bool
	oldStart, newStart   int
	deletions, additions int
}

// spans lists the rows a diff can show, in order: the gaps as unchanged
// runs around the hunks' blocks.
func (d *Diff) spans() []span {
	var out []span
	gap := func(i int) {
		if g, ok := d.gap(i); ok {
			out = append(out, span{oldStart: g.OldStart, newStart: g.NewStart, deletions: g.Lines, additions: g.Lines})
		}
	}
	for i, h := range d.Hunks {
		gap(i)
		for _, b := range h.Blocks {
			out = append(out, span{change: b.Kind == Change, oldStart: b.OldStart, newStart: b.NewStart, deletions: b.OldCount, additions: b.NewCount})
		}
	}
	gap(len(d.Hunks))
	return out
}

type rowPoint struct {
	point Point
	span  span
	index int
	row   int
}

// locate finds a side's line among the shown rows: Unified lists a
// change's deletions before its additions; Split pairs them.
func locate(spans []span, p Point, style Style) (rowPoint, bool) {
	for i, s := range spans {
		start, count := s.newStart, s.additions
		if p.Side == Old {
			start, count = s.oldStart, s.deletions
		}
		if p.Line < start || p.Line >= start+count {
			continue
		}
		row := p.Line - start
		if s.change && p.Side == New && style == Unified {
			row += s.deletions
		}
		return rowPoint{p, s, i, row}, true
	}
	return rowPoint{}, false
}

func (a rowPoint) before(b rowPoint) bool {
	return a.index < b.index || a.index == b.index && a.row <= b.row
}

// newLinesBefore counts a span's new-side lines in rows before row.
func newLinesBefore(s span, row int, style Style) int {
	if !s.change {
		return row
	}
	if style == Unified {
		return max(0, row-s.deletions)
	}
	return min(row, s.additions)
}

// Target resolves a selection to the lines it addresses. One side keeps
// its own numbers; a range across sides covers the new lines between its
// ends, in the rows as shown.
func (d *Diff) Target(s Selection, style Style) (Target, bool) {
	spans := d.spans()
	first, ok1 := locate(spans, s.Start, style)
	second, ok2 := locate(spans, s.End, style)
	if !ok1 || !ok2 {
		return Target{}, false
	}
	if first.point.Side == second.point.Side {
		return Target{first.point.Side, min(first.point.Line, second.point.Line), max(first.point.Line, second.point.Line)}, true
	}
	top, bottom := first, second
	if !first.before(second) {
		top, bottom = second, first
	}
	// An old-side end moves inward to the nearest new line.
	start, end := top.point.Line, bottom.point.Line
	if top.point.Side == Old {
		start = top.span.newStart + newLinesBefore(top.span, top.row, style)
	}
	if bottom.point.Side == Old {
		end = bottom.span.newStart + newLinesBefore(bottom.span, bottom.row+1, style) - 1
	}
	return Target{New, start, end}, true
}

// HunkSelection selects hunk i's rows as shown, first to last, context
// included. Unchanged rows and additions address the new side; an end on
// deletions only addresses the old side, so the range can cross sides.
func (d *Diff) HunkSelection(i int, style Style) (Selection, bool) {
	if i < 0 || i >= len(d.Hunks) || len(d.Hunks[i].Blocks) == 0 {
		return Selection{}, false
	}
	h := d.Hunks[i]
	first, last := h.Blocks[0], h.Blocks[len(h.Blocks)-1]
	startsOld := first.Kind == Change && (style == Unified && first.OldCount > 0 || style == Split && first.NewCount == 0)
	endsOld := last.Kind == Change && (style == Unified && last.NewCount == 0 || style == Split && last.OldCount > last.NewCount)
	sel := Selection{Start: Point{New, h.NewStart}, End: Point{New, h.NewStart + h.NewCount - 1}}
	if startsOld {
		sel.Start = Point{Old, h.OldStart}
	}
	if endsOld {
		sel.End = Point{Old, h.OldStart + h.OldCount - 1}
	}
	return sel, true
}

// Cursor places a position among hunks: inside hunk Index, or before it
// (Index is the hunk count past the last).
type Cursor struct {
	Before bool
	Index  int
}

// CursorAt places a line of either side among the hunks.
func (d *Diff) CursorAt(p Point) Cursor {
	for i, h := range d.Hunks {
		start, count := h.NewStart, h.NewCount
		if p.Side == Old {
			start, count = h.OldStart, h.OldCount
		}
		if p.Line < start {
			return Cursor{Before: true, Index: i}
		}
		if p.Line < start+count {
			return Cursor{Index: i}
		}
	}
	return Cursor{Before: true, Index: len(d.Hunks)}
}

// AdjacentHunk steps to the next (direction 1) or previous (-1) hunk of
// count, wrapping within the file; false with no hunks.
func AdjacentHunk(count int, c Cursor, direction int) (int, bool) {
	if count <= 0 {
		return 0, false
	}
	next := c.Index - 1
	if direction == 1 {
		next = c.Index
		if !c.Before {
			next++
		}
	}
	return (next%count + count) % count, true
}

// HunkInFile steps as AdjacentHunk without wrapping: false past the
// file's ends, where a review moves on to another file.
func HunkInFile(count int, c Cursor, direction int) (int, bool) {
	next := c.Index - 1
	if direction == 1 {
		next = c.Index
		if !c.Before {
			next++
		}
	}
	return next, next >= 0 && next < count
}

// GapAt finds the gap holding a line of either side: its Hunk and the
// line's number on the new side.
func (d *Diff) GapAt(p Point) (gap, newLine int, ok bool) {
	for _, g := range d.Gaps {
		start := g.NewStart
		if p.Side == Old {
			start = g.OldStart
		}
		if p.Line >= start && p.Line < start+g.Lines {
			return g.Hunk, g.NewStart + p.Line - start, true
		}
	}
	return 0, 0, false
}

// Letter is the side's anchor letter: L for Old, R for New.
func (s Side) Letter() string { return sideLetter(s) }
