// Package files discovers in-scope files and safely resolves indexed paths.
package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var (
	// ErrNotIndexed is returned when a path was not part of a scan's file index.
	ErrNotIndexed = errors.New("path is not indexed")
	// ErrPathEscape is returned when resolving a path would leave the scan root.
	ErrPathEscape = errors.New("path escapes scan root")
)

// Entry is a directory or supported file in a scan tree. Path is slash-separated
// and relative to the scan root; the root entry has an empty Path.
type Entry struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	IsDir    bool    `json:"isDir"`
	Children []Entry `json:"children,omitempty"`
}

// Warning describes an entry that could not be inspected during a scan. Path
// is slash-separated and relative to the scan root.
type Warning struct {
	Path string
	Err  error
}

func (w Warning) Error() string {
	if w.Path == "" {
		return w.Err.Error()
	}
	return fmt.Sprintf("%s: %v", w.Path, w.Err)
}

// Index is the deterministic result of a scan. Files contains the paths of all
// supported files in tree traversal order. Warnings contains recoverable errors
// encountered below the root and is intentionally omitted from JSON output.
type Index struct {
	Tree     Entry     `json:"tree"`
	Files    []string  `json:"files"`
	Warnings []Warning `json:"-"`

	root    string
	fileSet map[string]struct{}
	ignored map[string]struct{}
	accepts func(string) bool
}

// Scan builds an index rooted at root. The root directory is depth zero, so a
// maxDepth of zero includes supported files directly within root. Negative
// maxDepth values scan without a depth limit. Directory names in excluded are
// matched by base name at every scanned level. .git entries of any type are
// never indexed.
//
// Symlinks are not included. Failure to open or inspect the root is fatal;
// failures below it are recorded in Index.Warnings and the rest of the scan is
// returned.
func Scan(root string, maxDepth int, excluded []string, accepts func(string) bool) (*Index, error) {
	if accepts == nil {
		return nil, errors.New("file matcher is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve scan root: %w", err)
	}

	canonicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve scan root: %w", err)
	}

	rootInfo, err := os.Stat(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect scan root: %w", err)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("scan root %q is not a directory", root)
	}

	index := &Index{
		Files:   make([]string, 0),
		root:    canonicalRoot,
		fileSet: make(map[string]struct{}),
		ignored: make(map[string]struct{}),
		accepts: accepts,
	}
	index.Tree = Entry{Name: filepath.Base(canonicalRoot), IsDir: true}

	excludedNames := make(map[string]struct{}, len(excluded))
	for _, name := range excluded {
		name = filepath.Base(filepath.Clean(name))
		if name != "." && name != string(filepath.Separator) && name != "" {
			excludedNames[name] = struct{}{}
		}
	}

	if err := index.scanDirectory(&index.Tree, 0, maxDepth, excludedNames, NewIgnoreRules(canonicalRoot), false); err != nil {
		return nil, fmt.Errorf("read scan root: %w", err)
	}
	return index, nil
}

// Ignored reports whether an indexed file is excluded by gitignore rules: the
// root repository's core.excludesFile and info/exclude, including for linked
// worktrees, and the .gitignore files from the root down. Ignored files stay
// browsable; content search and code navigation skip them.
func (i *Index) Ignored(name string) bool {
	if i == nil {
		return false
	}
	_, ok := i.ignored[name]
	return ok
}

// RootPath returns the absolute, symlink-evaluated directory used by the scan.
func (i *Index) RootPath() string {
	return i.root
}

// Resolve returns an absolute path for an indexed supported file. It rejects
// unindexed paths and evaluates the current filesystem path to ensure it still
// resides within the scan root.
func (i *Index) Resolve(name string) (string, error) {
	if i == nil || i.root == "" {
		return "", ErrNotIndexed
	}
	if filepath.IsAbs(name) {
		return "", ErrPathEscape
	}

	rel := path.Clean(filepath.ToSlash(name))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", ErrPathEscape
	}
	if _, ok := i.fileSet[rel]; !ok {
		return "", ErrNotIndexed
	}

	candidate := filepath.Join(i.root, filepath.FromSlash(rel))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve indexed path %q: %w", rel, err)
	}
	if !Within(i.root, resolved) {
		return "", ErrPathEscape
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect indexed path %q: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("resolve indexed path %q: not a regular file", rel)
	}
	return resolved, nil
}

