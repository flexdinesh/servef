// Package tabs holds the workspace's open tabs as the web's
// workspace-tabs.ts, tab-menu.ts, and use-document-workspace.ts do:
// documents by project-relative path, the Uncommitted review (decision
// 0035), and Welcome (decision 0052). Native opens no commit snapshots,
// other compares, or walkthroughs yet (M8, M9). It is pure.
package tabs

import (
	"encoding/json"
	"path"
)

// Kind is what a tab shows; the zero Tab is no tab.
type Kind uint8

const (
	_ Kind = iota
	// Document is a working-tree file.
	Document
	// Review is the Uncommitted review: working-tree changes since HEAD.
	Review
	// Welcome is the Welcome tab, at most one.
	Welcome
)

// Tab is an open tab. Path is a document's project-relative path.
type Tab struct {
	Kind Kind
	Path string
}

// DocumentTab is a working-tree document's tab.
func DocumentTab(path string) Tab { return Tab{Kind: Document, Path: path} }

// Uncommitted is the Uncommitted review's tab.
func Uncommitted() Tab { return Tab{Kind: Review} }

// WelcomeTab is the Welcome tab.
func WelcomeTab() Tab { return Tab{Kind: Welcome} }

// None reports the zero Tab.
func (t Tab) None() bool { return t.Kind == 0 }

// Key identifies a tab uniquely across kinds, as the web's tabKey.
func (t Tab) Key() string {
	switch t.Kind {
	case Document:
		return "document:" + t.Path
	case Review:
		return "review:working"
	case Welcome:
		return "welcome"
	}
	return ""
}

// Index is tab's position in tabs, -1 when it is not open.
func Index(tabs []Tab, tab Tab) int {
	for i, open := range tabs {
		if open == tab {
			return i
		}
	}
	return -1
}

// Includes reports whether tab is open.
func Includes(tabs []Tab, tab Tab) bool { return !tab.None() && Index(tabs, tab) >= 0 }

// Name is a tab's label and its title, which shows the full path.
type Name struct {
	Label string
	Title string
}

// Name names a tab as VS Code does: documents by file name, the review by
// what it compares.
func (t Tab) Name() Name {
	switch t.Kind {
	case Document:
		return Name{Label: FileName(t.Path), Title: t.Path}
	case Review:
		return Name{Label: "Uncommitted", Title: "Uncommitted changes"}
	case Welcome:
		return Name{Label: "Welcome", Title: "Welcome"}
	}
	return Name{}
}

// FileName is a path's last element.
func FileName(p string) string {
	if p == "" {
		return p
	}
	return path.Base(p)
}

// stored is a remembered tab: a document's path as a JSON string, or an
// object for the others, as the web's StoredTab.
type stored struct {
	Review  string `json:"review,omitempty"`
	Welcome bool   `json:"welcome,omitempty"`
}

// MarshalJSON stores a tab as the web stores it: a document as its path,
// the review as {"review":"working"}, Welcome as {"welcome":true}, and no
// tab as "".
func (t Tab) MarshalJSON() ([]byte, error) {
	switch t.Kind {
	case Document:
		return json.Marshal(t.Path)
	case Review:
		return json.Marshal(stored{Review: "working"})
	case Welcome:
		return json.Marshal(stored{Welcome: true})
	}
	return json.Marshal("")
}

// Parse reads a stored tab; it reports false for no tab and for anything
// malformed or that native does not open.
func Parse(raw json.RawMessage) (Tab, bool) {
	var p string
	if json.Unmarshal(raw, &p) == nil {
		return DocumentTab(p), p != ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return Tab{}, false
	}
	if value, ok := fields["welcome"]; ok {
		var on bool
		return WelcomeTab(), json.Unmarshal(value, &on) == nil && on
	}
	if value, ok := fields["review"]; ok {
		var kind string
		return Uncommitted(), json.Unmarshal(value, &kind) == nil && kind == "working"
	}
	return Tab{}, false
}

// Closed is what closing tabs did: the tabs left open, those closed, and
// the tab to show in place of a closed active tab, if any.
type Closed struct {
	Tabs        []Tab
	Closed      []Tab
	Replacement Tab
}

// Close closes the open tabs among closing, as the web's closeTabs does:
// a closed active tab gives way to the next open tab to its right, else to
// its left. It reports false when none of closing is open.
func Close(tabs, closing []Tab, active Tab) (Closed, bool) {
	var result Closed
	for _, tab := range tabs {
		if Includes(closing, tab) {
			result.Closed = append(result.Closed, tab)
		} else {
			result.Tabs = append(result.Tabs, tab)
		}
	}
	if len(result.Closed) == 0 {
		return Closed{}, false
	}
	index := Index(tabs, active)
	for _, tab := range tabs[index+1:] {
		if !Includes(result.Closed, tab) {
			result.Replacement = tab
			return result, true
		}
	}
	for i := max(index, 0) - 1; i >= 0; i-- {
		if !Includes(result.Closed, tabs[i]) {
			result.Replacement = tabs[i]
			break
		}
	}
	return result, true
}

// Merge opens restored tabs ahead of those opened this session, as the
// web's restore does.
func Merge(restored, current []Tab) []Tab {
	merged := append([]Tab(nil), restored...)
	for _, tab := range current {
		if !Includes(restored, tab) {
			merged = append(merged, tab)
		}
	}
	return merged
}
