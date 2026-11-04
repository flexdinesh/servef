package diffview

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/width"

	"github.com/flexdinesh/servef/packages/mygo/diffs"
)

// Span colors a run of a line, from the previous span's End (or the
// line's start) to End, a byte offset in the line without its ending.
// Spans come in order.
type Span struct {
	End    int
	Color  ui.Color
	Italic bool
	Bold   bool
}

// MaxHighlightLength is Pierre's tokenizeMaxLineLength: lines this long
// (UTF-16 code units) or longer show without the host's colors.
const MaxHighlightLength = 1000

// Bounds of the viewer's caches; past them a cache starts over.
const (
	maxEmphasisCache = 4096
	maxStyledCache   = 8192
)

// Theme is the viewer's look; DefaultTheme derives one from a ui.Theme.
type Theme struct {
	// Font and LineHeight set the code's type; a row is LineHeight high.
	Font       ui.Font
	LineHeight float32
	Background ui.Color
	Text       ui.Color
	LineNumber ui.Color
	// Lines of each kind tint their content, their number, and their
	// changed words, and mark their gutter with a bar.
	AddedLine, DeletedLine     ui.Color
	AddedNumber, DeletedNumber ui.Color
	AddedWord, DeletedWord     ui.Color
	AddedBar, DeletedBar       ui.Color
	// AddedNumberText and DeletedNumberText color changed lines' numbers.
	AddedNumberText, DeletedNumberText ui.Color
	// Empty slashes the side a split change leaves without a line.
	Empty ui.Color
	// Separator is the bar of collapsed lines, SeparatorText its text.
	Separator, SeparatorText ui.Color
	// Selected tints selected lines, mixed into each line's background
	// by SelectedLine and into its number's by SelectedNumber, as Pierre.
	Selected                     ui.Color
	SelectedLine, SelectedNumber float32
	// Border divides a split view's sides.
	Border ui.Color
}

// DefaultTheme mixes Pierre's tints from t: changed lines take a little
// of their status color, changed words more.
func DefaultTheme(t *ui.Theme) *Theme {
	line, word := float32(0.12), float32(0.3)
	selectedLine, selectedNumber := float32(0.18), float32(0.25)
	if t.Dark {
		line, selectedLine, selectedNumber = 0.2, 0.25, 0.4
	}
	bg := t.Surface
	if t.Background != (ui.Color{}) {
		bg = t.Background
	}
	return &Theme{
		Font:       ui.Font{Family: "monospace", Size: 13},
		LineHeight: 22,
		Background: bg, Text: t.Text, LineNumber: t.TextMuted,
		AddedLine: bg.Mix(t.Success, line), DeletedLine: bg.Mix(t.Danger, line),
		AddedNumber: bg.Mix(t.Success, line), DeletedNumber: bg.Mix(t.Danger, line),
		AddedWord: bg.Mix(t.Success, word), DeletedWord: bg.Mix(t.Danger, word),
		AddedBar: t.Success, DeletedBar: t.Danger,
		AddedNumberText: t.Success, DeletedNumberText: t.Danger,
		Empty:     bg.Mix(t.Text, 0.12),
		Separator: bg.Mix(t.Text, 0.05), SeparatorText: t.TextMuted,
		Selected: t.Accent, SelectedLine: selectedLine, SelectedNumber: selectedNumber,
		Border: t.Border,
	}
}

