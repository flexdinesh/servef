package trees

import "github.com/egoist/mygo/ui"

// Status is a row's git status, as Pierre's GitStatus.
type Status string

// The git statuses Pierre draws. Ignored shows no letter: the row takes
// the ignored color and its icon half opacity, and a directory's status
// passes to everything inside it.
const (
	Added     Status = "added"
	Deleted   Status = "deleted"
	Ignored   Status = "ignored"
	Modified  Status = "modified"
	Renamed   Status = "renamed"
	Untracked Status = "untracked"
)

// Letter is the git lane's letter, "" for none.
func (s Status) Letter() string {
	switch s {
	case Added:
		return "A"
	case Deleted:
		return "D"
	case Modified:
		return "M"
	case Renamed:
		return "R"
	case Untracked:
		return "U"
	}
	return ""
}

// Title is the git lane's tooltip, as Pierre's.
func (s Status) Title() string {
	if s == "" {
		return ""
	}
	return "Git status: " + string(s)
}

// Theme is the tree's look. DefaultTheme is Pierre's; hosts map their own
// tokens onto it, as a web app overrides Pierre's --trees-* properties.
type Theme struct {
	Dark bool
	// Background is behind the rows (Transparent keeps what is behind the
	// tree); Hover is a hovered row's.
	Background ui.Color
	Hover      ui.Color
	// Text colors names; Muted chevrons and decorations.
	Text  ui.Color
	Muted ui.Color
	// Selected and SelectedText are the selected row's fill and ink;
	// SelectedBorder, when set, outlines it (servef adds it: the tint
	// alone is under 3:1).
	Selected       ui.Color
	SelectedText   ui.Color
	SelectedBorder ui.Color
	// Focus rings the keyboard cursor while the tree has the focus.
	Focus ui.Color
	// Status colors each git status's lane letter and name.
	Status map[Status]ui.Color
	// FontSize is the rows' text size; RowHeight their height.
	FontSize  float32
	RowHeight float32
	// Density scales Pierre's paddings and gaps (compact is 0.8).
	Density float32
	// Radius rounds rows' corners.
	Radius float32
	// PaddingX insets the rows from the tree's sides.
	PaddingX float32
}

// defaultThemes are DefaultTheme's, made once, for View's fallback; never
// handed out, since hosts change a Theme's Status map.
var defaultThemes = [2]Theme{DefaultTheme(false), DefaultTheme(true)}

// DefaultTheme is Pierre's look in the light or dark appearance, at its
// compact density. Each call makes a new Status map, the host's to
// change.
func DefaultTheme(dark bool) Theme {
	pick := func(pair [2]string) ui.Color {
		if dark {
			return ui.Hex(pair[1])
		}
		return ui.Hex(pair[0])
	}
	t := Theme{
		Dark:       dark,
		Background: pick([2]string{"#f8f8f8", "#141415"}),
		Text:       pick([2]string{"#6c6c71", "#adadb1"}),
		Muted:      ui.Hex("#84848a"),
		Focus:      ui.Hex("#009fff"),
		Status:     map[Status]ui.Color{},
		FontSize:   13,
		RowHeight:  24,
		Density:    0.8,
		Radius:     6 * 0.8,
		PaddingX:   16,
	}
	hover, selected := float32(0.08), float32(0.12)
	if dark {
		hover, selected = 0.10, 0.15
	}
	t.Hover = mix(t.Focus, t.Background, hover)
	t.Selected = mix(t.Focus, t.Background, selected)
	t.SelectedText = t.Text
	for status, pair := range pierreStatus {
		t.Status[Status(status)] = pick(pair)
	}
	return t
}

// mix is a share of a over b, as CSS color-mix in sRGB.
func mix(a, b ui.Color, share float32) ui.Color {
	channel := func(x, y uint8) uint8 { return uint8(float32(x)*share + float32(y)*(1-share) + 0.5) }
	return ui.Color{R: channel(a.R, b.R), G: channel(a.G, b.G), B: channel(a.B, b.B), A: channel(a.A, b.A)}
}

// Mix is mix for hosts: share of a over b.
func Mix(a, b ui.Color, share float32) ui.Color { return mix(a, b, share) }
