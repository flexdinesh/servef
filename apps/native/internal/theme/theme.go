// Package theme holds servef's Light and Dark theme values (the
// light-dark() pairs of apps/web/src/styles/tokens.css) that native draws
// with. A test fails when tokens.css changes them. It knows no UI. It is
// pure.
package theme

import (
	"fmt"
	"strconv"

	"github.com/flexdinesh/servef/apps/native/internal/highlight"
)

// Palette is one appearance's colors, as "#rrggbb", by servef token name.
type Palette struct {
	Bg               string // --color-bg
	Canvas           string // --color-canvas
	Surface          string // --color-surface
	SurfaceSecondary string // --color-surface-secondary
	SurfaceRaised    string // --color-surface-raised
	SurfaceRecessed  string // --color-surface-recessed (themes.css)
	TextPrimary      string // --color-text-primary
	TextSecondary    string // --color-text-secondary
	TextMuted        string // --color-text-muted
	Border           string // --color-border
	BorderStrong     string // --color-border-strong
	Accent           string // --color-accent
	AccentSubtle     string // --color-accent-subtle
	AccentHover      string // --color-accent-hover (--color-selected-text)
	Success          string // --color-success
	Warning          string // --color-warning
	WarningSubtle    string // --color-warning-subtle
	Destructive      string // --color-destructive
	// DestructiveSubtle is --color-destructive-subtle.
	DestructiveSubtle string
	// Syntax colors each servef syntax category (--color-syntax-<name>).
	Syntax map[highlight.Category]string
}

// Light is Servef Light.
var Light = Palette{
	Bg: "#ffffff", Canvas: "#ffffff", Surface: "#ffffff", SurfaceSecondary: "#f1f2f4", SurfaceRaised: "#ffffff", SurfaceRecessed: "#eff1f4",
	TextPrimary: "#333333", TextSecondary: "#515866", TextMuted: "#5d6676",
	Border: "#d8dbe0", BorderStrong: "#c4c9d1", Accent: "#2563eb", AccentSubtle: "#e0e9fd", AccentHover: "#1d4ed8",
	Success: "#13703f", Warning: "#9a5800", WarningSubtle: "#ffeab3", Destructive: "#bd3029", DestructiveSubtle: "#fff0ef",
	Syntax: map[highlight.Category]string{
		highlight.Comment: "#57606a", highlight.Keyword: "#cf222e", highlight.Type: "#0550ae", highlight.Function: "#6639ba",
		highlight.Variable: "#953800", highlight.String: "#0a3069", highlight.Number: "#0550ae", highlight.Operator: "#0550ae",
	},
}

// Dark is Servef Dark.
var Dark = Palette{
	Bg: "#181818", Canvas: "#0b0b0b", Surface: "#1f1f1f", SurfaceSecondary: "#181818", SurfaceRaised: "#252526", SurfaceRecessed: "#181818",
	TextPrimary: "#e8eaf0", TextSecondary: "#b2b8c4", TextMuted: "#8992a1",
	Border: "#2b2b2b", BorderStrong: "#414855", Accent: "#78a8ff", AccentSubtle: "#1d3155", AccentHover: "#9bbfff",
	Success: "#62d09b", Warning: "#efb15a", WarningSubtle: "#3c2d18", Destructive: "#ff827a", DestructiveSubtle: "#422523",
	Syntax: map[highlight.Category]string{
		highlight.Comment: "#8b949e", highlight.Keyword: "#ff7b72", highlight.Type: "#f0883e", highlight.Function: "#d2a8ff",
		highlight.Variable: "#79c0ff", highlight.String: "#a5d6ff", highlight.Number: "#79c0ff", highlight.Operator: "#ff7b72",
	},
}

// For returns the palette of an appearance.
func For(dark bool) Palette {
	if dark {
		return Dark
	}
	return Light
}

// Tokens lists a palette's values by token name, for comparing with
// tokens.css.
func (p Palette) Tokens() map[string]string {
	tokens := map[string]string{
		"bg": p.Bg, "canvas": p.Canvas, "surface": p.Surface, "surface-secondary": p.SurfaceSecondary,
		"surface-raised": p.SurfaceRaised, "text-primary": p.TextPrimary, "text-secondary": p.TextSecondary,
		"text-muted": p.TextMuted, "border": p.Border, "border-strong": p.BorderStrong, "accent": p.Accent,
		"accent-subtle": p.AccentSubtle, "accent-hover": p.AccentHover, "success": p.Success, "warning": p.Warning, "warning-subtle": p.WarningSubtle, "destructive": p.Destructive, "destructive-subtle": p.DestructiveSubtle,
	}
	for _, category := range highlight.Categories {
		tokens["syntax-"+category.Name()] = p.Syntax[category]
	}
	return tokens
}

// RadiusSmall is --radius-sm.
const RadiusSmall = "0.125rem"

// Ondine is the --ondine-* custom properties a diagram takes, as
// apps/web/src/features/documents/mermaid.css maps them to servef's
// tokens; the font is the app's monospace font.
func (p Palette) Ondine() map[string]string {
	return map[string]string{
		"--ondine-bg": p.Surface, "--ondine-fg": p.TextPrimary, "--ondine-line": p.TextSecondary,
		"--ondine-border": p.TextSecondary, "--ondine-muted": p.TextMuted, "--ondine-surface": p.Surface,
		"--ondine-surface-alt": p.SurfaceSecondary, "--ondine-accent": p.Accent, "--ondine-critical": p.Destructive,
		"--ondine-font": "monospace", "--ondine-radius": RadiusSmall,
	}
}

// Chrome is --color-workspace-chrome, which themes.css sets for Servef
// Light and Dark: its own light grey, and in dark the surface mixed
// halfway toward the canvas. The tab strip and activity bar sit on it.
func (p Palette) Chrome() string {
	if p.Bg == Light.Bg && p.Canvas == Light.Canvas && p.Surface == Light.Surface {
		return ChromeLight
	}
	return Mix(p.Surface, p.Canvas, 0.5)
}

// ChromeLight is Servef Light's --color-workspace-chrome.
const ChromeLight = "#e5e7eb"

// ChromeHover is --color-chrome-hover: in light the surface mixed halfway
// toward the chrome; in dark --color-hover (8% of the text) over it.
func (p Palette) ChromeHover() string {
	if p.Chrome() == ChromeLight {
		return Mix(p.Surface, ChromeLight, 0.5)
	}
	return Mix(p.Chrome(), p.TextPrimary, 0.08)
}

// Mix is CSS color-mix in srgb of two "#rrggbb" colors: share of b.
func Mix(a, b string, share float64) string {
	ca, cb := rgb(a), rgb(b)
	var out [3]uint8
	for i := range out {
		out[i] = uint8(float64(ca[i])*(1-share) + float64(cb[i])*share + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", out[0], out[1], out[2])
}

func rgb(hex string) [3]uint8 {
	var c [3]uint8
	if len(hex) == 7 {
		for i := range c {
			v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
			c[i] = uint8(v)
		}
	}
	return c
}
