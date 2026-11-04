package highlights

import "strings"

// Setting is a theme's syntax entry for a scope, as in a Highlights
// theme's style.syntax: a color (a value, or a palette name in a
// CSS-palette theme) and font settings. A string entry is a Setting with
// only a Color.
type Setting struct {
	Color      string
	FontStyle  string // "italic"
	FontWeight int
}

// Resolve returns the setting syntax gives scope as Highlights resolves
// themes (resolveThemeSyntax): the color of the nearest scope with one,
// itself then each parent ("keyword.control", then "keyword"), and the
// font settings of the nearest entry; ok is false when no entry applies.
func Resolve(syntax map[string]Setting, scope string) (Setting, bool) {
	var settings *Setting
	for k := scope; k != ""; {
		if v, ok := syntax[k]; ok {
			if settings == nil {
				settings = &v
			}
			if v.Color != "" {
				out := *settings
				out.Color = v.Color
				return out, true
			}
		}
		dot := strings.LastIndexByte(k, '.')
		if dot < 0 {
			break
		}
		k = k[:dot]
	}
	if settings == nil {
		return Setting{}, false
	}
	return *settings, true
}
