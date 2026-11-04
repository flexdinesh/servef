package trees

import (
	"unsafe"

	"github.com/egoist/mygo/ui"
)

// State is a tree's view state. Everything in it is kept by path, so it
// survives a new Tree (a data refresh) and a filtered one: open
// directories stay open, the selected row stays selected, and the
// keyboard cursor stays on its row while that row shows.
type State struct {
	// Selected is the selected row's path (the host's open document), ""
	// for none. A click, Enter, or Space selects a row, as Pierre's.
	Selected string

	open    map[string]bool
	version int
	focused string
	cursor  int
	list    ui.ListState

	rows      []row
	rowsTree  *Tree
	rowsOpen  int
	scrollTo  string
	focusList bool
	hovered   bool

	// marks are the resolved decorations of marksTree and the map at
	// marksOf with marksLen entries.
	marks     map[string]mark
	marksTree *Tree
	marksOf   unsafe.Pointer
	marksLen  int
}

// row is one visible row: its path, depth, and whether it is a directory
// and open.
type row struct {
	path  string
	depth int
	dir   bool
	open  bool
}

// IsOpen reports whether a directory row shows its children. A flattened
// chain is open when its deepest directory is.
func (s *State) IsOpen(path string) bool { return s.open[path] }

// Expand opens a directory and the directories above it, as Pierre's
// expand does: a flattened chain's row opens every directory of the
// chain, so they stay open when a change breaks the chain.
func (s *State) Expand(path string) {
	for _, dir := range prefixes(path) {
		s.setOpen(dir, true)
	}
	s.setOpen(path, true)
}

// Collapse closes a directory, only it: a chain's row closes its deepest
// directory, as Pierre's collapse does.
func (s *State) Collapse(path string) { s.setOpen(path, false) }

// prefixes are the directories above path, outermost first.
func prefixes(path string) []string {
	var dirs []string
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			dirs = append(dirs, path[:i])
		}
	}
	return dirs
}

// Reset forgets everything: open directories, the selection, the cursor,
// and where the list is scrolled, as for another project.
func (s *State) Reset() { *s = State{} }

// Toggle opens a closed directory and closes an open one.
func (s *State) Toggle(path string) {
	if s.open[path] {
		s.Collapse(path)
		return
	}
	s.Expand(path)
}

func (s *State) setOpen(path string, open bool) {
	if s.open == nil {
		s.open = map[string]bool{}
	}
	if s.open[path] == open {
		return
	}
	if open {
		s.open[path] = true
	} else {
		delete(s.open, path)
	}
	s.version++
}

// SetOpen replaces the open directories, as when a project's tree first
// arrives with the server's open folders.
func (s *State) SetOpen(paths []string) {
	s.open = make(map[string]bool, len(paths))
	for _, path := range paths {
		s.open[path] = true
	}
	s.version++
}

// OpenPaths lists the open directories, to remember them.
func (s *State) OpenPaths() []string {
	paths := make([]string, 0, len(s.open))
	for path := range s.open {
		paths = append(paths, path)
	}
	return paths
}

// ExpandAll opens every directory of t; CollapseAll closes them all.
func (s *State) ExpandAll(t *Tree) {
	for path, e := range t.entries {
		if e.dir {
			s.setOpen(path, true)
		}
	}
}

// CollapseAll closes every directory.
func (s *State) CollapseAll() {
	if len(s.open) > 0 {
		s.open = nil
		s.version++
	}
}

// AnyOpen reports whether a directory of t is open, as the web's
// collapse-all button decides between collapse and expand.
func (s *State) AnyOpen(t *Tree) bool {
	for path := range s.open {
		if t.IsDir(path) {
			return true
		}
	}
	return false
}

// Reveal opens the directories holding path and scrolls its row into
// view on the next frame, without moving the keyboard cursor.
func (s *State) Reveal(t *Tree, path string) {
	for _, dir := range prefixes(path) {
		s.setOpen(dir, true)
	}
	s.scrollTo = t.row(path)
}

