package svgdraw

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/packages/mygo/ondine"
)

// hostileValues are CSS and SVG values a diagram's author controls
// (Mermaid's style and classDef lines reach the SVG as written).
var hostileValues = []string{
	"rgb", "rgba", "rgb)", "rgb(", "rgb()", "rgb(1,2", "rgba(1,2,3,", "rgb(((", "rgb)(1,2,3",
	"color-mix", "color-mix(", "color-mix)", "color-mix(in srgb", "color-mix(in srgb, red", "color-mix(in srgb, rgb, rgb)",
	"color-mix(in srgb, color-mix(, red)", "var(", "var(--x", "var()", "var(--_s", "var(--_s,", "var(,)", ")", "(", "((((", "))))",
	"#", "#g", "#ggggggg", "#12345", "url(", "url(#x", "currentColor(", "calc(", "-", "1e999", "NaN", "inf", "\x00", "é(", "😀",
	"translate(", "translate(1", "rotate()", "scale(,)", "matrix(1,2)", "0px", "px ", "bold px ", "16px", "M", "M1", "A1 1 0 0 1",
}

// drawNoRecover draws d in a Tester without Draw's recover, so a panic in
// the parsers fails the test instead of being hidden.
func drawNoRecover(t *testing.T, d *Doc, s *Style) (panicked any) {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(200, 100).Draw(func(p *ui.Painter, r ui.Rect) {
			defer func() {
				if rec := recover(); rec != nil {
					panicked = rec
				}
			}()
			d.draw(p, r, s)
		})
	}, 220, 120)
	tt.Frame()
	return panicked
}

func hostileDocs(v string) []string {
	q := strings.NewReplacer(`"`, "&quot;", "<", "&lt;", "&", "&amp;").Replace(v)
	wrap := func(body string) string {
		return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50" width="100" height="50">` + body + `</svg>`
	}
	return []string{
		wrap(`<rect width="10" height="10" fill="` + q + `" stroke="` + q + `"/>`),
		wrap(`<rect width="10" height="10" style="fill:` + q + `;stroke:` + q + `;stroke-dasharray:` + q + `"/>`),
		wrap(`<g style="--_s:` + q + `"><rect width="10" height="10" style="fill:var(--_s);stroke:var(--_s, ` + q + `)"/></g>`),
		wrap(`<g transform="` + q + `"><path d="` + q + `" stroke="red"/><polygon points="` + q + `"/></g>`),
		wrap(`<text x="1" y="10" style="font:` + q + `;font-size:` + q + `;font-weight:` + q + `" text-anchor="` + q + `" fill="` + q + `">Hi</text>`),
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="` + q + `" width="` + q + `" height="` + q + `"><rect width="1" height="1"/></svg>`,
	}
}

// No author-controlled value panics the parsers.
func TestHostileValuesDrawWithoutPanics(t *testing.T) {
	style := &Style{Vars: themeVars, Font: "monospace", Color: ui.RGB(0, 0, 0)}
	for _, v := range hostileValues {
		for i, svg := range hostileDocs(v) {
			d, err := Parse(svg)
			if err != nil {
				continue
			}
			if p := drawNoRecover(t, d, style); p != nil {
				t.Errorf("value %q, document %d: panic %v", v, i, p)
			}
		}
		if p := func() (p any) {
			defer func() { p = recover() }()
			parseColor(v, ui.Color{})
			colorMix(v, ui.Color{})
			parseTransform(v)
			return nil
		}(); p != nil {
			t.Errorf("value %q: parser panic %v", v, p)
		}
	}
}

// Mermaid styles reach Ondine's SVG; none panics drawing it.
func TestHostileMermaidStylesDraw(t *testing.T) {
	style := &Style{Vars: themeVars, Font: "monospace", Color: ui.RGB(0, 0, 0)}
	for _, v := range hostileValues {
		if strings.ContainsAny(v, "\n\x00;") {
			continue
		}
		for _, src := range []string{
			fmt.Sprintf("flowchart LR\n  A --> B\n  style A fill:%s,stroke:%s,color:%s", v, v, v),
			fmt.Sprintf("flowchart LR\n  A:::x --> B\n  classDef x fill:%s,stroke-dasharray:%s", v, v),
		} {
			out, err := ondine.Render(src, ondine.Options{MaxWidth: 720, Padding: 16})
			if err != nil {
				continue
			}
			d, err := Parse(out.SVG)
			if err != nil {
				t.Errorf("%q: ondine's SVG does not parse: %v", v, err)
				continue
			}
			if p := drawNoRecover(t, d, style); p != nil {
				t.Errorf("%q: panic %v", v, p)
			}
		}
	}
}

// A panic while drawing becomes Err, and the document draws nothing more.
func TestDrawRecovers(t *testing.T) {
	d := &Doc{vw: 10, vh: 10, width: 10, height: 10} // no root: compiling panics
	var first, second error
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(20, 20).Draw(func(p *ui.Painter, r ui.Rect) {
			first = d.Draw(p, r, nil)
			second = d.Draw(p, r, nil)
		})
	}, 40, 40)
	tt.Frame()
	if first == nil || second != first || d.Err() != first {
		t.Fatalf("draw errors %v, %v; Err %v", first, second, d.Err())
	}
}

func FuzzDraw(f *testing.F) {
	for _, v := range hostileValues {
		for _, svg := range hostileDocs(v) {
			f.Add(svg)
		}
	}
	style := &Style{Vars: themeVars, Font: "monospace"}
	f.Fuzz(func(t *testing.T, svg string) {
		d, err := Parse(svg)
		if err != nil {
			return
		}
		if p := drawNoRecover(t, d, style); p != nil {
			t.Fatalf("panic %v", p)
		}
	})
}
