// Package bridge validates what pages send through window.servefDesktop
// (decision 0081). Pages are untrusted input: every field is checked and
// anything invalid is refused whole. It is pure but for a RealPath callback.
package bridge

import (
	"errors"
	"math"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxMenuItems   = 32
	maxItemID      = 64
	maxItemLabel   = 128
	maxAccelerator = 40
	maxPathLength  = 4096
)

// Reveal refusals, shown without blocking.
var (
	ErrNotServed          = errors.New("The project is not in a folder servef serves.")
	ErrProjectUnavailable = errors.New("The project is unavailable.")
	ErrPathMissing        = errors.New("The file no longer exists.")
	ErrPathEscapes        = errors.New("The path leads outside its project.")
)

// Item is one validated row of a context menu.
type Item struct {
	Separator bool
	ID        string
	Label     string
	Enabled   bool
	// Accelerator is a display hint, never registered: the page owns its keys.
	Accelerator string
}

// Point is a validated menu position in the page.
type Point struct{ X, Y int }

var (
	itemID      = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
	accelerator = regexp.MustCompile("^(?:(?:CommandOrControl|CmdOrCtrl|Command|Cmd|Control|Ctrl|Alt|Option|AltGr|Shift|Super|Meta)\\+)*(?:[A-Z0-9]|F(?:[1-9]|1[0-9]|2[0-4])|Plus|Space|Tab|Backspace|Delete|Insert|Return|Enter|Up|Down|Left|Right|Home|End|PageUp|PageDown|Escape|Esc|[,./;'`=\\[\\]\\\\-])$")
)

// IsAppPage reports the app's own page: the origin and the routes the web
// app runs on. Bytes served from the same origin (/api/raw) get no bridge.
func IsAppPage(raw, scheme, host string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != scheme || u.Host != host || u.User != nil {
		return false
	}
	return u.Path == "" || u.Path == "/" || u.Path == "/view"
}

// ParsePoint validates a menu point: finite numbers inside the content size,
// rounded to integers.
func ParsePoint(x, y any, width, height int) (Point, bool) {
	fx, okX := x.(float64)
	fy, okY := y.(float64)
	if !okX || !okY || math.IsNaN(fx) || math.IsNaN(fy) || math.IsInf(fx, 0) || math.IsInf(fy, 0) {
		return Point{}, false
	}
	if fx < 0 || fy < 0 || fx > float64(width) || fy > float64(height) {
		return Point{}, false
	}
	return Point{int(math.Round(fx)), int(math.Round(fy))}, true
}

// ParseItems validates menu items: {id, label, enabled?, accelerator?} or
// {separator: true}, ids unique. Leading, trailing, and repeated separators
// are dropped; a menu needs at least one item. Anything else is refused.
func ParseItems(value any) ([]Item, bool) {
	raw, ok := value.([]any)
	if !ok || len(raw) == 0 || len(raw) > maxMenuItems {
		return nil, false
	}
	items := make([]Item, 0, len(raw))
	ids := map[string]bool{}
	for _, entry := range raw {
		item, ok := parseItem(entry)
		if !ok {
			return nil, false
		}
		if item.Separator {
			if len(items) > 0 && !items[len(items)-1].Separator {
				items = append(items, item)
			}
			continue
		}
		if ids[item.ID] {
			return nil, false
		}
		ids[item.ID] = true
		items = append(items, item)
	}
	if len(items) > 0 && items[len(items)-1].Separator {
		items = items[:len(items)-1]
	}
	return items, len(items) > 0
}

func parseItem(value any) (Item, bool) {
	record, ok := value.(map[string]any)
	if !ok {
		return Item{}, false
	}
	if separator, ok := record["separator"].(bool); ok && separator {
		return Item{Separator: true}, len(record) == 1
	}
	id, okID := record["id"].(string)
	label, okLabel := record["label"].(string)
	if !okID || !okLabel || len(id) > maxItemID || !itemID.MatchString(id) {
		return Item{}, false
	}
	if strings.TrimSpace(label) == "" || utf8.RuneCountInString(label) > maxItemLabel || hasControl(label) {
		return Item{}, false
	}
	item := Item{ID: id, Label: label, Enabled: true}
	if enabled, present := record["enabled"]; present {
		flag, ok := enabled.(bool)
		if !ok {
			return Item{}, false
		}
		item.Enabled = flag
	}
	if hint, present := record["accelerator"]; present {
		text, ok := hint.(string)
		if !ok || len(text) > maxAccelerator || !accelerator.MatchString(text) {
			return Item{}, false
		}
		item.Accelerator = text
	}
	return item, true
}

func hasControl(text string) bool {
	return strings.ContainsFunc(text, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// RealPath resolves symlinks of an existing path, or fails.
type RealPath func(name string) (string, error)

// ParseReveal validates {project, path}: project absolute and normalized,
// path relative with "/" separators, no leading "/", backslash, empty, ".",
// or ".." segment; a directory's trailing "/" is allowed.
func ParseReveal(project, relative any) (string, string, bool) {
	p, okP := project.(string)
	r, okR := relative.(string)
	if !okP || !okR || !isPlainAbsolute(p) || segments(r) == nil {
		return "", "", false
	}
	return p, r, true
}

func isPlainAbsolute(name string) bool {
	if len(name) > maxPathLength || strings.ContainsRune(name, 0) || !filepath.IsAbs(name) || filepath.Clean(name) != name {
		return false
	}
	return true
}

func segments(name string) []string {
	if name == "" || len(name) > maxPathLength || strings.ContainsRune(name, 0) || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return nil
	}
	parts := strings.Split(strings.TrimSuffix(name, "/"), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil
		}
	}
	return parts
}

// within reports child at or below parent; both are real paths.
func within(parent, child string) bool {
	if child == parent {
		return true
	}
	prefix := parent
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(child, prefix)
}

// ResolveReveal returns the real path to reveal: the project's real path
// must lie in a root's real path, and the target's inside the project.
func ResolveReveal(roots []string, project, relative string, realpath RealPath) (string, error) {
	parts := segments(relative)
	if parts == nil {
		return "", ErrPathEscapes
	}
	real, err := realpath(project)
	if err != nil {
		return "", ErrProjectUnavailable
	}
	served := false
	for _, root := range roots {
		// A root missing on disk serves nothing.
		if resolved, err := realpath(root); err == nil && within(resolved, real) {
			served = true
			break
		}
	}
	if !served {
		return "", ErrNotServed
	}
	target, err := realpath(filepath.Join(append([]string{real}, parts...)...))
	if err != nil {
		return "", ErrPathMissing
	}
	if !within(real, target) {
		return "", ErrPathEscapes
	}
	return target, nil
}
