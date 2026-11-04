package svgdraw

import (
	"math"
	"strings"
)

// affine is the matrix [a c e; b d f; 0 0 1], as SVG's matrix(a b c d e f).
type affine struct{ a, b, c, d, e, f float64 }

var identity = affine{a: 1, d: 1}

// mul returns m then n: n·m.
func (m affine) then(n affine) affine {
	return affine{
		a: n.a*m.a + n.c*m.b, b: n.b*m.a + n.d*m.b,
		c: n.a*m.c + n.c*m.d, d: n.b*m.c + n.d*m.d,
		e: n.a*m.e + n.c*m.f + n.e, f: n.b*m.e + n.d*m.f + n.f,
	}
}

func (m affine) apply(x, y float64) (float64, float64) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// scale is how much m scales lengths on average (√|det|), for stroke
// widths and font sizes.
func (m affine) scale() float64 { return math.Sqrt(math.Abs(m.a*m.d - m.b*m.c)) }

// rotation is m's rotation in degrees, clockwise.
func (m affine) rotation() float64 { return math.Atan2(m.b, m.a) * 180 / math.Pi }

// parseTransform parses an SVG transform list; ok is false when it is
// malformed, which SVG treats as no transform.
func parseTransform(s string) (affine, bool) {
	m := identity
	s = strings.TrimSpace(s)
	for s != "" {
		open := strings.IndexByte(s, '(')
		end := strings.IndexByte(s, ')')
		if open < 0 || end < open {
			return identity, false
		}
		name := strings.TrimSpace(strings.Trim(s[:open], ", "))
		args := numbers(s[open+1 : end])
		s = strings.TrimSpace(strings.TrimLeft(s[end+1:], ", "))
		var t affine
		switch {
		case name == "matrix" && len(args) == 6:
			t = affine{args[0], args[1], args[2], args[3], args[4], args[5]}
		case name == "translate" && len(args) == 1:
			t = affine{a: 1, d: 1, e: args[0]}
		case name == "translate" && len(args) == 2:
			t = affine{a: 1, d: 1, e: args[0], f: args[1]}
		case name == "scale" && len(args) == 1:
			t = affine{a: args[0], d: args[0]}
		case name == "scale" && len(args) == 2:
			t = affine{a: args[0], d: args[1]}
		case name == "rotate" && (len(args) == 1 || len(args) == 3):
			sin, cos := math.Sincos(args[0] * math.Pi / 180)
			t = affine{a: cos, b: sin, c: -sin, d: cos}
			if len(args) == 3 {
				cx, cy := args[1], args[2]
				t = affine{a: 1, d: 1, e: -cx, f: -cy}.then(t).then(affine{a: 1, d: 1, e: cx, f: cy})
			}
		case name == "skewX" && len(args) == 1:
			t = affine{a: 1, d: 1, c: math.Tan(args[0] * math.Pi / 180)}
		case name == "skewY" && len(args) == 1:
			t = affine{a: 1, d: 1, b: math.Tan(args[0] * math.Pi / 180)}
		default:
			return identity, false
		}
		// transform="A B" applies B to the content first, then A.
		m = t.then(m)
	}
	return m, true
}

// segment is a move, line, cubic, or close, in document coordinates.
type segment struct {
	op  byte // 'M', 'L', 'C', 'Z'
	pts [3][2]float64
}

// shape is the outline of an element, in its user space.
type shape []segment

func (s *shape) move(x, y float64) { *s = append(*s, segment{op: 'M', pts: [3][2]float64{{x, y}}}) }
func (s *shape) line(x, y float64) { *s = append(*s, segment{op: 'L', pts: [3][2]float64{{x, y}}}) }
func (s *shape) cubic(x1, y1, x2, y2, x, y float64) {
	*s = append(*s, segment{op: 'C', pts: [3][2]float64{{x1, y1}, {x2, y2}, {x, y}}})
}
func (s *shape) close() { *s = append(*s, segment{op: 'Z'}) }

// transformed returns the shape under m.
func (s shape) transformed(m affine) shape {
	out := make(shape, len(s))
	for i, seg := range s {
		out[i].op = seg.op
		for j := range seg.pts {
			out[i].pts[j][0], out[i].pts[j][1] = m.apply(seg.pts[j][0], seg.pts[j][1])
		}
	}
	return out
}

