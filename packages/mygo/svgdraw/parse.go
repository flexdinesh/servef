// Package svgdraw draws SVG documents, such as the diagrams Ondine makes,
// with mygo's Painter: shapes, paths (with arcs), groups, nested svg
// elements, transforms, fills, strokes, dashes, opacity, and text (with
// tspans, text-anchor, and the font the caller picks), which mygo's own SVG
// renderer leaves out.
//
// CSS custom properties (var() with fallbacks) and color-mix(in srgb, ...)
// resolve when drawing, against the document's own declarations and the
// caller's Style.Vars, so a theme switch changes colors without parsing
// again. Links (a) draw their content and do not navigate; title, desc,
// metadata, style sheets, and defs draw nothing.
package svgdraw

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
)

// node is an element: its tag, its presentation attributes overlaid with
// the declarations of its style attribute (which win), the custom
// properties it declares, and its children. Text is the character data
// of text and tspan elements, in order with their tspans.
type node struct {
	tag      string
	props    map[string]string
	vars     map[string]string
	parent   *node
	children []*node
	// text runs of a text element, or of a tspan (one run).
	runs []textRun
}

// textRun is character data and the tspan it belongs to (nil for the
// text element's own).
type textRun struct {
	text  string
	tspan *node
}

// skipped draws nothing, and nothing inside it.
var skipped = map[string]bool{"title": true, "desc": true, "metadata": true, "style": true, "defs": true, "script": true, "foreignObject": true, "clipPath": true, "mask": true, "marker": true, "pattern": true, "linearGradient": true, "radialGradient": true, "symbol": true, "filter": true}

// parse reads an SVG document into its root svg element.
func parse(data []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	var root, cur *node
	var textOwner *node // the text element whose runs collect character data
	skip := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if skip > 0 || skipped[t.Name.Local] {
				skip++
				continue
			}
			n := &node{tag: t.Name.Local, parent: cur}
			for _, a := range t.Attr {
				if a.Name.Space != "" && a.Name.Space != "http://www.w3.org/2000/svg" {
					if a.Name.Local != "space" { // xml:space
						continue
					}
				}
				n.set(a.Name.Local, a.Value)
			}
			if style, ok := n.props["style"]; ok {
				delete(n.props, "style")
				for _, decl := range splitDeclarations(style) {
					n.set(decl[0], decl[1])
				}
			}
			if cur == nil {
				if n.tag != "svg" {
					return nil, errors.New("svgdraw: the root is not an svg element")
				}
				root = n
			} else {
				cur.children = append(cur.children, n)
			}
			switch n.tag {
			case "text":
				textOwner = n
			case "tspan":
				if textOwner != nil {
					textOwner.runs = append(textOwner.runs, textRun{tspan: n})
				}
			}
			cur = n
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			if cur == nil {
				continue
			}
			if cur.tag == "text" {
				textOwner = nil
			}
			cur = cur.parent
		case xml.CharData:
			if skip > 0 || textOwner == nil || cur == nil {
				continue
			}
			var span *node
			if cur.tag == "tspan" {
				span = cur
			}
			textOwner.runs = append(textOwner.runs, textRun{text: string(t), tspan: span})
		}
	}
	if root == nil {
		return nil, errors.New("svgdraw: no svg element")
	}
	return root, nil
}

// set records a property, or a custom property when it starts with --.
func (n *node) set(name, value string) {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if strings.HasPrefix(name, "--") {
		if n.vars == nil {
			n.vars = map[string]string{}
		}
		n.vars[name] = value
		return
	}
	if n.props == nil {
		n.props = map[string]string{}
	}
	n.props[name] = value
}

// splitDeclarations splits "a:b;c:d" into name and value pairs, keeping
// colons and semicolons inside parentheses and quotes.
func splitDeclarations(style string) [][2]string {
	var out [][2]string
	depth, quote, start := 0, byte(0), 0
	flush := func(end int) {
		decl := style[start:end]
		if i := strings.IndexByte(decl, ':'); i > 0 {
			out = append(out, [2]string{decl[:i], strings.TrimSuffix(strings.TrimSpace(decl[i+1:]), "!important")})
		}
	}
	for i := 0; i < len(style); i++ {
		switch c := style[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ';' && depth == 0:
			flush(i)
			start = i + 1
		}
	}
	flush(len(style))
	return out
}

// number parses a number with an optional px unit, rem as 16 px; ok is
// false for anything else (percentages, other units).
func number(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	scale := 1.0
	switch {
	case strings.HasSuffix(s, "px"):
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "rem"):
		s, scale = s[:len(s)-3], 16
	case strings.HasSuffix(s, "em"):
		s, scale = s[:len(s)-2], 16
	}
	v, err := parseFinite(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return v * scale, true
}

// parseFinite parses a finite float: NaN and infinities (which
// strconv accepts) are errors, so no drawing call sees them.
func parseFinite(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err == nil && (math.IsNaN(v) || math.IsInf(v, 0)) {
		err = strconv.ErrRange
	}
	return v, err
}

// numbers parses a list of numbers separated by commas or spaces.
func numbers(s string) []float64 {
	var out []float64
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' }) {
		v, ok := number(f)
		if !ok {
			return nil
		}
		out = append(out, v)
	}
	return out
}