// State is a diff viewer's state, kept by the app across frames.
type State struct {
	// Style lays the diff out side by side or in one column.
	Style diffs.Style
	// Inline is how changed pairs emphasize what changed.
	Inline diffs.Inline
	// ExpandUnchanged shows every collapsed line.
	ExpandUnchanged bool
	// TabSize is the columns of a tab stop, 2 when 0.
	TabSize int
	// Theme is the look, DefaultTheme of the window's when nil.
	Theme *Theme
	// Spans colors line (from 1) of a side, as a highlighter does; lines
	// without spans show plain. It is called for the rows in view only,
	// and its results are kept until SpansVersion changes.
	Spans func(side diffs.Side, line int) []Span
	// SpansVersion tells the viewer Spans returns new colors.
	SpansVersion int
	// Run runs work off the UI thread, then the apply it returns on it, as
	// highlighting does; it computes word emphasis. Without it, emphasis
	// is computed while building the row.
	Run func(work func() (apply func()))
	// OnSelect is told of each selection the user makes: a click on a
	// line number, a Shift-click extending it, or a hunk step.
	OnSelect func(diffs.Selection)
	// Utility builds what shows in the gutter of the selection's last
	// line, as a button copying a reference to the lines.
	Utility func(c *ui.Context, s diffs.Selection)

	diff      *diffs.Diff
	expanded  diffs.Expansion
	sel       diffs.Selection
	selected  bool
	rows      []diffs.Row
	rowsKey   rowsKey
	emphasis  map[emphasisKey]*emphasisEntry
	styled    map[styledKey]styledEntry
	styledFor styledFor
	list      ui.ListState
	scrollTo  int
	charWidth float32
	charFont  ui.Font
	columns   [2]int
}

// emphasisKey is a pair by its texts, so a reread that keeps a pair keeps
// its emphasis.
type emphasisKey struct {
	old, new string
	inline   diffs.Inline
}

// emphasisEntry is a pair's emphasis, pending until computed.
type emphasisEntry struct {
	ready    bool
	old, new []diffs.Range
}

type styledKey struct {
	side diffs.Side
	line int
}

// styledEntry is a line's styled text; pending while its emphasis is.
type styledEntry struct {
	spans   []ui.Span
	pending bool
}

// styledFor is what styled text depends on besides its line.
type styledFor struct {
	diff         *diffs.Diff
	spansVersion int
	inline       diffs.Inline
	tab          int
	added, del   ui.Color
}

type rowsKey struct {
	diff   *diffs.Diff
	style  diffs.Style
	all    bool
	expand int
}

// SetDiff shows d, collapsing every gap again and clearing the
// selection; nil shows nothing.
func (s *State) SetDiff(d *diffs.Diff) {
	s.diff, s.expanded, s.selected, s.scrollTo = d, diffs.Expansion{}, false, -1
	s.rows, s.rowsKey = nil, rowsKey{}
	s.columns = [2]int{}
	if d != nil {
		s.columns = [2]int{columns(d.Old, s.tabSize()), columns(d.New, s.tabSize())}
	}
}

// Update shows d in place of the diff shown, as a reread of the same
// file: what the user expanded and selected stays, and so does the
// emphasis of pairs whose texts did not change.
func (s *State) Update(d *diffs.Diff) {
	s.diff, s.rows, s.rowsKey = d, nil, rowsKey{}
	s.columns = [2]int{}
	if d != nil {
		s.columns = [2]int{columns(d.Old, s.tabSize()), columns(d.New, s.tabSize())}
	}
}

// Diff returns the diff shown.
func (s *State) Diff() *diffs.Diff { return s.diff }

func (s *State) tabSize() int {
	if s.TabSize <= 0 {
		return 2
	}
	return s.TabSize
}

// Expand reveals ExpansionLines more lines of gap g (a Gap's Hunk) in
// direction dir.
func (s *State) Expand(g int, dir diffs.Direction) { s.expand(g, dir, diffs.ExpansionLines) }

// ExpandGap reveals every line of gap g.
func (s *State) ExpandGap(g int) { s.expand(g, diffs.Both, int(^uint(0)>>1)) }

func (s *State) expand(g int, dir diffs.Direction, n int) {
	if s.expanded == nil {
		s.expanded = diffs.Expansion{}
	}
	s.expanded.Expand(g, dir, n)
	s.rowsKey.expand = -1
}

// Rows returns the rows the viewer shows.
func (s *State) Rows() []diffs.Row {
	if s.diff == nil {
		return nil
	}
	key := rowsKey{s.diff, s.Style, s.ExpandUnchanged, 0}
	if s.rows == nil || s.rowsKey != key {
		s.rows, s.rowsKey = s.diff.Layout(s.Style, s.expanded, s.ExpandUnchanged), key
	}
	return s.rows
}

// Selection returns the selected lines.
func (s *State) Selection() (diffs.Selection, bool) { return s.sel, s.selected }

