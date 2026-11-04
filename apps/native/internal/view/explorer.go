package view

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/apps/native/internal/tabs"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
	"github.com/flexdinesh/servef/apps/native/internal/tree"
	"github.com/flexdinesh/servef/packages/mygo/trees"
	"github.com/flexdinesh/servef/pkg/contract"
)

// explorer is the Files sidebar's tree state around the model's tree.
type explorer struct {
	state   trees.State
	tree    *trees.Tree
	read    int
	version int
	reveal  int
	synced  string

	// decorations are the statuses of decoratedTree's files from the
	// changes read decoratedChanges.
	decorations      map[string]trees.Decoration
	decoratedTree    *trees.Tree
	decoratedChanges int
}

// syncTree rebuilds the rows when a page brings the tree, opens the
// directories the model asks for, and selects the open document's row.
func (a *App) syncTree() {
	m, x := a.Model, &a.explorer
	if x.tree == nil || x.read != m.TreeRead {
		x.read = m.TreeRead
		x.tree = treeOf(m.Page.Tree)
	}
	if x.version != m.TreeVersion {
		// A project's first page: it starts with nothing selected, no
		// cursor, at the top, and the server's open folders.
		x.version, x.synced = m.TreeVersion, ""
		x.state.Reset()
		x.state.SetOpen(m.TreeOpen)
	}
	selected := m.Page.Selected
	if m.Page.At != "" || m.Active.Kind != tabs.Document {
		// A snapshot, a review, or no tab selects no working-tree file.
		selected = ""
	}
	if x.reveal != m.RevealVersion {
		x.reveal = m.RevealVersion
		if selected != "" {
			before := openPaths(&x.state)
			x.state.Reveal(x.tree, selected)
			if after := openPaths(&x.state); !slices.Equal(before, after) {
				m.RememberExpanded(after)
			}
		}
	}
	// The open document's row is selected when it changes; a folder a
	// click selected stays selected until then, as the web tree's.
	if selected != x.synced {
		x.synced = selected
		x.state.Selected = selected
	}
}

// openPaths lists the open folders in a stable order, to remember them.
func openPaths(s *trees.State) []string {
	paths := s.OpenPaths()
	slices.Sort(paths)
	return paths
}

// treeOf is the server's tree in the web tree's order, with single-child
// folder chains as one row: Pierre flattens them unless told not to, and
// FileTree.tsx never does.
func treeOf(nodes []contract.TreeNode) *trees.Tree {
	var convert func([]contract.TreeNode) []trees.Node
	convert = func(nodes []contract.TreeNode) []trees.Node {
		out := make([]trees.Node, len(nodes))
		for i, node := range nodes {
			out[i] = trees.Node{Name: node.Name, Path: node.Path, Dir: node.IsDir}
			if node.IsDir {
				out[i].Children = convert(node.Children)
			}
		}
		return out
	}
	return trees.New(convert(nodes), trees.Options{Sort: true, Flatten: true})
}

// files shows the Explorer tree: Pierre's icons, the uncommitted
// statuses of the files it shows, and the open document selected.
func (a *App) files(c *ui.Context) {
	m := a.Model
	var result trees.Result
	// The tree host's inset (explorer.css .file-tree-host).
	ui.Column(c).Grow(1).FillWidth().Padding(4).Children(func() { result = a.filesTree(c) })
	shown := m.Active == tabs.DocumentTab(m.Page.Selected) && m.Page.At == ""
	if result.Activated != "" && (result.Activated != m.Page.Selected || !shown) {
		m.OpenDocument(result.Activated)
	}
	if result.Toggled != "" {
		m.RememberExpanded(openPaths(&a.explorer.state))
	}
}

func (a *App) filesTree(c *ui.Context) trees.Result {
	m, x := a.Model, &a.explorer
	if x.decoratedTree != x.tree || x.decoratedChanges != m.ChangesVersion {
		x.decoratedTree, x.decoratedChanges = x.tree, m.ChangesVersion
		x.decorations = explorerDecorations(m.Tree, m.Changes.Files, m.ChangesRead)
	}
	return trees.View(c, &x.state, trees.Props{
		Tree:        x.tree,
		Theme:       explorerThemes[themeIndex(c.Theme().Dark)],
		Decorations: x.decorations,
		Label:       "Files",
	})
}

// explorerThemes are explorerTheme's, light and dark, made once.
var explorerThemes = [2]trees.Theme{explorerTheme(false), explorerTheme(true)}

func themeIndex(dark bool) int {
	if dark {
		return 1
	}
	return 0
}

// explorerDecorations are the git statuses of shown files once the
// changes are read; nil before, as the web passes none.
func explorerDecorations(x tree.Index, files []contract.ChangedFile, read bool) map[string]trees.Decoration {
	if !read {
		return nil
	}
	statuses := x.Statuses(files)
	decorations := make(map[string]trees.Decoration, len(statuses))
	for path, status := range statuses {
		decorations[path] = trees.Decoration{Status: trees.Status(status)}
	}
	return decorations
}

// explorerTheme maps servef's tokens onto the tree as explorer.css and
// tree-css.ts override Pierre's: the sidebar surface, secondary ink,
// the selection tint with its accent border, square rows 22px high at
// the label size, and servef's status hues.
func explorerTheme(dark bool) trees.Theme {
	p := theme.For(dark)
	th := trees.DefaultTheme(dark)
	th.Background = ui.Hex(p.SurfaceSecondary)
	th.Hover = trees.Mix(ui.Hex(p.TextPrimary), th.Background, 0.08)
	th.Text = ui.Hex(p.TextSecondary)
	th.Muted = ui.Hex(p.TextMuted)
	th.Selected = ui.Hex(p.AccentSubtle)
	th.SelectedText = ui.Hex(p.AccentHover)
	th.SelectedBorder = ui.Hex(p.Accent)
	th.Focus = ui.Hex(p.Accent)
	th.Radius = 0
	th.RowHeight = 22
	th.PaddingX = 8
	th.Status[trees.Modified] = ui.Hex(p.Warning)
	th.Status[trees.Added] = ui.Hex(p.Success)
	th.Status[trees.Untracked] = ui.Hex(p.Success)
	th.Status[trees.Deleted] = ui.Hex(p.Destructive)
	th.Status[trees.Renamed] = ui.Hex(p.Accent)
	return th
}

// fileIcon is a file's Pierre icon by name, as the web's FileIcon shows
// beside changed files.
func fileIcon(c *ui.Context, path string) {
	icon := trees.IconFor(path)
	hue, ok := icon.Color(c.Theme().Dark)
	if !ok {
		hue = c.Theme().TextMuted
	}
	ui.Icon(c, icon.SVG()).Size(16, 16).TextColor(hue).Shrink(0)
}