// Focused is the keyboard cursor's row, "" for none.
func (s *State) Focused() string { return s.focused }

// Focus moves the keyboard cursor to path's row and gives the tree the
// keyboard focus on the next frame.
func (s *State) Focus(t *Tree, path string) {
	s.Reveal(t, path)
	s.focused = t.row(path)
	s.focusList = true
}

// Visible is the range of rows the list shows, as ui.ListState's.
func (s *State) Visible() (first, last int) { return s.list.Visible() }

// Rows lists the visible rows' paths, as the last frame showed them.
func (s *State) Rows() []string {
	paths := make([]string, len(s.rows))
	for i, r := range s.rows {
		paths[i] = r.path
	}
	return paths
}

// flatten lists t's visible rows when t or the open set changed.
func (s *State) flatten(t *Tree) {
	if s.rowsTree == t && s.rowsOpen == s.version && s.rows != nil {
		return
	}
	s.rowsTree, s.rowsOpen = t, s.version
	s.rows = s.rows[:0]
	var walk func(paths []string, depth int)
	walk = func(paths []string, depth int) {
		for _, path := range paths {
			e := t.entries[path]
			open := e.dir && s.open[path]
			s.rows = append(s.rows, row{path: path, depth: depth, dir: e.dir, open: open})
			if open {
				walk(e.children, depth+1)
			}
		}
	}
	if t != nil {
		walk(t.roots, 0)
	}
	if s.rows == nil {
		s.rows = []row{}
	}
	s.syncCursor()
}

// syncCursor puts the cursor index on the focused row; a row that no
// longer shows leaves the cursor where it was, on the row now there.
func (s *State) syncCursor() {
	if s.focused == "" {
		s.cursor = -1
		return
	}
	if i := s.index(s.focused); i >= 0 {
		s.cursor = i
		return
	}
	if len(s.rows) == 0 {
		s.cursor, s.focused = -1, ""
		return
	}
	s.cursor = max(0, min(s.cursor, len(s.rows)-1))
	s.focused = s.rows[s.cursor].path
}

// index returns path's visible row, -1 for none.
func (s *State) index(path string) int {
	for i, r := range s.rows {
		if r.path == path {
			return i
		}
	}
	return -1
}

// step moves the cursor by delta rows, as Pierre's moveFocus: from no
// cursor, from the first row.
func (s *State) step(delta int) {
	if len(s.rows) == 0 {
		return
	}
	from := max(s.cursor, 0)
	s.moveTo(max(0, min(from+delta, len(s.rows)-1)))
}

// moveTo puts the cursor on row i.
func (s *State) moveTo(i int) {
	if i < 0 || i >= len(s.rows) {
		return
	}
	s.cursor, s.focused = i, s.rows[i].path
	s.list.ScrollIntoView(i)
}

// right does Pierre's ArrowRight: open a closed directory, else go to
// the next row (an open directory's first child).
func (s *State) right() {
	if s.cursor < 0 || s.cursor >= len(s.rows) {
		return
	}
	if r := s.rows[s.cursor]; r.dir && !r.open {
		s.Expand(r.path)
		return
	}
	s.moveTo(s.cursor + 1)
}

// left does Pierre's ArrowLeft: close an open directory, else go to the
// parent row.
func (s *State) left(t *Tree) {
	if s.cursor < 0 || s.cursor >= len(s.rows) {
		return
	}
	r := s.rows[s.cursor]
	if r.dir && r.open {
		s.Collapse(r.path)
		return
	}
	if parent := t.Parent(r.path); parent != "" {
		s.moveTo(s.index(parent))
	}
}

// activate does a click, Enter, or Space on row i, as on Pierre's row
// button: the row is selected, and a directory opens or closes while a
// file is returned.
func (s *State) activate(i int) (file, toggled string) {
	if i < 0 || i >= len(s.rows) {
		return "", ""
	}
	r := s.rows[i]
	s.cursor, s.focused = i, r.path
	s.Selected = r.path
	if r.dir {
		s.Toggle(r.path)
		return "", r.path
	}
	return r.path, ""
}
