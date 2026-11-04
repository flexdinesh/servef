package diffs

import "fmt"

// Pierre's defaults: gaps this short always show; a click expands this
// many lines.
const (
	collapsedThreshold = 1
	// ExpansionLines is how many lines an expand button reveals.
	ExpansionLines = 100
)

// CellKind is what a cell of a row shows.
type CellKind uint8

const (
	// Empty is the filler beside a split change's longer side, or the
	// cell of a side a unified row does not show.
	Empty CellKind = iota
	// Unchanged is a line of both sides.
	Unchanged
	// Deleted is a line of the old side only.
	Deleted
	// Added is a line of the new side only.
	Added
)

// Cell is one side of a row. Line is its line (from 1), 0 for Empty; Pair
// is the line of the other side it pairs with in a change, whose words
// emphasis compares, 0 for none.
type Cell struct {
	Kind CellKind
	Line int
	Pair int
}

// RowKind tells lines from the rows between them.
type RowKind uint8

const (
	// LineRow shows lines.
	LineRow RowKind = iota
	// SeparatorRow stands for collapsed unchanged lines.
	SeparatorRow
	// NoNewlineRow marks the line above as its file's last, without a
	// newline.
	NoNewlineRow
)

// Direction is where an expand button reveals collapsed lines.
type Direction uint8

const (
	// Up reveals the lines at the gap's top, under the hunk before.
	Up Direction = iota
	// Down reveals the lines at the gap's bottom, over the hunk after.
	Down
	// Both reveals both ends.
	Both
)

// Buttons are a separator's expand buttons, as Pierre shows them: one
// (Down at the file's top, Up at its end, Both between hunks) for a gap of
// at most ExpansionLines, else Up and Down where they apply and All.
type Buttons struct {
	Up, Down, Both, All bool
}

// Row is a row of the laid out diff. A split row shows Old on the left
// and New on the right; a unified row shows one line: an unchanged row
// fills both cells and numbers the new side, a deleted row Old, an added
// row New. Hunk is the hunk a row of lines belongs to, -1 for a gap's
// lines and separators.
type Row struct {
	Kind     RowKind
	Old, New Cell
	Hunk     int
	// Gap is a separator's gap (its Hunk), Hidden how many lines it still
	// collapses, and Buttons what it offers.
	Gap     int
	Hidden  int
	Buttons Buttons
	// NoNewline marks, on a NoNewlineRow, the sides it marks.
	NoNewlineOld, NoNewlineNew bool
}

// Expansion is what of each gap the user revealed, by the gap's Hunk:
// lines from its top and from its bottom.
type Expansion map[int]Reveal

// Reveal is how many lines of a gap show from its top and its bottom.
type Reveal struct{ Top, Bottom int }

// Expand reveals n more lines of gap g in direction dir.
func (e Expansion) Expand(g int, dir Direction, n int) {
	r := e[g]
	if dir == Up || dir == Both {
		r.Top = sat(r.Top, n)
	}
	if dir == Down || dir == Both {
		r.Bottom = sat(r.Bottom, n)
	}
	e[g] = r
}

func sat(a, b int) int {
	if s := a + b; s >= a {
		return s
	}
	return int(^uint(0) >> 1)
}

// Layout lays d out as rows in style: its gaps, collapsed but for what
// expanded reveals (all of them with all), around its hunks' rows.
func (d *Diff) Layout(style Style, expanded Expansion, all bool) []Row {
	var rows []Row
	for i := 0; i <= len(d.Hunks); i++ {
		if g, ok := d.gap(i); ok {
			rows = d.appendGap(rows, g, expanded[g.Hunk], all)
		}
		if i < len(d.Hunks) {
			rows = d.appendHunk(rows, i, style)
		}
	}
	return rows
}

