package svgdraw

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Doc is a parsed SVG document, ready to draw.
type Doc struct {
	root *node
	// viewBox, and the size the document gives itself.
	vx, vy, vw, vh float64
	width, height  float64
	lists          []*list
	// err is the first failure drawing it; a failed document draws
	// nothing more.
	err error
}

// Style is how a document draws beyond its own markup.
type Style struct {
	// Vars are custom properties for the root element, as a style sheet
	// gives them (".ondine { --ondine-bg: #fff }"): the document's own
	// declarations win over them, and its var() references see them.
	Vars map[string]string
	// Font, when set, replaces every font family the document names.
	Font string
	// Color is currentColor at the root, as the text color around an
	// inline SVG.
	Color ui.Color
}

// Parse parses an SVG document. Malformed input is an error, never a
// panic.
func Parse(svg string) (doc *Doc, err error) {
	defer func() {
		if r := recover(); r != nil {
			doc, err = nil, fmt.Errorf("svgdraw: parse: %v", r)
		}
	}()
	root, err := parse([]byte(svg))
	if err != nil {
		return nil, err
	}
	d := &Doc{root: root}
	d.width, _ = number(root.props["width"])
	d.height, _ = number(root.props["height"])
	if vb := numbers(root.props["viewBox"]); len(vb) == 4 && vb[2] > 0 && vb[3] > 0 {
		d.vx, d.vy, d.vw, d.vh = vb[0], vb[1], vb[2], vb[3]
	} else {
		d.vw, d.vh = d.width, d.height
	}
	if d.width <= 0 || d.height <= 0 {
		d.width, d.height = d.vw, d.vh
	}
	return d, nil
}

// Size is the size the document gives itself, in DIPs (its width and
// height, else its viewBox's).
func (d *Doc) Size() (w, h float32) { return float32(d.width), float32(d.height) }

// computed is an element's computed style.
type computed struct {
	fill, stroke          paint
	fillOpacity, strokeOp float64
	opacity               float64 // the product of the ancestors' and its own
	strokeWidth           float64
	cap                   ui.LineCap
	join                  ui.LineJoin
	miter                 float64
	dash                  []float64
	dashOffset            float64
	fontSize              float64
	fontWeight            int
	fontFamily            string
	anchor                string
	color                 ui.Color
	preserve              bool
	visible               bool
	ctm                   affine
}

// op is one thing to paint, in the root's user space.
type op struct {
	shape        shape
	fill, stroke ui.Color
	hasFill      bool
	hasStroke    bool
	width        float64
	cap          ui.LineCap
	join         ui.LineJoin
	miter        float64
	dash         []float64
	dashOffset   float64
	text         *textOp
}

// textOp is a text element: its chunks, each starting at a point in the
// root's user space, of pieces in their own styles.
type textOp struct {
	chunks   []textChunk
	rotation float64 // degrees
}

type textChunk struct {
	x, y   float64
	anchor string
	pieces []textPiece
}

type textPiece struct {
	text   string
	size   float64 // in the root's user space
	weight int
	family string
	color  ui.Color
	// glyphs shaped at shapedSize DIPs, kept between frames.
	glyphs     []ui.Glyph
	shapedSize float32
	width      float32
}

// list is a document compiled for one Style.
type list struct {
	key string
	ops []op
}

func styleKey(s *Style) string {
	if s == nil {
		return ""
	}
	names := make([]string, 0, len(s.Vars))
	for name := range s.Vars {
		names = append(names, name)
	}
	slices.Sort(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(name + ":" + s.Vars[name] + ";")
	}
	c := s.Color
	b.WriteString("font:" + s.Font + ";color:" + string([]byte{c.R, c.G, c.B, c.A}))
	return b.String()
}

