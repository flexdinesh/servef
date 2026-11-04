package trees

import (
	"reflect"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Decoration is a row's data from the host: its git status (lane letter
// and tint), trailing parts (line stats, a dot, notes), and marks. It
// knows nothing of the host's domain.
type Decoration struct {
	Status Status
	// Parts are the decoration lane's text runs, right-aligned before the
	// git lane; a zero Color takes the theme's Muted ink.
	Parts []Part
	// Viewed checks the row and mutes its name and icon, unless selected.
	Viewed bool
	// Dimmed mutes the row's name and halves its icon, as an ignored row.
	Dimmed bool
	// Title is the decoration lane's tooltip.
	Title string
}

// Part is a run of a decoration's text.
type Part struct {
	Text  string
	Color ui.Color
}

// Props are one frame's data for View.
type Props struct {
	Tree  *Tree
	Theme Theme
	// Decorations by row path; nil turns the git lane off. Directories
	// holding a changed (non-ignored) path get Pierre's dot, and an
	// ignored directory's rows are all ignored.
	Decorations map[string]Decoration
	// Match returns byte ranges of a row's name to emphasize, as the
	// matches of a filter; nil emphasizes none.
	Match func(path, name string) [][2]int
	// Menu builds a row's context menu; nil gives rows none.
	Menu func(m *ui.Menu, path string)
	// Label names the tree for assistive technology.
	Label string
}

// Result is what one frame's input did.
type Result struct {
	ui.Element
	// Activated is the file a click, Enter, or Space selected, "" for
	// none.
	Activated string
	// Submitted reports that Enter or a double click activated it, as
	// asking to move on to the file (FilesTree's Enter).
	Submitted bool
	// Toggled is the directory a click, Enter, Space, or arrow opened or
	// closed, "" for none.
	Toggled string
	// Moved reports that the keyboard cursor moved.
	Moved bool
}

// View draws the tree and handles its input. Assistive technology sees a
// tree of items with their levels, open states, and the selected row (the
// host's open document), while the keyboard cursor is the tree's active
// item.
func View(c *ui.Context, s *State, p Props) Result {
	t, th := p.Tree, p.Theme
	if th.RowHeight == 0 {
		th = defaultThemes[0]
		if c.Theme().Dark {
			th = defaultThemes[1]
		}
	}
	if t == nil {
		t = New(nil, Options{})
	}
	s.flatten(t)
	if s.scrollTo != "" {
		if i := s.index(s.scrollTo); i >= 0 {
			s.list.ScrollIntoView(i)
		}
		s.scrollTo = ""
	}
	// Rows are their paths, so scroll position and row state follow them
	// across refreshes.
	s.list.Key = func(i int) any { return s.rows[i].path }
	marks := s.decorate(t, p.Decorations)
	var res Result

	ui.Themed(c, listTheme(c.Theme(), th)).Background(th.Background).Grow(1).FillWidth().Children(func() {
		before := s.cursor
		var active ui.Element
		hasActive := false
		list := ui.List(c, &s.list, len(s.rows), func(i int) {
			r := s.rows[i]
			cursor := i == s.cursor
			focused := cursor && s.list.FocusWithin(c)
			e := drawRow(c, t, th, r, marks, p, s.Selected == r.path, focused, s.hovered)
			if cursor {
				active, hasActive = e, true
			}
			if e.Clicked() || e.DoubleClicked() {
				file, toggled := s.activate(i)
				res.Activated, res.Toggled = file, toggled
				res.Submitted = file != "" && e.DoubleClicked()
				s.focusList = true
			}
			if p.Menu != nil {
				path := r.path
				e.ContextMenu(func(m *ui.Menu) { p.Menu(m, path) })
			}
		}).Grow(1).FillWidth().Label(p.Label).Role(ui.RoleTree).Focusable().Padding(0, th.PaddingX-2*th.Density)
		if hasActive {
			list.ActiveDescendant(active)
		}
		res.Element = list
		s.hovered = list.Hovered()
		if s.focusList && s.list.Focus(c) {
			s.focusList = false
		}
		// Pierre's keys: arrows, Home, and End move the cursor only;
		// Right and Left open, close, or step; Enter and Space activate.
		switch {
		case s.list.Shortcut(c, 0, ui.KeyDown):
			s.step(1)
		case s.list.Shortcut(c, 0, ui.KeyUp):
			s.step(-1)
		case s.list.Shortcut(c, 0, ui.KeyHome):
			s.moveTo(0)
		case s.list.Shortcut(c, 0, ui.KeyEnd):
			s.moveTo(len(s.rows) - 1)
		case s.list.Shortcut(c, 0, ui.KeyRight):
			was := s.version
			s.right()
			if s.version != was {
				res.Toggled = s.focused
			}
		case s.list.Shortcut(c, 0, ui.KeyLeft):
			was := s.version
			s.left(t)
			if s.version != was {
				res.Toggled = s.focused
			}
		case s.list.Shortcut(c, 0, ui.KeyEnter):
			s.keyActivate(&res, true)
		case s.list.Shortcut(c, 0, ui.KeySpace):
			s.keyActivate(&res, false)
		}
		if s.cursor != before && s.cursor >= 0 && s.cursor < len(s.rows) {
			s.focused = s.rows[s.cursor].path
			res.Moved = true
		}
	})
	return res
}

// keyActivate does Enter (submit) or Space on the cursor's row, unless a
// click activated a row this frame.
func (s *State) keyActivate(res *Result, submit bool) {
	if res.Activated != "" || res.Toggled != "" {
		return
	}
	file, toggled := s.activate(s.cursor)
	res.Activated, res.Toggled = file, toggled
	res.Submitted = file != "" && submit
}

// listTheme is the host's theme with the list's own row marks off: the
// tree draws selection, hover, and focus itself, as Pierre does.
func listTheme(base *ui.Theme, th Theme) *ui.Theme {
	t := *base
	t.Accent, t.AccentText = ui.Transparent, th.Text
	t.SurfaceHover = th.Hover
	t.Radius = th.Radius
	t.FontSize = th.FontSize
	t.Text = th.Text
	return &t
}

// mark is a row's resolved decoration.
type mark struct {
	Decoration
	contains bool // a directory holding a change
	ignored  bool // inside an ignored directory
}

// decorate resolves the host's decorations once per tree and map: a host
// that keeps its map while nothing changed pays for it once.
func (s *State) decorate(t *Tree, given map[string]Decoration) map[string]mark {
	if given == nil {
		s.marks, s.marksTree, s.marksOf = nil, nil, nil
		return nil
	}
	if s.marksTree != t || reflect.ValueOf(given).UnsafePointer() != s.marksOf || len(given) != s.marksLen {
		s.marks = decorations(t, given)
		s.marksTree, s.marksOf, s.marksLen = t, reflect.ValueOf(given).UnsafePointer(), len(given)
	}
	return s.marks
}

// decorations resolves the host's decorations: Pierre's folder dots and
// ignored directories' inheritance.
func decorations(t *Tree, given map[string]Decoration) map[string]mark {
	if given == nil {
		return nil
	}
	marks := make(map[string]mark, len(given))
	var ignoredDirs []string
	for path, d := range given {
		row := t.row(path)
		m := marks[row]
		m.Decoration = d
		marks[row] = m
		if d.Status == "" {
			continue
		}
		if d.Status == Ignored {
			if t.IsDir(row) {
				ignoredDirs = append(ignoredDirs, row)
			}
			continue
		}
		for _, dir := range t.ancestors(path) {
			m := marks[dir]
			m.contains = true
			marks[dir] = m
		}
	}
	for _, dir := range ignoredDirs {
		var walk func(paths []string)
		walk = func(paths []string) {
			for _, path := range paths {
				m := marks[path]
				if m.Status == "" {
					m.ignored = true
					marks[path] = m
				}
				walk(t.Children(path))
			}
		}
		walk(t.Children(dir))
	}
	return marks
}

// status is a row's effective status: its own, or ignored from a
// directory above.
func (m mark) status() Status {
	if m.Status == "" && m.ignored {
		return Ignored
	}
	return m.Status
}

// geometry is Pierre's row metrics at the theme's density.
type geometry struct {
	itemPadding, gap, levelGap, icon, lane float32
}

func metrics(th Theme) geometry {
	d := th.Density
	if d == 0 {
		d = 1
	}
	return geometry{itemPadding: 8 * d, gap: 6 * d, levelGap: 8 * d, icon: 16, lane: 12}
}

// indent is the width of a row's spacing at depth, as Pierre's spacing
// section and its guides lay it out, with the gap before the icon.
func (g geometry) indent(depth int) float32 {
	if depth == 0 {
		return 0
	}
	return float32(depth) * (g.icon/2 - 0.5 + g.levelGap + g.gap)
}

// drawRow builds one row: spacing, icon or chevron, name, decoration
// lane, git lane.
func drawRow(c *ui.Context, t *Tree, th Theme, r row, marks map[string]mark, p Props, selected, focused, guides bool) ui.Element {
	g := metrics(th)
	m := marks[r.path]
	status := m.status()
	name := t.Name(r.path)
	ink := th.Text
	if selected {
		ink = th.SelectedText
	}
	nameInk := ink
	iconOpacity := float32(1)
	switch {
	case status != "":
		if color, ok := th.Status[status]; ok {
			nameInk = color
		}
		if status == Ignored {
			iconOpacity = 0.5
		}
	}
	if m.Dimmed {
		nameInk, iconOpacity = th.Muted, 0.5
	}
	if m.Viewed && !selected {
		nameInk, iconOpacity = th.Muted, 0.6
	}
	e := ui.Row(c).Height(th.RowHeight).FillWidth().AlignItems(ui.Center).Gap(g.gap).Padding(0, g.itemPadding).Radius(th.Radius)
	// Selected paints the theme's accent: first, so the tree's own fill
	// and ink below win.
	e = e.Role(ui.RoleTreeItem).Level(r.depth + 1).Selected(selected).Label(name)
	if selected {
		e = e.Background(th.Selected)
	}
	hovered := e.Hovered()
	if hovered && !selected {
		e = e.Background(th.Hover)
	}
	if r.dir {
		e = e.Expanded(r.open)
	}
	ring := th.SelectedBorder
	if focused {
		ring = th.Focus
	}
	if (selected || focused) && ring != ui.Transparent {
		radius := th.Radius
		e = e.DrawOver(func(pt *ui.Painter, box ui.Rect) {
			pt.Stroke(ui.Rect{X: box.X + 0.5, Y: box.Y + 0.5, W: box.W - 1, H: box.H - 1}, ring, radius, 1)
		})
	}
	e.Children(func() {
		if r.depth > 0 {
			spacing := ui.Box(c).Width(g.indent(r.depth) - g.gap).Height(th.RowHeight).Shrink(0)
			if guides {
				// Pierre's indent guides show while the tree is hovered:
				// one per level, in muted ink at 25%, 0.75 opacity.
				guide := th.Muted
				guide.A = uint8(float32(guide.A) * 0.25 * 0.75)
				depth, step, first := r.depth, g.indent(1), g.icon/2-0.5
				spacing.Draw(func(pt *ui.Painter, box ui.Rect) {
					for i := range depth {
						x := box.X + first + float32(i)*step
						pt.Fill(ui.Rect{X: x, Y: box.Y, W: 1, H: box.H}, guide, 0)
					}
				})
			}
		}
		if r.dir {
			ink := th.Muted
			if selected {
				ink = th.SelectedText
			}
			ui.Box(c).Size(g.icon, g.icon).Shrink(0).Center().Children(func() {
				chevron := ui.Icon(c, Chevron()).Size(12, 12).TextColor(ink)
				if !r.open {
					chevron.Rotate(-90)
				}
			})
		} else {
			icon := IconFor(name)
			hue, ok := icon.Color(th.Dark)
			if !ok {
				hue = th.Muted
			}
			ui.Box(c).Size(g.icon, g.icon).Shrink(0).Center().Opacity(iconOpacity).Children(func() {
				ui.Icon(c, icon.SVG()).Size(g.icon, g.icon).TextColor(hue)
			})
		}
		nameText(c, name, r.path, p.Match, nameInk)
		if marks != nil {
			lane(c, th, g, m, status, r.dir)
		}
	})
	return e
}

// nameText draws a row's name, its filter matches emphasized.
func nameText(c *ui.Context, name, path string, match func(path, name string) [][2]int, ink ui.Color) {
	var ranges [][2]int
	if match != nil {
		ranges = match(path, name)
	}
	if len(ranges) == 0 {
		ui.Text(c, name).SingleLine().TextColor(ink).Shrink(1).MinWidth(0)
		return
	}
	var spans []ui.Span
	at := 0
	for _, r := range ranges {
		start, end := max(r[0], at), min(r[1], len(name))
		if start >= end {
			continue
		}
		if start > at {
			spans = append(spans, ui.Span{Text: name[at:start], Color: ink})
		}
		spans = append(spans, ui.Span{Text: name[start:end], Color: ink, Weight: 600})
		at = end
	}
	if at < len(name) {
		spans = append(spans, ui.Span{Text: name[at:], Color: ink})
	}
	ui.RichText(c, spans...).SingleLine().TextColor(ink).Shrink(1).MinWidth(0).Label(name)
}

// lane draws the decoration lane (viewed check, parts) and the git lane
// (status letter, or a folder's dot).
func lane(c *ui.Context, th Theme, g geometry, m mark, status Status, dir bool) {
	ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End).AlignItems(ui.Center).Children(func() {
		if m.Viewed {
			ui.Icon(c, check()).Size(12, 12).TextColor(th.Muted).Shrink(0).Margin(0, 4, 0, 0)
		}
		for _, part := range m.Parts {
			ink := part.Color
			if ink == ui.Transparent {
				ink = th.Muted
			}
			ui.Text(c, part.Text).SingleLine().TextColor(ink).Shrink(0)
		}
	}).Tooltip(m.Title)
	ui.Box(c).Width(g.lane).Shrink(0).Center().Children(func() {
		switch {
		case status.Letter() != "":
			ui.Text(c, status.Letter()).FontWeight(600).TextColor(th.Status[status]).Tooltip(status.Title())
		case dir && m.contains:
			modified := th.Status[Modified]
			modified.A /= 2
			ui.Icon(c, dot()).Size(6, 6).TextColor(modified).Tooltip("Contains git status items")
		}
	})
}

// checkSVG is lucide's Check (ISC), the Viewed mark.
const checkSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="black" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`

func check() *ui.SVG {
	svgMu.Lock()
	defer svgMu.Unlock()
	if s, ok := svgCache["check"]; ok {
		return s
	}
	s := ui.MustParseSVG([]byte(checkSVG))
	svgCache["check"] = s
	return s
}

// Names joins row names, for tests and labels.
func names(paths []string, t *Tree) string {
	out := make([]string, len(paths))
	for i, path := range paths {
		out[i] = t.Name(path)
	}
	return strings.Join(out, ",")
}