// appendGap appends a gap's revealed top, its separator while lines stay
// collapsed, and its revealed bottom. The gap after the last hunk reveals
// from its top only.
func (d *Diff) appendGap(rows []Row, g Gap, r Reveal, all bool) []Row {
	first, last := g.Hunk == 0, g.Hunk == len(d.Hunks)
	top, bottom := min(r.Top, g.Lines), min(r.Bottom, g.Lines)
	if last {
		bottom = 0
	}
	if all || g.Lines <= collapsedThreshold || top+bottom >= g.Lines {
		top, bottom = g.Lines, 0
	}
	context := func(k int) Row {
		return Row{Kind: LineRow, Hunk: -1, Old: Cell{Kind: Unchanged, Line: g.OldStart + k}, New: Cell{Kind: Unchanged, Line: g.NewStart + k}}
	}
	for k := range top {
		rows = append(rows, context(k))
	}
	if hidden := g.Lines - top - bottom; hidden > 0 {
		sep := Row{Kind: SeparatorRow, Hunk: -1, Gap: g.Hunk, Hidden: hidden}
		switch {
		case g.Lines <= ExpansionLines && first:
			sep.Buttons.Down = true
		case g.Lines <= ExpansionLines && last:
			sep.Buttons.Up = true
		case g.Lines <= ExpansionLines:
			sep.Buttons.Both = true
		default:
			sep.Buttons = Buttons{Up: !first, Down: !last, All: true}
		}
		rows = append(rows, sep)
	}
	for k := g.Lines - bottom; k < g.Lines; k++ {
		rows = append(rows, context(k))
	}
	return rows
}

// appendHunk appends hunk i's rows and its no-newline marks, placed as
// Pierre places them.
func (d *Diff) appendHunk(rows []Row, i int, style Style) []Row {
	h := d.Hunks[i]
	if len(h.Blocks) == 0 {
		return rows
	}
	// A side reaching its last line without a newline is marked; a mark
	// after unchanged lines covers both sides.
	oldEnd := d.endsWithoutNewline(Old, h.OldStart, h.OldCount)
	newEnd := d.endsWithoutNewline(New, h.NewStart, h.NewCount)
	lastBlock := h.Blocks[len(h.Blocks)-1]
	endsInContext := lastBlock.Kind == Context
	markOld, markNew := oldEnd || endsInContext && newEnd, newEnd || endsInContext && oldEnd
	mark := func(old, new bool) Row {
		return Row{Kind: NoNewlineRow, Hunk: i, NoNewlineOld: old, NoNewlineNew: new}
	}
	for b, block := range h.Blocks {
		lastOfHunk := b == len(h.Blocks)-1
		if block.Kind == Context {
			for k := range block.OldCount {
				rows = append(rows, Row{Kind: LineRow, Hunk: i, Old: Cell{Kind: Unchanged, Line: block.OldStart + k}, New: Cell{Kind: Unchanged, Line: block.NewStart + k}})
			}
			continue
		}
		paired := min(block.OldCount, block.NewCount)
		pair := func(k, start int) int {
			if k < paired {
				return start + k
			}
			return 0
		}
		deleted := func(k int) Cell {
			return Cell{Kind: Deleted, Line: block.OldStart + k, Pair: pair(k, block.NewStart)}
		}
		added := func(k int) Cell {
			return Cell{Kind: Added, Line: block.NewStart + k, Pair: pair(k, block.OldStart)}
		}
		if style == Split {
			for k := range max(block.OldCount, block.NewCount) {
				row := Row{Kind: LineRow, Hunk: i}
				if k < block.OldCount {
					row.Old = deleted(k)
				}
				if k < block.NewCount {
					row.New = added(k)
				}
				rows = append(rows, row)
			}
			continue
		}
		for k := range block.OldCount {
			rows = append(rows, Row{Kind: LineRow, Hunk: i, Old: deleted(k)})
		}
		if lastOfHunk && markOld && block.OldCount > 0 {
			rows = append(rows, mark(true, false))
		}
		for k := range block.NewCount {
			rows = append(rows, Row{Kind: LineRow, Hunk: i, New: added(k)})
		}
		if lastOfHunk && markNew && block.NewCount > 0 {
			rows = append(rows, mark(false, true))
		}
	}
	if (style == Split || endsInContext) && (markOld || markNew) {
		rows = append(rows, mark(markOld, markNew))
	}
	return rows
}

// Unmodified reads "12 unmodified lines", as Pierre's separators.
func Unmodified(n int) string {
	if n == 1 {
		return "1 unmodified line"
	}
	return fmt.Sprintf("%d unmodified lines", n)
}