// compiled returns the document compiled for s, keeping the last two
// styles (a light and a dark theme) compiled.
func (d *Doc) compiled(s *Style) *list {
	key := styleKey(s)
	for _, l := range d.lists {
		if l.key == key {
			return l
		}
	}
	var vars map[string]string
	var font string
	color := ui.RGB(0, 0, 0)
	if s != nil {
		vars, font, color = s.Vars, s.Font, s.Color
	}
	c := compiler{r: resolver{vars: vars}, font: font}
	initial := computed{
		fill: paint{set: true, color: ui.RGB(0, 0, 0)}, fillOpacity: 1, strokeOp: 1, opacity: 1,
		strokeWidth: 1, cap: ui.ButtCap, join: ui.MiterJoin, miter: 4, fontSize: 16, fontWeight: 400,
		fontFamily: "monospace", anchor: "start", color: color, visible: true, ctm: identity,
	}
	if font != "" {
		initial.fontFamily = font
	}
	c.element(d.root, initial, true)
	l := &list{key: key, ops: c.ops}
	d.lists = append([]*list{l}, d.lists...)
	if len(d.lists) > 2 {
		d.lists = d.lists[:2]
	}
	return l
}

type compiler struct {
	r    resolver
	font string
	ops  []op
}

// value returns n's resolved value of a property, if it has a valid one.
func (c *compiler) value(n *node, name string) (string, bool) {
	raw, ok := n.props[name]
	if !ok {
		return "", false
	}
	v, ok := c.r.substitute(raw, n, 0)
	if !ok || strings.TrimSpace(v) == "" || v == "inherit" {
		return "", false
	}
	return strings.TrimSpace(v), true
}

// style computes n's style from its parent's.
func (c *compiler) style(n *node, parent computed) computed {
	s := parent
	s.dash = parent.dash
	if v, ok := c.value(n, "color"); ok {
		if col, ok := parseColor(v, parent.color); ok {
			s.color = col
		}
	}
	if v, ok := c.value(n, "fill"); ok {
		if p, ok := parsePaint(v, s.color); ok {
			s.fill = p
		}
	}
	if v, ok := c.value(n, "stroke"); ok {
		if p, ok := parsePaint(v, s.color); ok {
			s.stroke = p
		}
	}
	opacity := func(name string, dst *float64, base float64) {
		if v, ok := c.value(n, name); ok {
			if f, ok := number(strings.TrimSuffix(v, "%")); ok {
				if strings.HasSuffix(v, "%") {
					f /= 100
				}
				*dst = base * math.Max(0, math.Min(1, f))
			}
		}
	}
	opacity("fill-opacity", &s.fillOpacity, 1)
	opacity("stroke-opacity", &s.strokeOp, 1)
	opacity("opacity", &s.opacity, parent.opacity)
	if v, ok := c.value(n, "stroke-width"); ok {
		if f, ok := number(v); ok && f >= 0 {
			s.strokeWidth = f
		}
	}
	if v, ok := c.value(n, "stroke-linecap"); ok {
		switch v {
		case "butt":
			s.cap = ui.ButtCap
		case "round":
			s.cap = ui.RoundCap
		case "square":
			s.cap = ui.SquareCap
		}
	}
	if v, ok := c.value(n, "stroke-linejoin"); ok {
		switch v {
		case "miter", "miter-clip", "arcs":
			s.join = ui.MiterJoin
		case "round":
			s.join = ui.RoundJoin
		case "bevel":
			s.join = ui.BevelJoin
		}
	}
	if v, ok := c.value(n, "stroke-miterlimit"); ok {
		if f, ok := number(v); ok && f >= 1 {
			s.miter = f
		}
	}
	if v, ok := c.value(n, "stroke-dasharray"); ok {
		if v == "none" {
			s.dash = nil
		} else if d := numbers(v); d != nil {
			s.dash = d
		}
	}
	if v, ok := c.value(n, "stroke-dashoffset"); ok {
		if f, ok := number(v); ok {
			s.dashOffset = f
		}
	}
	if v, ok := c.value(n, "font"); ok {
		for _, part := range strings.Fields(v) {
			if f, ok := number(part); ok && strings.HasSuffix(part, "px") {
				s.fontSize = f
			} else if w := fontWeight(part); w > 0 && (part == "bold" || part == "normal") {
				s.fontWeight = w
			}
		}
		if i := strings.LastIndex(v, "px "); i >= 0 && c.font == "" {
			s.fontFamily = fontFamily(v[i+3:])
		}
	}
	if v, ok := c.value(n, "font-size"); ok {
		if f, ok := number(v); ok && f > 0 {
			s.fontSize = f
		}
	}
	if v, ok := c.value(n, "font-weight"); ok {
		if w := fontWeight(v); w > 0 {
			s.fontWeight = w
		}
	}
	if v, ok := c.value(n, "font-family"); ok && c.font == "" {
		if f := fontFamily(v); f != "" {
			s.fontFamily = f
		}
	}
	if v, ok := c.value(n, "text-anchor"); ok && (v == "start" || v == "middle" || v == "end") {
		s.anchor = v
	}
	if v, ok := c.value(n, "space"); ok {
		s.preserve = v == "preserve"
	}
	if v, ok := c.value(n, "display"); ok && v == "none" {
		s.visible = false
	}
	if v, ok := c.value(n, "visibility"); ok {
		s.visible = v == "visible"
	}
	if v, ok := c.value(n, "transform"); ok {
		if m, ok := parseTransform(v); ok {
			s.ctm = m.then(parent.ctm)
		}
	}
	return s
}