// pathData parses SVG path data into lines and cubics; it stops at the
// first error, keeping what came before, as SVG renders paths.
func pathData(d string) shape {
	var s shape
	sc := scanner{s: d}
	var cx, cy, sx, sy float64 // current point, subpath start
	var lcx, lcy float64       // last control point, for S and T
	var lastCmd byte
	var cmd byte
	for {
		sc.skipSpace()
		if sc.done() {
			return s
		}
		if c := sc.peek(); isCommand(c) {
			cmd = c
			sc.i++
		} else if cmd == 0 {
			return s
		}
		rel := cmd >= 'a'
		abs := func(x, y float64) (float64, float64) {
			if rel {
				return cx + x, cy + y
			}
			return x, y
		}
		upper := cmd &^ 0x20
		var ok bool
		switch upper {
		case 'Z':
			s.close()
			cx, cy = sx, sy
			lastCmd = 'Z'
			cmd = 0 // Z takes no numbers
			continue
		case 'M', 'L', 'T':
			var x, y float64
			if x, y, ok = sc.pair(); !ok {
				return s
			}
			x, y = abs(x, y)
			switch upper {
			case 'M':
				s.move(x, y)
				sx, sy = x, y
				// Pairs after a moveto are linetos.
				if rel {
					cmd = 'l'
				} else {
					cmd = 'L'
				}
			case 'L':
				s.line(x, y)
			case 'T':
				qx, qy := cx, cy
				if lastCmd == 'Q' || lastCmd == 'T' {
					qx, qy = 2*cx-lcx, 2*cy-lcy
				}
				s.cubic(cx+2*(qx-cx)/3, cy+2*(qy-cy)/3, x+2*(qx-x)/3, y+2*(qy-y)/3, x, y)
				lcx, lcy = qx, qy
			}
			cx, cy = x, y
		case 'H', 'V':
			var v float64
			if v, ok = sc.num(); !ok {
				return s
			}
			if upper == 'H' {
				if rel {
					v += cx
				}
				cx = v
			} else {
				if rel {
					v += cy
				}
				cy = v
			}
			s.line(cx, cy)
		case 'C', 'S', 'Q':
			var x1, y1, x2, y2, x, y float64
			switch upper {
			case 'C':
				var v [6]float64
				if !sc.nums(v[:]) {
					return s
				}
				x1, y1 = abs(v[0], v[1])
				x2, y2 = abs(v[2], v[3])
				x, y = abs(v[4], v[5])
			case 'S':
				var v [4]float64
				if !sc.nums(v[:]) {
					return s
				}
				x1, y1 = cx, cy
				if lastCmd == 'C' || lastCmd == 'S' {
					x1, y1 = 2*cx-lcx, 2*cy-lcy
				}
				x2, y2 = abs(v[0], v[1])
				x, y = abs(v[2], v[3])
			case 'Q':
				var v [4]float64
				if !sc.nums(v[:]) {
					return s
				}
				qx, qy := abs(v[0], v[1])
				x, y = abs(v[2], v[3])
				x1, y1 = cx+2*(qx-cx)/3, cy+2*(qy-cy)/3
				x2, y2 = x+2*(qx-x)/3, y+2*(qy-y)/3
				lcx, lcy = qx, qy
			}
			s.cubic(x1, y1, x2, y2, x, y)
			if upper != 'Q' {
				lcx, lcy = x2, y2
			}
			cx, cy = x, y
		case 'A':
			var v [3]float64
			if !sc.nums(v[:]) {
				return s
			}
			large, ok1 := sc.flag()
			sweep, ok2 := sc.flag()
			x, y, ok3 := sc.pair()
			if !ok1 || !ok2 || !ok3 {
				return s
			}
			x, y = abs(x, y)
			arc(&s, cx, cy, v[0], v[1], v[2], large, sweep, x, y)
			cx, cy = x, y
		default:
			return s
		}
		lastCmd = upper
	}
}

func isCommand(c byte) bool { return strings.IndexByte("MmLlHhVvCcSsQqTtAaZz", c) >= 0 }

// arc appends the elliptical arc from (x1, y1) to (x2, y2) as cubics,
// following the SVG implementation notes (F.6).
func arc(s *shape, x1, y1, rx, ry, phi float64, large, sweep bool, x2, y2 float64) {
	if x1 == x2 && y1 == y2 {
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 {
		s.line(x2, y2)
		return
	}
	sinPhi, cosPhi := math.Sincos(phi * math.Pi / 180)
	dx, dy := (x1-x2)/2, (y1-y2)/2
	x1p := cosPhi*dx + sinPhi*dy
	y1p := -sinPhi*dx + cosPhi*dy
	if lambda := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry); lambda > 1 {
		k := math.Sqrt(lambda)
		rx, ry = rx*k, ry*k
	}
	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	coef := 0.0
	if den != 0 && num > 0 {
		coef = math.Sqrt(num / den)
	}
	if large == sweep {
		coef = -coef
	}
	cxp, cyp := coef*rx*y1p/ry, -coef*ry*x1p/rx
	cx := cosPhi*cxp - sinPhi*cyp + (x1+x2)/2
	cy := sinPhi*cxp + cosPhi*cyp + (y1+y2)/2
	angle := func(ux, uy, vx, vy float64) float64 {
		return math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	}
	theta := angle(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	delta := angle((x1p-cxp)/rx, (y1p-cyp)/ry, (-x1p-cxp)/rx, (-y1p-cyp)/ry)
	if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	} else if sweep && delta < 0 {
		delta += 2 * math.Pi
	}
	n := int(math.Ceil(math.Abs(delta) / (math.Pi / 2)))
	step := delta / float64(n)
	k := 4.0 / 3 * math.Tan(step/4)
	point := func(t float64) (float64, float64, float64, float64) {
		sin, cos := math.Sincos(t)
		x := cx + rx*cos*cosPhi - ry*sin*sinPhi
		y := cy + rx*cos*sinPhi + ry*sin*cosPhi
		// The derivative, for control points.
		dx := -rx*sin*cosPhi - ry*cos*sinPhi
		dy := -rx*sin*sinPhi + ry*cos*cosPhi
		return x, y, dx, dy
	}
	t := theta
	px, py, pdx, pdy := point(t)
	for i := 0; i < n; i++ {
		t2 := t + step
		qx, qy, qdx, qdy := point(t2)
		if i == n-1 {
			qx, qy = x2, y2
		}
		s.cubic(px+k*pdx, py+k*pdy, qx-k*qdx, qy-k*qdy, qx, qy)
		px, py, pdx, pdy, t = qx, qy, qdx, qdy, t2
	}
}