// scanDirectory indexes node's directory. inIgnored reports that it or an
// ancestor directory is ignored, which git never re-includes below.
func (i *Index) scanDirectory(node *Entry, depth, maxDepth int, excluded map[string]struct{}, ignore *IgnoreRules, inIgnored bool) error {
	dirPath := i.root
	if node.Path != "" {
		dirPath = filepath.Join(i.root, filepath.FromSlash(node.Path))
	}
	resolvedDir, err := filepath.EvalSymlinks(dirPath)
	if err != nil {
		if depth == 0 {
			return err
		}
		i.Warnings = append(i.Warnings, Warning{Path: node.Path, Err: err})
		return nil
	}
	if !Within(i.root, resolvedDir) {
		if depth == 0 {
			return ErrPathEscape
		}
		i.Warnings = append(i.Warnings, Warning{Path: node.Path, Err: ErrPathEscape})
		return nil
	}
	dirPath = resolvedDir

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if depth == 0 {
			return err
		}
		i.Warnings = append(i.Warnings, Warning{Path: node.Path, Err: err})
		return nil
	}

	children := make([]scannedEntry, 0, len(entries))
	for _, dirEntry := range entries {
		// Git metadata is never content, including a linked worktree's .git file.
		if dirEntry.Name() == ".git" {
			continue
		}
		rel := joinSlash(node.Path, dirEntry.Name())
		info, infoErr := dirEntry.Info()
		if infoErr != nil {
			i.Warnings = append(i.Warnings, Warning{Path: rel, Err: infoErr})
			continue
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			continue
		}

		if info.IsDir() {
			if _, skip := excluded[dirEntry.Name()]; skip {
				continue
			}
			if maxDepth >= 0 && depth >= maxDepth {
				continue
			}
			children = append(children, scannedEntry{name: dirEntry.Name(), path: rel, isDir: true})
			continue
		}
		if info.Mode().IsRegular() && i.accepts(dirEntry.Name()) {
			children = append(children, scannedEntry{name: dirEntry.Name(), path: rel})
		}
	}

	sort.Slice(children, func(a, b int) bool {
		if children[a].isDir != children[b].isDir {
			return children[a].isDir
		}
		aFold, bFold := strings.ToLower(children[a].name), strings.ToLower(children[b].name)
		if aFold != bFold {
			return aFold < bFold
		}
		return children[a].name < children[b].name
	})

	node.Children = make([]Entry, 0, len(children))
	for _, child := range children {
		node.Children = append(node.Children, Entry{
			Name:  child.name,
			Path:  child.path,
			IsDir: child.isDir,
		})
		childNode := &node.Children[len(node.Children)-1]
		if child.isDir {
			// Nested failures are recorded as warnings so other siblings remain usable.
			ignored := inIgnored || ignore.ignoredIn(node.Path, child.path, true)
			_ = i.scanDirectory(childNode, depth+1, maxDepth, excluded, ignore, ignored)
			if len(childNode.Children) == 0 {
				node.Children = node.Children[:len(node.Children)-1]
			}
			continue
		}
		i.Files = append(i.Files, child.path)
		i.fileSet[child.path] = struct{}{}
		if inIgnored || ignore.ignoredIn(node.Path, child.path, false) {
			i.ignored[child.path] = struct{}{}
		}
	}
	return nil
}

type scannedEntry struct {
	name  string
	path  string
	isDir bool
}

func joinSlash(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

// Within reports whether candidate is root or lies below it. Both paths must be
// absolute and symlink-evaluated.
func Within(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