// num returns a geometry attribute of n, or 0.
func (c *compiler) num(n *node, name string) float64 {
	v, ok := c.value(n, name)
	if !ok {
		return 0
	}
	f, _ := number(v)
	return f
}

func (c *compiler) element(n *node, parent computed, root bool) {
	s := c.style(n, parent)
	if n.props["display"] == "none" {
		return
	}
	switch n.tag {
	case "svg":
		if !root {
			s.ctm = c.viewport(n).then(s.ctm)
		}
		fallthrough
	case "g", "a", "switch":
		for _, child := range n.children {
			c.element(child, s, false)
		}
	case "rect":
		var sh shape
		w, h := c.num(n, "width"), c.num(n, "height")
		if w <= 0 || h <= 0 {
			return
		}
		rx, okx := c.value(n, "rx")
		ry, oky := c.value(n, "ry")
		fx, _ := number(rx)
		fy, _ := number(ry)
		if !okx || rx == "auto" {
			fx = fy
		}
		if !oky || ry == "auto" {
			fy = fx
		}
		roundedRect(&sh, c.num(n, "x"), c.num(n, "y"), w, h, fx, fy)
		c.shape(sh, s)
	case "circle":
		var sh shape
		if r := c.num(n, "r"); r > 0 {
			ellipse(&sh, c.num(n, "cx"), c.num(n, "cy"), r, r)
			c.shape(sh, s)
		}
	case "ellipse":
		var sh shape
		if rx, ry := c.num(n, "rx"), c.num(n, "ry"); rx > 0 && ry > 0 {
			ellipse(&sh, c.num(n, "cx"), c.num(n, "cy"), rx, ry)
			c.shape(sh, s)
		}
	case "line":
		var sh shape
		sh.move(c.num(n, "x1"), c.num(n, "y1"))
		sh.line(c.num(n, "x2"), c.num(n, "y2"))
		s.fill = paint{}
		c.shape(sh, s)
	case "polyline", "polygon":
		pts := numbers(n.props["points"])
		if len(pts) < 4 {
			return
		}
		var sh shape
		sh.move(pts[0], pts[1])
		for i := 2; i+1 < len(pts); i += 2 {
			sh.line(pts[i], pts[i+1])
		}
		if n.tag == "polygon" {
			sh.close()
		}
		c.shape(sh, s)
	case "path":
		c.shape(pathData(n.props["d"]), s)
	case "text":
		c.text(n, s)
	}
}