// Select selects lines, revealing the gaps their ends are in, and
// scrolls to them.
func (s *State) Select(sel diffs.Selection) {
	if s.diff == nil {
		return
	}
	for _, p := range []diffs.Point{sel.Start, sel.End} {
		if g, _, ok := s.diff.GapAt(p); ok {
			s.ExpandGap(g)
		}
	}
	s.sel, s.selected = sel, true
	if row, ok := s.rowOf(sel.Start); ok {
		s.scrollTo = row
	}
}

// ClearSelection clears the selected lines.
func (s *State) ClearSelection() { s.selected = false }

// StepHunk selects the next hunk (direction 1) or the previous one (-1)
// from the selection, or from the rows in view, wrapping within the file,
// and reports whether there was one.
func (s *State) StepHunk(direction int) bool {
	if s.diff == nil {
		return false
	}
	cursor := diffs.Cursor{Before: true, Index: len(s.diff.Hunks)}
	if s.selected {
		cursor = s.diff.CursorAt(s.sel.Start)
	} else {
		first, _ := s.list.Visible()
		for i, r := range s.Rows() {
			if i >= first && r.Hunk >= 0 {
				cursor = diffs.Cursor{Before: true, Index: r.Hunk}
				if i > 0 && s.rows[i-1].Hunk == r.Hunk {
					// The hunk's top is above the view.
					cursor.Index++
				}
				break
			}
		}
		if direction == 1 && cursor.Index == len(s.diff.Hunks) && first == 0 {
			cursor.Index = 0
		}
	}
	i, ok := diffs.AdjacentHunk(len(s.diff.Hunks), cursor, direction)
	if !ok {
		return false
	}
	sel, ok := s.diff.HunkSelection(i, s.Style)
	if !ok {
		return false
	}
	s.Select(sel)
	if s.OnSelect != nil {
		s.OnSelect(sel)
	}
	return true
}

// rowOf finds the row showing a side's line.
func (s *State) rowOf(p diffs.Point) (int, bool) {
	for i, r := range s.Rows() {
		if r.Kind != diffs.LineRow {
			continue
		}
		c := r.New
		if p.Side == diffs.Old {
			c = r.Old
		}
		if c.Kind != diffs.Empty && c.Line == p.Line {
			return i, true
		}
	}
	return 0, false
}

// selectedRows returns the rows the selection spans, and its sides.
func (s *State) selectedRows() (from, to int, ok bool) {
	if !s.selected {
		return 0, 0, false
	}
	a, ok1 := s.rowOf(s.sel.Start)
	b, ok2 := s.rowOf(s.sel.End)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return min(a, b), max(a, b), true
}

// cellSelected reports whether the cell on side of row i is selected: on
// one side, that side's lines between the ends, and in split, as Pierre,
// both cells of their rows; across sides, every row between them.
func (s *State) cellSelected(i int, side diffs.Side, from, to int) bool {
	if i < from || i > to {
		return false
	}
	if s.sel.Start.Side != s.sel.End.Side {
		return true
	}
	r := s.rows[i]
	if s.Style == diffs.Split {
		side = s.sel.Start.Side
	} else if r.Old.Kind == diffs.Unchanged {
		// A unified unchanged row numbers the new side.
		return s.sel.Start.Side == diffs.New
	}
	c := r.New
	if side == diffs.Old {
		c = r.Old
	}
	return c.Kind != diffs.Empty && s.sel.Start.Side == side
}

// emphasized returns the changed runs of a pair, old line o and new n,
// and whether they are known yet: with Run they are computed off the UI
// thread, once per pair of texts.
func (s *State) emphasized(o, n int) (oldRanges, newRanges []diffs.Range, ready bool) {
	key := emphasisKey{s.diff.Text(diffs.Old, o), s.diff.Text(diffs.New, n), s.Inline}
	if e, ok := s.emphasis[key]; ok {
		return e.old, e.new, e.ready
	}
	if s.emphasis == nil || len(s.emphasis) >= maxEmphasisCache {
		s.emphasis = map[emphasisKey]*emphasisEntry{}
	}
	e := &emphasisEntry{}
	s.emphasis[key] = e
	if s.Run == nil {
		e.old, e.new = diffs.Emphasis(key.old, key.new, key.inline)
		e.ready = true
		return e.old, e.new, true
	}
	s.Run(func() func() {
		old, new := diffs.Emphasis(key.old, key.new, key.inline)
		return func() { e.old, e.new, e.ready = old, new, true }
	})
	return nil, nil, false
}

