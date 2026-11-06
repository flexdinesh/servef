package projects

import (
	"errors"
	"fmt"
	"sync"

	"github.com/flexdinesh/srcz/internal/files"
)

// RootSet is the ordered set of canonical served roots (decision 0060). Roots
// are absolute, symlink-evaluated directories; none lies inside another
// (decision 0009). It is safe for concurrent use. The zero value is an empty
// set.
type RootSet struct {
	mu    sync.RWMutex
	roots []string
}

// NewRootSet resolves paths to canonical roots in order, drops duplicates, and
// rejects missing paths, non-directories, overlapping roots, and an empty set.
func NewRootSet(paths []string) (*RootSet, error) {
	set := &RootSet{roots: make([]string, 0, len(paths))}
	for _, name := range paths {
		if _, _, err := set.Add(name); err != nil {
			return nil, err
		}
	}
	if len(set.roots) == 0 {
		return nil, errors.New("no root to serve")
	}
	return set, nil
}

// List returns a copy of the roots in order.
func (s *RootSet) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string{}, s.roots...)
}

// Add appends path's canonical directory and reports whether it was added. An
// existing root returns (canonical, false, nil); a root inside an existing
// root, or containing one, is an error.
func (s *RootSet) Add(path string) (string, bool, error) {
	root, err := canonicalDir(path)
	if err != nil {
		return "", false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if containsPath(s.roots, root) {
		return root, false, nil
	}
	for _, other := range s.roots {
		if files.Within(other, root) || files.Within(root, other) {
			return "", false, fmt.Errorf("roots %q and %q overlap; pass only the outer one", other, root)
		}
	}
	s.roots = append(s.roots, root)
	return root, true, nil
}

// Cover serves path: a path within (or equal to) a root returns that root
// with added false. Otherwise path's canonical directory becomes a root,
// replacing the roots inside it, returned in order as replaced; it takes the
// first replaced root's place, or is appended (decision 0066).
func (s *RootSet) Cover(path string) (root string, added bool, replaced []string, err error) {
	dir, err := canonicalDir(path)
	if err != nil {
		return "", false, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, other := range s.roots {
		if files.Within(other, dir) {
			return other, false, nil, nil
		}
	}
	roots := make([]string, 0, len(s.roots)+1)
	for _, other := range s.roots {
		if !files.Within(dir, other) {
			roots = append(roots, other)
			continue
		}
		if replaced == nil {
			roots = append(roots, dir)
		}
		replaced = append(replaced, other)
	}
	if replaced == nil {
		roots = append(roots, dir)
	}
	s.roots = roots
	return dir, true, replaced, nil
}
