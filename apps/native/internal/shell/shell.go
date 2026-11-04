// Package shell holds the workspace shell's decisions as the web's
// use-sidebar.ts, use-sidebar-resize.ts, Sidebar.tsx, and
// empty-workspace.ts make them (decisions 0020, 0052): the activity bar's
// views, the sidebar's visibility and width, and what the document pane
// shows with no tab open. Native has no phone layout. It is pure.
package shell

import (
	"strconv"

	"github.com/flexdinesh/servef/apps/native/internal/memory"
)

// View is a sidebar view.
type View string

const (
	Explorer      View = "explorer"
	Search        View = "search"
	SourceControl View = "source-control"
	Walkthroughs  View = "walkthroughs"
)

// Activity is an activity bar item.
type Activity struct {
	View  View
	Label string
	// Shortcut labels its key binding, "" for none.
	Shortcut string
}

// Activities are the activity bar's items, in order, with the platform's
// shortcut labels.
func Activities(mac bool) []Activity {
	pick := func(macLabel, other string) string {
		if mac {
			return macLabel
		}
		return other
	}
	return []Activity{
		{View: Explorer, Label: "Explorer", Shortcut: pick("⇧⌘E", "Ctrl+Shift+E")},
		{View: Search, Label: "Search", Shortcut: pick("⇧⌘F", "Ctrl+Shift+F")},
		{View: SourceControl, Label: "Source Control", Shortcut: pick("⌃⇧G", "Ctrl+Shift+G")},
		{View: Walkthroughs, Label: "Walkthroughs"},
	}
}

// Name names a view as the web's sidebarViewName.
func (v View) Name() string {
	switch v {
	case Search:
		return "Search"
	case SourceControl:
		return "Source Control"
	case Walkthroughs:
		return "Walkthroughs"
	}
	return "Files"
}

// Badge is an activity's count as VS Code caps it, "" for none.
func Badge(count int) string {
	switch {
	case count <= 0:
		return ""
	case count > 99:
		return "99+"
	}
	return strconv.Itoa(count)
}

// BadgeLabel names an activity with its count for assistive technology,
// as the web's activity labels.
func BadgeLabel(activity Activity, count int) string {
	if count <= 0 || activity.View != SourceControl && activity.View != Walkthroughs {
		return activity.Label
	}
	noun := "pending change"
	if activity.View == Walkthroughs {
		noun = "new walkthrough"
	}
	if count != 1 {
		noun += "s"
	}
	return activity.Label + ", " + Count(count) + " " + noun
}

// Sidebar widths in DIPs, as use-sidebar-resize.ts.
const (
	DefaultWidth float32 = 288
	MinWidth     float32 = 208
	MaxWidth     float32 = 480
	// WidthStep is a keyboard resize step.
	WidthStep float32 = 16
)

// ClampWidth keeps a width within the sidebar's bounds.
func ClampWidth(width float32) float32 { return min(MaxWidth, max(MinWidth, width)) }

// Sidebar is the activity bar's view, whether the sidebar is hidden, and
// its width.
type Sidebar struct {
	View   View
	Hidden bool
	Width  float32
}

// FromMemory reads the remembered sidebar; unknown views open the
// Explorer and widths outside the bounds are clamped, as parseSidebar and
// readWidth.
func FromMemory(m memory.Sidebar) Sidebar {
	s := Sidebar{View: Explorer, Hidden: m.Hidden, Width: DefaultWidth}
	switch View(m.View) {
	case Explorer, Search, SourceControl, Walkthroughs:
		s.View = View(m.View)
	}
	if m.Width > 0 {
		s.Width = ClampWidth(m.Width)
	}
	return s
}

// Memory is the sidebar to remember.
func (s Sidebar) Memory() memory.Sidebar {
	return memory.Sidebar{View: string(s.View), Hidden: s.Hidden, Width: s.Width}
}

// Select is an activity bar click: the shown view hides the sidebar, as
// in VS Code; any other shows it with that view.
func (s Sidebar) Select(view View) Sidebar {
	if view == s.View && !s.Hidden {
		s.Hidden = true
		return s
	}
	return s.Reveal(view)
}

// Reveal shows the sidebar with view, as Cmd/Ctrl+Shift+E and +G do.
func (s Sidebar) Reveal(view View) Sidebar {
	s.View, s.Hidden = view, false
	return s
}

// Toggle hides or shows the sidebar, as Cmd/Ctrl+B does.
func (s Sidebar) Toggle() Sidebar {
	s.Hidden = !s.Hidden
	return s
}

// Shown is the visible view, "" when the sidebar is hidden.
func (s Sidebar) Shown() View {
	if s.Hidden {
		return ""
	}
	return s.View
}

// Count formats a count with thousands separators, as toLocaleString.
func Count(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + Count(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
