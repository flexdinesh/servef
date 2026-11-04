// Package trees is a file tree widget for mygo, ported from what
// @pierre/trees (Apache-2.0) does for a web app: Pierre's file icons by
// name, folder chevrons, git status lanes, host decorations, dimmed rows,
// filter-match emphasis, keyboard navigation, and selection and focus kept
// by path across data refreshes and filters. It is generic: hosts pass
// data, never callbacks that know their domain.
//
// The widget draws its own rows on a virtualized ui.List rather than on
// ui.Outline: Outline puts its disclosure arrow before every row, leaves
// an empty arrow box before leaves, and fixes its rows' padding, while
// Pierre draws the folder chevron in the icon slot that files use for
// their icon.
package trees

import (
	"cmp"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Node is one entry of a tree the host supplies: a file, or a directory
// with its children. Paths are unique, "/"-separated, and identify rows.
type Node struct {
	Name     string
	Path     string
	Dir      bool
	Children []Node
}

// Options shape a Tree.
type Options struct {
	// Sort orders siblings as the web tree does: folders first, then
	// natural, case-insensitive names. Without it, the given order stands.
	Sort bool
	// Flatten shows a chain of directories that each hold one directory
	// as one row ("src / app"), as Pierre's flattenEmptyDirectories does;
	// the row is the deepest directory's.
	Flatten bool
}

// entry is one row-to-be: a file, a directory, or a flattened chain of
// directories ending at path.
type entry struct {
	path     string
	name     string
	dir      bool
	parent   string // the parent row's path, "" at the top
	children []string
	// segments are a flattened chain's directories, outermost first; nil
	// unless the row stands for more than one directory.
	segments []string
}

// Tree is an immutable snapshot of a host's tree: rows by path. Make a
// new one when the data changes; State keeps expansion, selection, and
// focus by path.
type Tree struct {
	roots   []string
	entries map[string]*entry
	// rowOf maps every directory of a flattened chain to its row.
	rowOf map[string]string
	files int
}

// New builds a tree from nodes.
func New(nodes []Node, o Options) *Tree {
	t := &Tree{entries: map[string]*entry{}, rowOf: map[string]string{}}
	t.roots = t.add(nodes, "", o)
	return t
}

func (t *Tree) add(nodes []Node, parent string, o Options) []string {
	if o.Sort {
		nodes = sorted(nodes)
	}
	paths := make([]string, 0, len(nodes))
	for _, node := range nodes {
		e := &entry{path: node.Path, name: node.Name, dir: node.Dir, parent: parent}
		if node.Dir {
			chain := []Node{node}
			for o.Flatten && len(node.Children) == 1 && node.Children[0].Dir {
				node = node.Children[0]
				chain = append(chain, node)
			}
			if len(chain) > 1 {
				e.path = node.Path
				names := make([]string, len(chain))
				for i, link := range chain {
					names[i] = link.Name
					e.segments = append(e.segments, link.Path)
					t.rowOf[link.Path] = node.Path
				}
				e.name = strings.Join(names, " / ")
			}
			e.children = t.add(node.Children, e.path, o)
		} else {
			t.files++
		}
		t.entries[e.path] = e
		paths = append(paths, e.path)
	}
	return paths
}

// sorted orders nodes as the web tree's path store does
// (path-store/src/sort.ts): folders first, then names compared
// case-insensitively with digit runs as numbers ("file2" before
// "file10"), then the exact names.
func sorted(nodes []Node) []Node {
	out := append([]Node(nil), nodes...)
	keys := make(map[string]sortKey, len(out))
	key := func(name string) sortKey {
		k, ok := keys[name]
		if !ok {
			k = newSortKey(name)
			keys[name] = k
		}
		return k
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return compareNames(a.Name, b.Name, key(a.Name), key(b.Name)) < 0
	})
	return out
}

// sortKey is a name's lower-case form and its natural tokens: text runs
// and digit runs read as numbers, as JavaScript numbers.
type sortKey struct {
	lower  string
	tokens []token
}

type token struct {
	text   string
	number float64
	digits bool
}

// String is the token as JavaScript's String() gives it.
func (t token) String() string {
	if !t.digits {
		return t.text
	}
	switch {
	case math.IsInf(t.number, 1):
		return "Infinity"
	case t.number < 1e21:
		return strconv.FormatFloat(t.number, 'f', -1, 64)
	}
	return strconv.FormatFloat(t.number, 'g', -1, 64)
}

