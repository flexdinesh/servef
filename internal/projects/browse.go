package projects

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flexdinesh/servef/internal/files"
)

// ErrNotDirectory is returned when Browse is given a path that is not a
// directory.
var ErrNotDirectory = errors.New("not a directory")

// BrowseEntry is a child directory offered when choosing a folder to serve
// (decision 0076).
type BrowseEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Repository bool   `json:"repository"`
	Hidden     bool   `json:"hidden"`
}

// Browse lists the child directories of an absolute directory outside any
// root set: directories and symlinks resolving to directories, sorted by
// name, without excluded names (files.Excluded), at most limit. truncated
// reports that more existed.
func Browse(dir string, limit int) (entries []BrowseEntry, truncated bool, err error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, false, ErrNotDirectory
	}
	children, err := os.ReadDir(dir)
	if err != nil {
		return nil, false, err
	}
	entries = []BrowseEntry{}
	for _, child := range children {
		name := child.Name()
		if files.Excluded(name) {
			continue
		}
		path := filepath.Join(dir, name)
		if !child.IsDir() {
			if child.Type()&os.ModeSymlink == 0 {
				continue
			}
			if target, err := os.Stat(path); err != nil || !target.IsDir() {
				continue
			}
		}
		entries = append(entries, BrowseEntry{Name: name, Path: path, Repository: hasGit(path), Hidden: strings.HasPrefix(name, ".")})
	}
	sort.Slice(entries, func(a, b int) bool {
		left, right := strings.ToLower(entries[a].Name), strings.ToLower(entries[b].Name)
		if left != right {
			return left < right
		}
		return entries[a].Name < entries[b].Name
	})
	if len(entries) > limit {
		return entries[:limit], true, nil
	}
	return entries, false, nil
}