// viewport maps a nested svg's viewBox into its x, y, width, and height,
// as preserveAspectRatio's default (xMidYMid meet) does.
func (c *compiler) viewport(n *node) affine {
	x, y, w, h := c.num(n, "x"), c.num(n, "y"), c.num(n, "width"), c.num(n, "height")
	vb := numbers(n.props["viewBox"])
	if len(vb) != 4 || vb[2] <= 0 || vb[3] <= 0 || w <= 0 || h <= 0 {
		return affine{a: 1, d: 1, e: x, f: y}
	}
	k := math.Min(w/vb[2], h/vb[3])
	ox, oy := x+(w-vb[2]*k)/2-vb[0]*k, y+(h-vb[3]*k)/2-vb[1]*k
	return affine{a: k, d: k, e: ox, f: oy}
}

func (c *compiler) shape(sh shape, s computed) {
	if len(sh) == 0 || !s.visible {
		return
	}
	o := op{shape: sh.transformed(s.ctm), cap: s.cap, join: s.join, miter: s.miter}
	if s.fill.set {
		o.hasFill = true
		o.fill = s.fill.color.Alpha(float32(s.fillOpacity * s.opacity))
	}
	if s.stroke.set && s.strokeWidth > 0 {
		o.hasStroke = true
		o.stroke = s.stroke.color.Alpha(float32(s.strokeOp * s.opacity))
		k := s.ctm.scale()
		o.width = s.strokeWidth * k
		if len(s.dash) > 0 {
			o.dash = make([]float64, len(s.dash))
			for i, d := range s.dash {
				o.dash[i] = d * k
			}
			o.dashOffset = s.dashOffset * k
		}
	}
	if o.hasFill || o.hasStroke {
		c.ops = append(c.ops, o)
	}
}

// text lays out a text element's chunks: each tspan with an x or y
// starts one; other runs continue the one before.
func (c *compiler) text(n *node, s computed) {
	if !s.visible {
		return
	}
	t := &textOp{rotation: s.ctm.rotation()}
	k := s.ctm.scale()
	x, y := firstNumber(n.props["x"]), firstNumber(n.props["y"])
	var chunk *textChunk
	start := func(cx, cy float64, anchor string) {
		px, py := s.ctm.apply(cx, cy)
		t.chunks = append(t.chunks, textChunk{x: px, y: py, anchor: anchor})
		chunk = &t.chunks[len(t.chunks)-1]
	}
	seen := map[*node]computed{}
	for _, run := range n.runs {
		rs := s
		if run.tspan != nil {
			ts, ok := seen[run.tspan]
			if !ok {
				ts = c.style(run.tspan, s)
				seen[run.tspan] = ts
				_, hasX := run.tspan.props["x"]
				_, hasY := run.tspan.props["y"]
				if hasX || hasY {
					if hasX {
						x = firstNumber(run.tspan.props["x"])
					}
					if hasY {
						y = firstNumber(run.tspan.props["y"])
					}
					start(x, y, ts.anchor)
				}
			}
			rs = ts
			if run.text == "" {
				continue
			}
		}
		if chunk == nil {
			start(x, y, s.anchor)
		}
		text := run.text
		text = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\t' || r == '\r' {
				return ' '
			}
			return r
		}, text)
		if !rs.preserve {
			for strings.Contains(text, "  ") {
				text = strings.ReplaceAll(text, "  ", " ")
			}
		}
		if text == "" || !rs.fill.set {
			continue
		}
		chunk.pieces = append(chunk.pieces, textPiece{
			text: text, size: rs.fontSize * k, weight: rs.fontWeight, family: rs.fontFamily,
			color: rs.fill.color.Alpha(float32(rs.fillOpacity * rs.opacity)),
		})
	}
	for i := range t.chunks {
		ch := &t.chunks[i]
		if len(ch.pieces) == 0 || s.preserve {
			continue
		}
		ch.pieces[0].text = strings.TrimLeft(ch.pieces[0].text, " ")
		last := &ch.pieces[len(ch.pieces)-1]
		last.text = strings.TrimRight(last.text, " ")
	}
	if len(t.chunks) > 0 {
		c.ops = append(c.ops, op{text: t})
	}
}

func firstNumber(s string) float64 {
	if v := numbers(s); len(v) > 0 {
		return v[0]
	}
	return 0
}

// Err is the failure that stopped the document drawing, or nil.
func (d *Doc) Err() error { return d.err }