func newSortKey(name string) sortKey {
	lower := strings.ToLower(name)
	return sortKey{lower: lower, tokens: naturalTokens(lower)}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func naturalTokens(value string) []token {
	var tokens []token
	start, i := 0, 0
	for i < len(value) {
		for i < len(value) && !isDigit(value[i]) {
			i++
		}
		if i >= len(value) {
			break
		}
		if i > start {
			tokens = append(tokens, token{text: value[start:i]})
		}
		var number float64
		for i < len(value) && isDigit(value[i]) {
			number = number*10 + float64(value[i]-'0')
			i++
		}
		tokens = append(tokens, token{number: number, digits: true})
		start = i
	}
	if start < len(value) || len(tokens) == 0 {
		tokens = append(tokens, token{text: value[start:]})
	}
	return tokens
}

func compareTokens(a, b []token) int {
	for i := range min(len(a), len(b)) {
		x, y := a[i], b[i]
		if x == y {
			continue
		}
		if x.digits && y.digits {
			return cmp.Compare(x.number, y.number)
		}
		if xs, ys := x.String(), y.String(); xs != ys {
			return jsCompare(xs, ys)
		}
	}
	return cmp.Compare(len(a), len(b))
}

// compareNames is compareSegmentValues: the keys, then the exact names.
func compareNames(a, b string, ka, kb sortKey) int {
	c := 0
	if len(ka.tokens) == 1 && len(kb.tokens) == 1 && !ka.tokens[0].digits && !kb.tokens[0].digits {
		c = jsCompare(ka.lower, kb.lower)
	} else if c = compareTokens(ka.tokens, kb.tokens); c == 0 {
		c = jsCompare(ka.lower, kb.lower)
	}
	if c != 0 {
		return c
	}
	return jsCompare(a, b)
}

// jsCompare orders strings as JavaScript's < does: by UTF-16 code units.
func jsCompare(a, b string) int {
	if a == b {
		return 0
	}
	x, y := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	return slices.Compare(x, y)
}

// FromPaths builds a tree from file paths, with their directories, as
// Pierre's tree takes a path list.
func FromPaths(paths []string, o Options) *Tree {
	type dir struct {
		node  Node
		order []string
		kids  map[string]*dir
		files []Node
	}
	root := &dir{kids: map[string]*dir{}}
	for _, path := range paths {
		parts := strings.Split(path, "/")
		at := root
		for i, part := range parts[:len(parts)-1] {
			next, ok := at.kids[part]
			if !ok {
				next = &dir{node: Node{Name: part, Path: strings.Join(parts[:i+1], "/"), Dir: true}, kids: map[string]*dir{}}
				at.kids[part] = next
				at.order = append(at.order, part)
			}
			at = next
		}
		at.files = append(at.files, Node{Name: parts[len(parts)-1], Path: path})
	}
	var build func(d *dir) []Node
	build = func(d *dir) []Node {
		nodes := make([]Node, 0, len(d.order)+len(d.files))
		for _, name := range d.order {
			kid := d.kids[name]
			node := kid.node
			node.Children = build(kid)
			nodes = append(nodes, node)
		}
		return append(nodes, d.files...)
	}
	return New(build(root), o)
}

// Filter returns the tree of the files keep accepts and the directories
// holding them, as filterTreeByFilename does: filters change what shows,
// while State keeps expansion and selection by path. keep gets a file's
// path and name.
func (t *Tree) Filter(keep func(path, name string) bool) *Tree {
	out := &Tree{entries: map[string]*entry{}, rowOf: t.rowOf}
	var walk func(paths []string) []string
	walk = func(paths []string) []string {
		var kept []string
		for _, path := range paths {
			e := t.entries[path]
			if !e.dir {
				if !keep(e.path, e.name) {
					continue
				}
				out.files++
				out.entries[path] = e
				kept = append(kept, path)
				continue
			}
			children := walk(e.children)
			if len(children) == 0 {
				continue
			}
			copied := *e
			copied.children = children
			out.entries[path] = &copied
			kept = append(kept, path)
		}
		return kept
	}
	out.roots = walk(t.roots)
	return out
}

// Roots are the top rows' paths.
func (t *Tree) Roots() []string { return t.roots }

// Children returns a directory row's children, nil for a file or an
// unknown path.
func (t *Tree) Children(path string) []string {
	if e, ok := t.entries[path]; ok && e.dir {
		return e.children
	}
	return nil
}

// Has reports whether path is a row.
func (t *Tree) Has(path string) bool { _, ok := t.entries[path]; return ok }

// IsDir reports whether path is a directory row.
func (t *Tree) IsDir(path string) bool { e, ok := t.entries[path]; return ok && e.dir }

// Name returns a row's name; a flattened chain's joins its directories
// with " / ".
func (t *Tree) Name(path string) string {
	if e, ok := t.entries[path]; ok {
		return e.name
	}
	return ""
}

// Parent returns a row's parent row, "" at the top.
func (t *Tree) Parent(path string) string {
	if e, ok := t.entries[path]; ok {
		return e.parent
	}
	return ""
}

// Files counts the files.
func (t *Tree) Files() int { return t.files }

// Len counts the rows.
func (t *Tree) Len() int { return len(t.entries) }

// row returns the row path shows in: itself, or a flattened chain's row.
func (t *Tree) row(path string) string {
	if row, ok := t.rowOf[path]; ok {
		return row
	}
	return path
}

// ancestors returns the directory rows that hold path, outermost first;
// for a path the tree lacks, those of its "/"-separated prefixes.
func (t *Tree) ancestors(path string) []string {
	var dirs []string
	if !t.Has(t.row(path)) {
		for i := strings.Index(path, "/"); i >= 0; i = nextSlash(path, i) {
			if row := t.row(path[:i]); t.IsDir(row) && (len(dirs) == 0 || dirs[len(dirs)-1] != row) {
				dirs = append(dirs, row)
			}
		}
		return dirs
	}
	for p := t.Parent(t.row(path)); p != ""; p = t.Parent(p) {
		dirs = append(dirs, p)
	}
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs
}

func nextSlash(path string, i int) int {
	if next := strings.Index(path[i+1:], "/"); next >= 0 {
		return i + 1 + next
	}
	return -1
}
