// Package tree indexes a project's file tree (contract.TreeNode) for a
// virtualized outline: items are project-relative paths, children are
// looked up by path. It is pure.
package tree

import (
	"github.com/flexdinesh/servef/pkg/contract"
)

// Entry is one file or directory of the tree.
type Entry struct {
	Path     string
	Name     string
	FileType string
	IsDir    bool
	// Children are the paths of a directory's entries, in server order;
	// nil for a file. An empty directory has an empty, non-nil slice.
	Children []string
}

// Index is a tree by path.
type Index struct {
	roots   []string
	entries map[string]Entry
}

// New indexes nodes, the server's tree.
func New(nodes []contract.TreeNode) Index {
	index := Index{entries: map[string]Entry{}}
	index.roots = index.add(nodes)
	return index
}

func (x *Index) add(nodes []contract.TreeNode) []string {
	paths := make([]string, 0, len(nodes))
	for _, node := range nodes {
		entry := Entry{Path: node.Path, Name: node.Name, FileType: node.FileType, IsDir: node.IsDir}
		if node.IsDir {
			entry.Children = x.add(node.Children)
		}
		x.entries[node.Path] = entry
		paths = append(paths, node.Path)
	}
	return paths
}

// Roots are the top-level paths.
func (x Index) Roots() []string { return x.roots }

// Children returns a directory's children, nil for a file or an unknown
// path, as ui.Outline expects.
func (x Index) Children(path string) []string {
	entry, ok := x.entries[path]
	if !ok || !entry.IsDir {
		return nil
	}
	return entry.Children
}

// Entry returns the entry at path.
func (x Index) Entry(path string) (Entry, bool) {
	entry, ok := x.entries[path]
	return entry, ok
}

// Len counts every entry.
func (x Index) Len() int { return len(x.entries) }

// Open returns the directories the server marked open, so the outline
// starts as the web tree does.
func Open(nodes []contract.TreeNode) []string {
	var open []string
	var walk func([]contract.TreeNode)
	walk = func(nodes []contract.TreeNode) {
		for _, node := range nodes {
			if node.IsDir && node.Open {
				open = append(open, node.Path)
				walk(node.Children)
			}
		}
	}
	walk(nodes)
	return open
}

// Statuses are the change statuses of the files the tree shows, by path,
// as the web Explorer decorates them; Source Control lists the rest.
func (x Index) Statuses(files []contract.ChangedFile) map[string]contract.ChangeStatus {
	statuses := map[string]contract.ChangeStatus{}
	for _, file := range files {
		if entry, ok := x.entries[file.Path]; ok && !entry.IsDir {
			statuses[file.Path] = file.Status
		}
	}
	return statuses
}
