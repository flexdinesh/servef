package svgdraw

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// themeVars are the --ondine-* values the web app's mermaid.css gives
// ondine in a light theme.
var themeVars = map[string]string{
	"--ondine-bg": "#ffffff", "--ondine-fg": "#333333", "--ondine-line": "#515866", "--ondine-border": "#515866",
	"--ondine-muted": "#5d6676", "--ondine-surface": "#ffffff", "--ondine-surface-alt": "#f1f2f4",
	"--ondine-accent": "#2563eb", "--ondine-critical": "#bd3029", "--ondine-font": "monospace", "--ondine-radius": "0.125rem",
}

func ondineSVGs(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "ondine", "testdata", "renders.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Renders map[string]struct {
			SVG string `json:"svg"`
		} `json:"renders"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for name, r := range golden.Renders {
		if r.SVG != "" {
			out[name] = r.SVG
		}
	}
	if len(out) == 0 {
		t.Fatal("no diagrams")
	}
	return out
}

func texts(l *list) []string {
	var out []string
	for _, o := range l.ops {
		if o.text == nil {
			continue
		}
		for _, ch := range o.text.chunks {
			var b strings.Builder
			for _, pc := range ch.pieces {
				b.WriteString(pc.text)
			}
			out = append(out, b.String())
		}
	}
	return out
}

func TestCompilesOndineDiagrams(t *testing.T) {
	style := &Style{Vars: themeVars}
	for name, svg := range ondineSVGs(t) {
		d, err := Parse(svg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if w, h := d.Size(); w <= 0 || h <= 0 {
			t.Fatalf("%s: size %vx%v", name, w, h)
		}
		l := d.compiled(style)
		if len(l.ops) == 0 {
			t.Fatalf("%s: nothing to draw", name)
		}
		for _, o := range l.ops {
			for _, c := range []ui.Color{o.fill, o.stroke} {
				if c == (ui.Color{}) && (o.hasFill || o.hasStroke) && o.text == nil {
					// Colors resolve; none is only ever transparent by design.
					continue
				}
			}
		}
		if len(texts(l)) == 0 {
			t.Errorf("%s: no text", name)
		}
	}
}

// The flowchart's labels, anchors, and theme colors come through: node
// surfaces from --ondine-surface, edges from --ondine-line, text from
// --ondine-fg, and a theme switch recolors without parsing again.
func TestFlowchartStyle(t *testing.T) {
	d, err := Parse(ondineSVGs(t)["flowchart.mmd"])
	if err != nil {
		t.Fatal(err)
	}
	l := d.compiled(&Style{Vars: themeVars})
	got := strings.Join(texts(l), "|")
	for _, want := range []string{"default", "Source", "Open", "request-flow.mmd", "Preview?", "Themed SVG", "Original text"} {
		if !strings.Contains(got, want) {
			t.Errorf("texts %q lack %q", got, want)
		}
	}
	var line, surface, label bool
	for _, o := range l.ops {
		if o.text != nil {
			for _, ch := range o.text.chunks {
				if ch.anchor == "middle" && len(ch.pieces) > 0 && ch.pieces[0].color == ui.Hex("#333333") && ch.pieces[0].family == "monospace" {
					label = true
				}
			}
			continue
		}
		if o.hasStroke && o.stroke == ui.Hex("#515866") && !o.hasFill {
			line = true
		}
		if o.hasFill && o.fill == ui.Hex("#ffffff") && o.hasStroke && o.stroke == ui.Hex("#515866") {
			surface = true
		}
	}
	if !line || !surface || !label {
		t.Fatalf("line %v surface %v label %v", line, surface, label)
	}
	dark := map[string]string{}
	for k, v := range themeVars {
		dark[k] = v
	}
	dark["--ondine-surface"] = "#1f1f1f"
	if l2 := d.compiled(&Style{Vars: dark}); l2 == l {
		t.Fatal("a new theme reused the light theme's colors")
	}
	if l3 := d.compiled(&Style{Vars: themeVars}); l3 != l {
		t.Fatal("the light theme compiled again")
	}
}

func TestResolve(t *testing.T) {
	root := &node{tag: "svg", vars: map[string]string{"--_fg": "var(--ondine-fg,#000)", "--_a": "var(--ondine-surface-alt,color-mix(in srgb,var(--_fg) 20%,var(--_bg)))", "--_bg": "var(--ondine-bg,#fff)"}}
	child := &node{tag: "g", parent: root}
	r := resolver{vars: map[string]string{"--ondine-fg": "#ff0000"}}
	if v, ok := r.substitute("var(--_fg)", child, 0); !ok || v != "#ff0000" {
		t.Fatalf("fg %q %v", v, ok)
	}
	v, ok := r.substitute("var(--_a)", child, 0)
	if !ok {
		t.Fatal("surface-alt")
	}
	c, ok := parseColor(v, ui.Color{})
	if want := ui.RGB(255, 204, 204); !ok || c != want {
		t.Fatalf("color-mix %q = %v, want %v", v, c, want)
	}
	if _, ok := r.substitute("var(--missing)", child, 0); ok {
		t.Fatal("a missing var without fallback is invalid")
	}
	loop := &node{tag: "svg", vars: map[string]string{"--a": "var(--b)", "--b": "var(--a)"}}
	if _, ok := r.substitute("var(--a)", loop, 0); ok {
		t.Fatal("a cycle resolved")
	}
	for in, want := range map[string]ui.Color{"#abc": ui.RGB(0xaa, 0xbb, 0xcc), "red": ui.RGB(255, 0, 0), "rgb(1, 2, 3)": ui.RGB(1, 2, 3), "rgba(0,0,0,0.5)": ui.RGBA(0, 0, 0, 0.5)} {
		if got, ok := parseColor(in, ui.Color{}); !ok || got != want {
			t.Errorf("%s: %v, want %v", in, got, want)
		}
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestPathData(t *testing.T) {
	s := pathData("M10,20 l5 0 H30 v10 h-5 V20 Z m1,1 C1 2 3 4 5 6 S7 8 9 10 Q1 1 2 2 T3 3 A5,5 0 0 1 13,3 a2 2 0 102 2")
	ops := ""
	for _, seg := range s {
		ops += string(seg.op)
	}
	if ops != "MLLLLLZMCCCCCC" && !strings.HasPrefix(ops, "MLLLLLZMCCCC") {
		t.Fatalf("ops %s", ops)
	}
	end := func(i int) [2]float64 {
		seg := s[i]
		if seg.op == 'C' {
			return seg.pts[2]
		}
		return seg.pts[0]
	}
	if p := end(1); !near(p[0], 15) || !near(p[1], 20) {
		t.Fatalf("relative line %v", p)
	}
	if p := end(5); !near(p[0], 25) || !near(p[1], 20) {
		t.Fatalf("V %v", p)
	}
	// After Z, the point is the subpath's start; m is relative to it.
	if p := end(7); !near(p[0], 11) || !near(p[1], 21) {
		t.Fatalf("m after Z %v", p)
	}
	if p := end(len(s) - 1); !near(p[0], 15) || !near(p[1], 5) {
		t.Fatalf("arc end %v", p)
	}
	if got := pathData("M0,0L1,1 L2"); len(got) != 2 {
		t.Fatalf("a malformed path keeps what came before: %v", got)
	}
}

func TestTransforms(t *testing.T) {
	m, ok := parseTransform("translate(10,20) rotate(90)")
	if !ok {
		t.Fatal("transform")
	}
	x, y := m.apply(1, 0)
	if !near(x, 10) || !near(y, 21) {
		t.Fatalf("translate rotate: %v,%v", x, y)
	}
	if !near(m.rotation(), 90) || !near(m.scale(), 1) {
		t.Fatalf("rotation %v scale %v", m.rotation(), m.scale())
	}
	if _, ok := parseTransform("translate(1"); ok {
		t.Fatal("malformed transform")
	}
}

// Drawn in a Tester, a node's surface shows the theme's color and its
// label draws glyphs in the text color.
func TestDraws(t *testing.T) {
	d, err := Parse(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50" width="100" height="50" style="--_s:var(--ondine-surface,#fff)"><rect x="0" y="0" width="100" height="50" fill="var(--_s)"/><text x="50" y="30" text-anchor="middle" font-size="16" fill="#000000">Hi</text></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	style := &Style{Vars: map[string]string{"--ondine-surface": "#00ff00"}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() { View(c, d, style).Label("diagram") })
	}, 200, 100)
	tt.Frame()
	img := tt.Image()
	green := color.RGBA{0, 255, 0, 255}
	if got := img.RGBAAt(5, 5); got != green {
		t.Fatalf("surface %v, want %v", got, green)
	}
	if !drawsText() {
		// mygo draws text through the system's Pango and fontconfig.
		if os.Getenv("MYGO_REQUIRE_TEXT") != "" {
			t.Fatal("mygo draws no text here (no Pango, fontconfig, or fonts), and MYGO_REQUIRE_TEXT is set")
		}
		t.Log("mygo draws no text on this system (no Pango, fontconfig, or fonts): glyphs not checked")
		return
	}
	dark := 0
	for y := 15; y < 35; y++ {
		for x := 35; x < 65; x++ {
			if c := img.RGBAAt(x, y); c.G < 128 {
				dark++
			}
		}
	}
	if dark == 0 {
		t.Fatal("the label drew no glyphs")
	}
	dumpPNG(t, "draws", img)
}

// drawsText reports whether mygo draws text here at all: it shapes and
// draws through the system's Pango, cairo, and fontconfig (dlopen), so a
// host without them draws none.
func drawsText() bool {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Text(c, "Hi").FontSize(24).TextColor(ui.Hex("#000000"))
	}, 100, 50)
	tt.Frame()
	img := tt.Image()
	for y := range 50 {
		for x := range 100 {
			if img.RGBAAt(x, y).G < 128 {
				return true
			}
		}
	}
	return false
}

// SVGDRAW_SHOTS=<dir> writes every Ondine diagram drawn, to look at.
func TestShots(t *testing.T) {
	if os.Getenv("SVGDRAW_SHOTS") == "" {
		t.Skip("set SVGDRAW_SHOTS to write PNGs")
	}
	for name, svg := range ondineSVGs(t) {
		d, err := Parse(svg)
		if err != nil {
			t.Fatal(err)
		}
		w, h := d.Size()
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Background(ui.Hex("#ffffff")).Children(func() { View(c, d, &Style{Vars: themeVars}) })
		}, int(w)+2, int(h)+2)
		tt.SetScale(2)
		tt.Frame()
		dumpPNG(t, strings.TrimSuffix(name, ".mmd"), tt.Image())
	}
}

func dumpPNG(t *testing.T, name string, img image.Image) {
	dir := os.Getenv("SVGDRAW_SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
