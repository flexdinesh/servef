package files

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// NameFilter selects files by case-insensitive base-name globs in path.Match
// syntax. It knows no file formats; every matching file is in scope.
type NameFilter struct {
	patterns []string
	folded   []string
}

// AllNames is the default filter, equivalent to the single pattern "*".
func AllNames() NameFilter {
	filter, _ := NewNameFilter([]string{"*"})
	return filter
}

// NewNameFilter validates patterns. Empty patterns, malformed globs, and
// patterns containing "/" (which can never match a base name) are rejected.
func NewNameFilter(patterns []string) (NameFilter, error) {
	if len(patterns) == 0 {
		return NameFilter{}, errors.New("at least one filename pattern is required")
	}
	filter := NameFilter{patterns: make([]string, 0, len(patterns)), folded: make([]string, 0, len(patterns))}
	for _, pattern := range patterns {
		if pattern == "" {
			return NameFilter{}, errors.New("empty filename pattern")
		}
		if strings.Contains(pattern, "/") {
			return NameFilter{}, fmt.Errorf("invalid filename pattern %q: patterns match base names and cannot contain /", pattern)
		}
		folded := strings.ToLower(pattern)
		if _, err := path.Match(folded, ""); err != nil {
			return NameFilter{}, fmt.Errorf("invalid filename pattern %q: %w", pattern, err)
		}
		filter.patterns = append(filter.patterns, pattern)
		filter.folded = append(filter.folded, folded)
	}
	return filter, nil
}

// Patterns returns the validated patterns as given.
func (f NameFilter) Patterns() []string { return append([]string{}, f.patterns...) }

// Match reports whether the base name of name matches any pattern.
func (f NameFilter) Match(name string) bool {
	base := strings.ToLower(path.Base(name))
	for _, pattern := range f.folded {
		if matched, _ := path.Match(pattern, base); matched {
			return true
		}
	}
	return false
}