// Draw draws the document into r, its viewBox scaled to fit and
// centered, as an img of that size shows it. A value it cannot draw is an
// error (kept in Err), never a panic; the document then draws nothing.
// Call it on the UI thread.
func (d *Doc) Draw(p *ui.Painter, r ui.Rect, s *Style) (err error) {
	if d.err != nil {
		return d.err
	}
	defer func() {
		if rec := recover(); rec != nil {
			d.err = fmt.Errorf("svgdraw: draw: %v", rec)
			d.lists = nil
			err = d.err
		}
	}()
	d.draw(p, r, s)
	return nil
}

func (d *Doc) draw(p *ui.Painter, r ui.Rect, s *Style) {
	if d.vw <= 0 || d.vh <= 0 || r.W <= 0 || r.H <= 0 {
		return
	}
	l := d.compiled(s)
	k := math.Min(float64(r.W)/d.vw, float64(r.H)/d.vh)
	ox := float64(r.X) + (float64(r.W)-d.vw*k)/2 - d.vx*k
	oy := float64(r.Y) + (float64(r.H)-d.vh*k)/2 - d.vy*k
	at := func(x, y float64) (float32, float32) { return float32(x*k + ox), float32(y*k + oy) }
	var path ui.Path
	for i := range l.ops {
		o := &l.ops[i]
		if o.text != nil {
			drawText(p, o.text, k, at)
			continue
		}
		path.Reset()
		for _, seg := range o.shape {
			switch seg.op {
			case 'M':
				path.MoveTo(at(seg.pts[0][0], seg.pts[0][1]))
			case 'L':
				path.LineTo(at(seg.pts[0][0], seg.pts[0][1]))
			case 'C':
				x1, y1 := at(seg.pts[0][0], seg.pts[0][1])
				x2, y2 := at(seg.pts[1][0], seg.pts[1][1])
				x3, y3 := at(seg.pts[2][0], seg.pts[2][1])
				path.CubeTo(x1, y1, x2, y2, x3, y3)
			case 'Z':
				path.Close()
			}
		}
		if o.hasFill {
			p.FillPath(&path, o.fill)
		}
		if o.hasStroke {
			path.Cap(o.cap).Join(o.join).MiterLimit(float32(o.miter))
			if len(o.dash) > 0 {
				dash, ok := make([]float32, len(o.dash)), true
				for i, v := range o.dash {
					dash[i] = float32(v * k)
					ok = ok && finite32(dash[i]) && dash[i] >= 0
				}
				if offset := float32(o.dashOffset * k); ok && finite32(offset) {
					path.Dash(offset, dash...)
				}
			}
			p.StrokePath(&path, float32(o.width*k), o.stroke)
		}
	}
}

func drawText(p *ui.Painter, t *textOp, k float64, at func(x, y float64) (float32, float32)) {
	for i := range t.chunks {
		ch := &t.chunks[i]
		var width float32
		for j := range ch.pieces {
			pc := &ch.pieces[j]
			size := float32(pc.size * k)
			if pc.glyphs == nil || pc.shapedSize != size {
				pc.glyphs = ui.Shape(pc.text, ui.Font{Family: pc.family, Size: size, Weight: pc.weight})
				pc.shapedSize, pc.width = size, 0
				if n := len(pc.glyphs); n > 0 {
					for _, g := range pc.glyphs {
						pc.width = max(pc.width, g.X+g.Advance)
					}
				}
			}
			width += pc.width
		}
		x, y := at(ch.x, ch.y)
		pen := x
		switch ch.anchor {
		case "middle":
			pen -= width / 2
		case "end":
			pen -= width
		}
		paint := func() {
			for j := range ch.pieces {
				pc := &ch.pieces[j]
				p.Glyphs(pc.glyphs, pen, y, pc.color)
				pen += pc.width
			}
		}
		if math.Abs(t.rotation) > 0.01 {
			p.Transform(ui.Rotation(float32(t.rotation)), x, y, paint)
		} else {
			paint()
		}
	}
}

func finite32(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }
