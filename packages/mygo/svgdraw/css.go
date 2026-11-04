package svgdraw

import (
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/colornames"

	"github.com/egoist/mygo/ui"
)

// resolver substitutes var() references: custom properties come from the
// element's ancestors (each computed where it is declared), then from the
// caller's variables, which apply at the root as a style sheet would.
type resolver struct {
	vars map[string]string
}

// lookup returns the value of custom property name seen from n.
func (r *resolver) lookup(name string, n *node, depth int) (string, bool) {
	for at := n; at != nil; at = at.parent {
		if v, ok := at.vars[name]; ok {
			return r.substitute(v, at, depth+1)
		}
	}
	if v, ok := r.vars[name]; ok {
		return r.substitute(v, nil, depth+1)
	}
	return "", false
}

// substitute replaces every var(--name[, fallback]) in value, seen from n;
// ok is false when a reference has neither a value nor a fallback, which
// makes the declaration invalid.
func (r *resolver) substitute(value string, n *node, depth int) (string, bool) {
	if depth > 32 {
		return "", false
	}
	for {
		i := strings.Index(value, "var(")
		if i < 0 {
			return value, true
		}
		end := closing(value, i+3)
		if end < 0 {
			return "", false
		}
		inner := value[i+4 : end]
		name, fallback, hasFallback := strings.Cut(inner, ",")
		name = strings.TrimSpace(name)
		got, ok := r.lookup(name, n, depth)
		if !ok {
			if !hasFallback {
				return "", false
			}
			got, ok = r.substitute(strings.TrimSpace(fallback), n, depth+1)
			if !ok {
				return "", false
			}
		}
		value = value[:i] + got + value[end+1:]
	}
}

// closing returns the index of the parenthesis closing the one at open.
func closing(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// paint is a resolved fill or stroke: none, or a color.
type paint struct {
	set   bool // false: none
	color ui.Color
}

// parsePaint parses a resolved paint value; current is currentColor.
func parsePaint(value string, current ui.Color) (paint, bool) {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "inherit":
		return paint{}, false
	case "none", "transparent":
		return paint{}, true
	case "currentcolor":
		return paint{set: true, color: current}, true
	}
	if strings.HasPrefix(value, "url(") {
		return paint{}, true // gradients and patterns are not drawn
	}
	c, ok := parseColor(value, current)
	if !ok {
		return paint{}, false
	}
	return paint{set: true, color: c}, true
}

// parseColor parses hex, rgb(), rgba(), named colors, currentColor, and
// color-mix(in srgb, a p%, b q%).
func parseColor(value string, current ui.Color) (ui.Color, bool) {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	switch {
	case lower == "currentcolor":
		return current, true
	case lower == "transparent":
		return ui.Color{}, true
	case strings.HasPrefix(value, "#"):
		return hexColor(value[1:])
	case strings.HasPrefix(lower, "rgb"):
		open := strings.IndexByte(value, '(')
		end := closing(value, open)
		if open < 0 || end < 0 {
			return ui.Color{}, false
		}
		parts := strings.FieldsFunc(value[open+1:end], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		if len(parts) < 3 {
			return ui.Color{}, false
		}
		var ch [4]float64
		ch[3] = 1
		for i, part := range parts[:min(len(parts), 4)] {
			v, err := strconv.ParseFloat(strings.TrimSuffix(part, "%"), 64)
			if err != nil {
				return ui.Color{}, false
			}
			if strings.HasSuffix(part, "%") {
				v = v / 100
				if i < 3 {
					v *= 255
				}
			}
			ch[i] = v
		}
		return ui.RGBA(clampByte(ch[0]), clampByte(ch[1]), clampByte(ch[2]), float32(math.Max(0, math.Min(1, ch[3])))), true
	case strings.HasPrefix(lower, "color-mix("):
		return colorMix(value, current)
	}
	if c, ok := colornames.Map[lower]; ok {
		return ui.RGBA(c.R, c.G, c.B, float32(c.A)/255), true
	}
	return ui.Color{}, false
}

func clampByte(v float64) uint8 { return uint8(math.Round(math.Max(0, math.Min(255, v)))) }

func hexColor(h string) (ui.Color, bool) {
	if len(h) == 3 || len(h) == 4 {
		var b strings.Builder
		for _, c := range h {
			b.WriteRune(c)
			b.WriteRune(c)
		}
		h = b.String()
	}
	if len(h) != 6 && len(h) != 8 {
		return ui.Color{}, false
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return ui.Color{}, false
	}
	if len(h) == 6 {
		return ui.RGB(uint8(v>>16), uint8(v>>8), uint8(v)), true
	}
	return ui.RGBA(uint8(v>>24), uint8(v>>16), uint8(v>>8), float32(v&0xff)/255), true
}

// colorMix mixes two colors in sRGB, as CSS's color-mix(in srgb, a p%, b),
// with premultiplied alpha.
func colorMix(value string, current ui.Color) (ui.Color, bool) {
	open := strings.IndexByte(value, '(')
	end := closing(value, open)
	if end < 0 {
		return ui.Color{}, false
	}
	args := splitTopLevel(value[open+1:end], ',')
	if len(args) != 3 || strings.ToLower(strings.Join(strings.Fields(args[0]), " ")) != "in srgb" {
		return ui.Color{}, false
	}
	type part struct {
		color ui.Color
		pct   float64
		has   bool
	}
	parse := func(s string) (part, bool) {
		s = strings.TrimSpace(s)
		var p part
		if i := strings.LastIndexByte(s, ' '); i > 0 && strings.HasSuffix(s, "%") {
			v, err := strconv.ParseFloat(strings.TrimSuffix(s[i+1:], "%"), 64)
			if err != nil {
				return p, false
			}
			p.pct, p.has, s = v/100, true, s[:i]
		}
		c, ok := parseColor(s, current)
		p.color = c
		return p, ok
	}
	a, ok1 := parse(args[1])
	b, ok2 := parse(args[2])
	if !ok1 || !ok2 {
		return ui.Color{}, false
	}
	switch {
	case a.has && !b.has:
		b.pct = 1 - a.pct
	case b.has && !a.has:
		a.pct = 1 - b.pct
	case !a.has && !b.has:
		a.pct, b.pct = 0.5, 0.5
	}
	sum := a.pct + b.pct
	if sum <= 0 {
		return ui.Color{}, false
	}
	wa, wb := a.pct/sum, b.pct/sum
	aa, ab := float64(a.color.A)/255, float64(b.color.A)/255
	alpha := aa*wa + ab*wb
	if alpha == 0 {
		return ui.Color{}, true
	}
	mix := func(x, y uint8) uint8 {
		return clampByte((float64(x)*aa*wa + float64(y)*ab*wb) / alpha)
	}
	out := ui.RGBA(mix(a.color.R, b.color.R), mix(a.color.G, b.color.G), mix(a.color.B, b.color.B), float32(alpha*math.Min(1, sum)))
	return out, true
}

// splitTopLevel splits s at sep outside parentheses.
func splitTopLevel(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// fontFamily picks one family from a CSS font-family list: the generic
// monospace when the list has it, else its first family.
func fontFamily(list string) string {
	families := splitTopLevel(list, ',')
	for _, f := range families {
		if strings.TrimSpace(f) == "monospace" {
			return "monospace"
		}
	}
	if len(families) == 0 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(families[0]), `"'`)
}

// fontWeight parses a font-weight; 0 when it is not one.
func fontWeight(v string) int {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "normal":
		return 400
	case "bold", "bolder":
		return 700
	case "lighter":
		return 300
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 || n > 1000 {
		return 0
	}
	return n
}