// columns is the widest line's columns, tabs expanded.
func columns(lines []string, tab int) int {
	widest := 0
	for _, l := range lines {
		widest = max(widest, expandedWidth(strings.TrimRight(l, "\r\n"), tab))
	}
	return widest
}

func expandedWidth(s string, tab int) int {
	col := 0
	for _, r := range s {
		if r == '\t' {
			col += tab - col%tab
			continue
		}
		col += runeColumns(r)
	}
	return col
}

// runeColumns is the columns r takes in a monospace font: two for East
// Asian wide and fullwidth characters (most emoji among them), none for
// combining marks and format characters, else one.
func runeColumns(r rune) int {
	switch {
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

const (
	barWidth       = 4
	separatorRow   = 36
	contentPadding = 12
)

// View builds the viewer, as tall and wide as it is given: a virtualized
// list of rows, scrolling sideways when lines are wider. F7 and Shift+F7
// step through hunks while it has the focus.
func View(c *ui.Context, s *State) ui.Element {
	th := s.Theme
	if th == nil {
		th = DefaultTheme(c.Theme())
	}
	if s.charWidth == 0 || s.charFont != th.Font {
		s.charFont, s.charWidth = th.Font, charWidth(th.Font)
	}
	rows := s.Rows()
	digits := len(fmt.Sprint(max(len(s.diffLines(diffs.Old)), len(s.diffLines(diffs.New)), 1)))
	numberWidth := float32(max(digits, 1))*s.charWidth + 2*contentPadding
	side := func(n int) float32 {
		return barWidth + numberWidth + contentPadding + float32(n)*s.charWidth + contentPadding
	}
	width := side(max(s.columns[0], s.columns[1]))
	if s.Style == diffs.Split {
		width = side(s.columns[0]) + 1 + side(s.columns[1])
	}
	from, to, hasSel := s.selectedRows()
	root := ui.Column(c).Focusable().Background(th.Background)
	if root.Shortcut(0, ui.KeyF7) {
		s.StepHunk(1)
	}
	if root.Shortcut(ui.Shift, ui.KeyF7) {
		s.StepHunk(-1)
	}
	if root.Shortcut(0, ui.KeyEscape) {
		s.ClearSelection()
	}
	if s.scrollTo >= 0 && s.scrollTo < len(rows) {
		first, last := s.list.Visible()
		if s.scrollTo < first || s.scrollTo > last || last == first {
			s.list.ScrollTo(max(0, s.scrollTo-max(1, (last-first)/3)), ui.Start)
		}
		s.scrollTo = -1
	}
	root.Children(func() {
		ui.List(c, &s.list, len(rows), func(i int) {
			s.row(c, th, i, numberWidth, hasSel, from, to)
		}).Grow(1).RowWidth(width).Label("Diff")
	})
	return root
}

func (s *State) diffLines(side diffs.Side) []string {
	if s.diff == nil {
		return nil
	}
	return s.diff.Lines(side)
}

// charWidth measures a column of the code's font.
func charWidth(f ui.Font) float32 {
	g := ui.Shape("0000000000", f)
	if len(g) == 0 {
		return f.Size * 0.6
	}
	last := g[len(g)-1]
	return (last.X + last.Advance) / 10
}

// row builds row i.
func (s *State) row(c *ui.Context, th *Theme, i int, numberWidth float32, hasSel bool, from, to int) {
	r := s.rows[i]
	switch r.Kind {
	case diffs.SeparatorRow:
		s.separator(c, th, r)
		return
	case diffs.NoNewlineRow:
		ui.Row(c).Height(th.LineHeight).Children(func() {
			if s.Style == diffs.Split {
				s.noNewline(c, th, r.NoNewlineOld, numberWidth, diffs.Deleted)
				ui.Box(c).Width(1).FillHeight().Background(th.Border)
				s.noNewline(c, th, r.NoNewlineNew, numberWidth, diffs.Added)
				return
			}
			kind := diffs.Unchanged
			switch {
			case r.NoNewlineOld && !r.NoNewlineNew:
				kind = diffs.Deleted
			case r.NoNewlineNew && !r.NoNewlineOld:
				kind = diffs.Added
			}
			s.noNewline(c, th, true, numberWidth, kind)
		})
		return
	}
	selected := func(side diffs.Side) bool { return hasSel && s.cellSelected(i, side, from, to) }
	last := hasSel && i == to && s.Utility != nil
	ui.Row(c).Height(th.LineHeight).Children(func() {
		if s.Style == diffs.Split {
			s.cell(c, th, diffs.Old, r.Old, numberWidth, selected(diffs.Old), last && s.utilitySide() == diffs.Old)
			ui.Box(c).Width(1).FillHeight().Background(th.Border)
			s.cell(c, th, diffs.New, r.New, numberWidth, selected(diffs.New), last && s.utilitySide() == diffs.New)
			return
		}
		side, cell := diffs.New, r.New
		if r.New.Kind == diffs.Empty {
			side, cell = diffs.Old, r.Old
		}
		s.cell(c, th, side, cell, numberWidth, selected(side), last)
	})
}

// utilitySide is the side of the selection's last row the utility shows
// on.
func (s *State) utilitySide() diffs.Side {
	a, _ := s.rowOf(s.sel.Start)
	b, _ := s.rowOf(s.sel.End)
	if b >= a {
		return s.sel.End.Side
	}
	return s.sel.Start.Side
}

// cell builds a side of a row: the change bar, the line number, which
// selects lines, and the code.
func (s *State) cell(c *ui.Context, th *Theme, side diffs.Side, cell diffs.Cell, numberWidth float32, selected, utility bool) {
	lineBg, numberBg, bar, number := th.Background, th.Background, ui.Color{}, th.LineNumber
	switch cell.Kind {
	case diffs.Empty:
		// As Pierre: a plain gutter beside slashes over the code.
		ui.Row(c).Grow(1).Basis(0).FillHeight().Clip().Background(th.Background).Children(func() {
			ui.Box(c).Width(barWidth + numberWidth).FillHeight()
			ui.Box(c).Grow(1).Basis(0).FillHeight().Background(th.Background).Stripes(th.Empty, 1, 6, 45)
		})
		return
	case diffs.Added:
		lineBg, numberBg, bar, number = th.AddedLine, th.AddedNumber, th.AddedBar, th.AddedNumberText
	case diffs.Deleted:
		lineBg, numberBg, bar, number = th.DeletedLine, th.DeletedNumber, th.DeletedBar, th.DeletedNumberText
	}
	if selected {
		lineBg, numberBg = lineBg.Mix(th.Selected, th.SelectedLine), numberBg.Mix(th.Selected, th.SelectedNumber)
	}
	ui.Row(c).Grow(1).Basis(0).FillHeight().Clip().Background(lineBg).Children(func() {
		b := ui.Box(c).Width(barWidth).FillHeight()
		if bar != (ui.Color{}) {
			b.Background(bar)
			if cell.Kind == diffs.Deleted {
				b.Stripes(lineBg, 1, 1, 90)
			}
		}
		label := fmt.Sprintf("%s%d", side.Letter(), cell.Line)
		gutter := ui.ButtonBase(c).Key(label).Label(label).Width(numberWidth).FillHeight().Background(numberBg).Cursor(ui.CursorPointer)
		gutter.Children(func() {
			if utility {
				ui.Row(c).Absolute().Left(0).Top(0).FillHeight().AlignItems(ui.Center).Children(func() { s.Utility(c, s.sel) })
			}
			ui.Text(c, fmt.Sprint(cell.Line)).Font(th.Font.Family).FontSize(th.Font.Size).FixedLineHeight(th.LineHeight).
				TextColor(number).TextAlign(ui.End).FillWidth().PaddingX(contentPadding).NoWrap()
		})
		if gutter.Clicked() {
			s.click(diffs.Point{Side: side, Line: cell.Line}, gutter.ClickModifiers()&ui.Shift != 0)
		}
		ui.RichText(c, s.spans(th, side, cell)...).Font(th.Font.Family).FontSize(th.Font.Size).FixedLineHeight(th.LineHeight).
			TextColor(th.Text).NoWrap().PaddingX(contentPadding).Grow(1)
	})
}

// click selects a line, or extends the selection to it with Shift.
func (s *State) click(p diffs.Point, extend bool) {
	sel := diffs.Selection{Start: p, End: p}
	if extend && s.selected {
		sel = diffs.Selection{Start: s.sel.Start, End: p}
	}
	s.sel, s.selected = sel, true
	if s.OnSelect != nil {
		s.OnSelect(sel)
	}
}

// spans styles a cell's code: the host's colors split where changed
// words start and end, tabs expanded. Lines keep their styled text until
// the diff, the host's colors, or the look changes.
func (s *State) spans(th *Theme, side diffs.Side, cell diffs.Cell) []ui.Span {
	tab := s.tabSize()
	now := styledFor{s.diff, s.SpansVersion, s.Inline, tab, th.AddedWord, th.DeletedWord}
	if s.styled == nil || s.styledFor != now || len(s.styled) >= maxStyledCache {
		s.styled, s.styledFor = map[styledKey]styledEntry{}, now
	}
	key := styledKey{side, cell.Line}
	if e, ok := s.styled[key]; ok && !e.pending {
		return e.spans
	}
	text := s.diff.Text(side, cell.Line)
	var colors []Span
	if s.Spans != nil && utf16Len(text) < MaxHighlightLength {
		colors = s.Spans(side, cell.Line)
	}
	var words []diffs.Range
	var wordBg ui.Color
	ready := true
	if cell.Pair > 0 && cell.Kind != diffs.Unchanged {
		o, n := cell.Line, cell.Pair
		if side == diffs.New {
			o, n = n, o
		}
		var oldRanges, newRanges []diffs.Range
		oldRanges, newRanges, ready = s.emphasized(o, n)
		words, wordBg = oldRanges, th.DeletedWord
		if side == diffs.New {
			words, wordBg = newRanges, th.AddedWord
		}
	}
	out := styleLine(text, colors, words, wordBg, tab)
	s.styled[key] = styledEntry{out, !ready}
	return out
}

// styleLine cuts text where a color or an emphasized run starts or ends,
// walking both in order once.
func styleLine(text string, colors []Span, words []diffs.Range, wordBg ui.Color, tab int) []ui.Span {
	n := len(text)
	clamp := func(v int) int { return min(max(v, 0), n) }
	out := make([]ui.Span, 0, len(colors)+2*len(words)+1)
	col, at, ci, wi := 0, 0, 0, 0
	for at < n {
		for ci < len(colors) && clamp(colors[ci].End) <= at {
			ci++
		}
		for wi < len(words) && clamp(words[wi].To) <= at {
			wi++
		}
		next, inWord := n, false
		if ci < len(colors) {
			next = clamp(colors[ci].End)
		}
		if wi < len(words) {
			if from := clamp(words[wi].From); at < from {
				next = min(next, from)
			} else {
				next, inWord = min(next, clamp(words[wi].To)), true
			}
		}
		span := ui.Span{Text: expandTabs(text[at:next], &col, tab)}
		if ci < len(colors) {
			span.Color, span.Italic = colors[ci].Color, colors[ci].Italic
			if colors[ci].Bold {
				span.Weight = 700
			}
		}
		if inWord {
			span.Background = wordBg
		}
		out = append(out, span)
		at = next
	}
	if len(out) == 0 {
		out = append(out, ui.Span{Text: ""})
	}
	return out
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// expandTabs replaces tabs with spaces to the next stop, from display
// column *col, which it advances.
func expandTabs(s string, col *int, tab int) string {
	if !strings.Contains(s, "\t") {
		for _, r := range s {
			*col += runeColumns(r)
		}
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r == '\t' {
			n := tab - *col%tab
			b.WriteString(strings.Repeat(" ", n))
			*col += n
			continue
		}
		b.WriteRune(r)
		*col += runeColumns(r)
	}
	return b.String()
}

// noNewline builds a side of a no-newline row.
func (s *State) noNewline(c *ui.Context, th *Theme, marked bool, numberWidth float32, kind diffs.CellKind) {
	if !marked {
		ui.Box(c).Grow(1).Basis(0).FillHeight().Background(th.Background)
		return
	}
	bg := th.Background
	switch kind {
	case diffs.Added:
		bg = th.AddedLine
	case diffs.Deleted:
		bg = th.DeletedLine
	}
	ui.Row(c).Grow(1).Basis(0).FillHeight().Clip().Background(bg).Children(func() {
		ui.Box(c).Width(barWidth + numberWidth).FillHeight()
		ui.Text(c, "No newline at end of file").FontSize(th.Font.Size - 1).FixedLineHeight(th.LineHeight).
			TextColor(th.SeparatorText).NoWrap().PaddingX(contentPadding)
	})
}

// separator builds the bar of a gap's collapsed lines, with Pierre's
// expand buttons; Shift expands all of them.
func (s *State) separator(c *ui.Context, th *Theme, r diffs.Row) {
	t := c.Theme()
	expand := func(dir diffs.Direction) {
		if c.Modifiers()&ui.Shift != 0 {
			s.ExpandGap(r.Gap)
			return
		}
		s.Expand(r.Gap, dir)
	}
	// Split shows a bar per side, as Pierre: the old side's holds the
	// buttons.
	ui.Row(c).Height(separatorRow).Padding(4, 8).Gap(8).Children(func() {
		defer func() {
			if s.Style == diffs.Split {
				ui.Box(c).Grow(1).Basis(0).FillHeight().Radius(6).Background(th.Separator)
			}
		}()
		ui.Row(c).Grow(1).Basis(0).FillHeight().AlignItems(ui.Center).Gap(2).Padding(0, 4).Radius(6).Background(th.Separator).Children(func() {
			chevron := func(label string, dir diffs.Direction) {
				b := ui.ButtonBase(c).Key(label).Label(label).Tooltip(label).Size(24, 24).Radius(4).Cursor(ui.CursorPointer)
				if b.Hovered() {
					b.Background(th.Text.Alpha(0.08))
				}
				b.Draw(func(p *ui.Painter, rect ui.Rect) {
					cx, cy, d := rect.X+rect.W/2, rect.Y+rect.H/2, float32(3.5)
					var path ui.Path
					switch dir {
					case diffs.Up:
						path.MoveTo(cx-d, cy+d/2).LineTo(cx, cy-d/2).LineTo(cx+d, cy+d/2)
					case diffs.Down:
						path.MoveTo(cx-d, cy-d/2).LineTo(cx, cy+d/2).LineTo(cx+d, cy-d/2)
					default:
						path.MoveTo(cx-d, cy-1.5).LineTo(cx, cy-1.5-d).LineTo(cx+d, cy-1.5)
						path.MoveTo(cx-d, cy+1.5).LineTo(cx, cy+1.5+d).LineTo(cx+d, cy+1.5)
					}
					p.StrokePath(&path, 1.5, th.SeparatorText)
				})
				if b.Clicked() {
					expand(dir)
				}
			}
			if r.Buttons.Up {
				chevron("Expand up", diffs.Up)
			}
			if r.Buttons.Down {
				chevron("Expand down", diffs.Down)
			}
			if r.Buttons.Both {
				chevron("Expand", diffs.Both)
			}
			label := diffs.Unmodified(r.Hidden)
			text := ui.ButtonBase(c).Key("lines").Label(label).Padding(2, 6).Radius(4).Cursor(ui.CursorPointer)
			text.Children(func() {
				ui.Text(c, label).FontSize(max(t.FontSize-1, 11)).TextColor(th.SeparatorText).NoWrap()
			})
			if text.Clicked() {
				expand(diffs.Both)
			}
			if r.Buttons.All {
				ui.Spacer(c)
				all := ui.ButtonBase(c).Key("all").Label("Expand all").Padding(2, 6).Radius(4).Cursor(ui.CursorPointer)
				all.Children(func() {
					ui.Text(c, "Expand all").FontSize(max(t.FontSize-1, 11)).TextColor(th.SeparatorText).NoWrap()
				})
				if all.Clicked() {
					s.ExpandGap(r.Gap)
				}
			}
		})
	})
}
