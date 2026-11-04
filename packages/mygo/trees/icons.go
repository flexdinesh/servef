package trees

import (
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Icon is a resolved file icon: Pierre's token ("go", "markdown",
// "default", …), which names its SVG and its hue.
type Icon struct {
	Token string
}

// IconFor resolves a file's icon by its name, as Pierre's complete set
// does: the exact lower-case base name, then each extension from the
// longest compound one ("d.ts" before "ts"), with the complete set's
// overrides (jsx, tsx, sass, scss) first; unknown names get "default".
// It is presentation by name, never a file-type decision.
func IconFor(name string) Icon {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	iconMu.Lock()
	icon, ok := iconCache[name]
	iconMu.Unlock()
	if ok {
		return icon
	}
	icon = resolveIcon(name)
	iconMu.Lock()
	if len(iconCache) >= iconCacheSize {
		clear(iconCache)
	}
	iconCache[name] = icon
	iconMu.Unlock()
	return icon
}

// iconCache keeps resolved names, as rows resolve theirs each frame;
// iconCacheSize bounds it.
var (
	iconMu    sync.Mutex
	iconCache = map[string]Icon{}
)

const iconCacheSize = 8192

func resolveIcon(name string) Icon {
	lower := strings.ToLower(name)
	if token, ok := pierreFileNames[lower]; ok {
		return Icon{Token: token}
	}
	segments := strings.Split(lower, ".")
	for i := 1; i < len(segments); i++ {
		extension := strings.Join(segments[i:], ".")
		if token, ok := pierreCompleteOverrides[extension]; ok {
			return Icon{Token: token}
		}
		if token, ok := pierreExtensions[extension]; ok {
			return Icon{Token: token}
		}
	}
	return Icon{Token: "default"}
}

// Color is the icon's hue in the light or dark palette; false for a
// token without one (font, nextjs, stylelint), which takes the tree's
// muted ink, as Pierre's.
func (i Icon) Color(dark bool) (ui.Color, bool) {
	hue, ok := pierreIconColors[i.Token]
	if !ok {
		return ui.Transparent, false
	}
	pair := pierrePalette[hue]
	if dark {
		return ui.Hex(pair[1]), true
	}
	return ui.Hex(pair[0]), true
}

// SVG returns the icon's shapes, drawn in one color as ui.Icon draws.
func (i Icon) SVG() *ui.SVG { return symbol(i.Token) }

var (
	svgMu    sync.Mutex
	svgCache = map[string]*ui.SVG{}
)

// symbol parses a sprite symbol once; unknown tokens get the default
// file icon.
func symbol(token string) *ui.SVG {
	svgMu.Lock()
	defer svgMu.Unlock()
	if s, ok := svgCache[token]; ok {
		return s
	}
	source, ok := pierreSymbols[token]
	if !ok {
		source = pierreSymbols["default"]
	}
	s := ui.MustParseSVG([]byte(source))
	svgCache[token] = s
	return s
}

// Chevron is Pierre's folder chevron, pointing down; a closed folder's
// turns -90 degrees.
func Chevron() *ui.SVG { return symbol("icon-chevron") }

// dot is the git lane's dot for a folder holding changes.
func dot() *ui.SVG { return symbol("icon-dot") }