// ellipse appends a closed ellipse.
func ellipse(s *shape, cx, cy, rx, ry float64) {
	const k = 0.5522847498307936 // 4/3·(√2−1)
	s.move(cx+rx, cy)
	s.cubic(cx+rx, cy+k*ry, cx+k*rx, cy+ry, cx, cy+ry)
	s.cubic(cx-k*rx, cy+ry, cx-rx, cy+k*ry, cx-rx, cy)
	s.cubic(cx-rx, cy-k*ry, cx-k*rx, cy-ry, cx, cy-ry)
	s.cubic(cx+k*rx, cy-ry, cx+rx, cy-k*ry, cx+rx, cy)
	s.close()
}

// roundedRect appends a rectangle with corners rx by ry.
func roundedRect(s *shape, x, y, w, h, rx, ry float64) {
	if rx <= 0 || ry <= 0 {
		s.move(x, y)
		s.line(x+w, y)
		s.line(x+w, y+h)
		s.line(x, y+h)
		s.close()
		return
	}
	rx, ry = math.Min(rx, w/2), math.Min(ry, h/2)
	const k = 0.5522847498307936
	s.move(x+rx, y)
	s.line(x+w-rx, y)
	s.cubic(x+w-rx+k*rx, y, x+w, y+ry-k*ry, x+w, y+ry)
	s.line(x+w, y+h-ry)
	s.cubic(x+w, y+h-ry+k*ry, x+w-rx+k*rx, y+h, x+w-rx, y+h)
	s.line(x+rx, y+h)
	s.cubic(x+rx-k*rx, y+h, x, y+h-ry+k*ry, x, y+h-ry)
	s.line(x, y+ry)
	s.cubic(x, y+ry-k*ry, x+rx-k*rx, y, x+rx, y)
	s.close()
}

// scanner reads the numbers and flags of path data.
type scanner struct {
	s string
	i int
}

func (sc *scanner) done() bool { return sc.i >= len(sc.s) }
func (sc *scanner) peek() byte { return sc.s[sc.i] }

func (sc *scanner) skipSpace() {
	for sc.i < len(sc.s) {
		switch sc.s[sc.i] {
		case ' ', '\t', '\n', '\r', ',':
			sc.i++
		default:
			return
		}
	}
}

func (sc *scanner) num() (float64, bool) {
	sc.skipSpace()
	start := sc.i
	if sc.i < len(sc.s) && (sc.s[sc.i] == '+' || sc.s[sc.i] == '-') {
		sc.i++
	}
	dot, digits := false, false
	for sc.i < len(sc.s) {
		c := sc.s[sc.i]
		switch {
		case c >= '0' && c <= '9':
			digits = true
		case c == '.' && !dot:
			dot = true
		case (c == 'e' || c == 'E') && digits:
			// An exponent: e, sign, digits.
			j := sc.i + 1
			if j < len(sc.s) && (sc.s[j] == '+' || sc.s[j] == '-') {
				j++
			}
			if j < len(sc.s) && sc.s[j] >= '0' && sc.s[j] <= '9' {
				sc.i = j
				for sc.i < len(sc.s) && sc.s[sc.i] >= '0' && sc.s[sc.i] <= '9' {
					sc.i++
				}
			}
			v, err := parseFinite(sc.s[start:sc.i])
			return v, err == nil
		default:
			goto end
		}
		sc.i++
	}
end:
	if !digits {
		sc.i = start
		return 0, false
	}
	v, err := parseFinite(sc.s[start:sc.i])
	return v, err == nil
}

func (sc *scanner) pair() (float64, float64, bool) {
	x, ok := sc.num()
	if !ok {
		return 0, 0, false
	}
	y, ok := sc.num()
	return x, y, ok
}

func (sc *scanner) nums(dst []float64) bool {
	for i := range dst {
		v, ok := sc.num()
		if !ok {
			return false
		}
		dst[i] = v
	}
	return true
}

// flag reads an arc flag, a single 0 or 1 that needs no separator.
func (sc *scanner) flag() (bool, bool) {
	sc.skipSpace()
	if sc.i >= len(sc.s) {
		return false, false
	}
	switch sc.s[sc.i] {
	case '0':
		sc.i++
		return false, true
	case '1':
		sc.i++
		return true, true
	}
	return false, false
}
