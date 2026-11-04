package theme

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The palettes equal tokens.css's light-dark() pairs.
func TestPalettesMatchTokens(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "styles", "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	pair := regexp.MustCompile(`--color-([a-z-]+): light-dark\((#[0-9a-f]{6}), (#[0-9a-f]{6})\);`)
	found := map[string][2]string{}
	for _, match := range pair.FindAllStringSubmatch(string(css), -1) {
		found[match[1]] = [2]string{match[2], match[3]}
	}
	for side, palette := range []Palette{Light, Dark} {
		for token, value := range palette.Tokens() {
			values, ok := found[token]
			if !ok {
				t.Errorf("--color-%s is not a light-dark() pair in tokens.css", token)
				continue
			}
			if values[side] != value {
				t.Errorf("--color-%s: native %s, tokens.css %s", token, value, values[side])
			}
		}
	}
}

// Servef's surface-recessed lives in themes.css: light-dark over the
// secondary surface.
func TestSurfaceRecessedMatchesThemes(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "styles", "themes.css"))
	if err != nil {
		t.Fatal(err)
	}
	want := "--color-surface-recessed: light-dark(" + Light.SurfaceRecessed + ", var(--color-surface-secondary));"
	if !strings.Contains(string(css), want) || Dark.SurfaceRecessed != Dark.SurfaceSecondary {
		t.Errorf("themes.css lacks %q, or dark recessed %s is not secondary %s", want, Dark.SurfaceRecessed, Dark.SurfaceSecondary)
	}
}

// Ondine's variables are mermaid.css's, through the same tokens.
func TestOndineMatchesMermaidCSS(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "features", "documents", "mermaid.css"))
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "styles", "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`--radius-sm: ` + regexp.QuoteMeta(RadiusSmall) + `;`).Match(tokens) {
		t.Errorf("--radius-sm is not %s in tokens.css", RadiusSmall)
	}
	decl := regexp.MustCompile(`(--ondine-[a-z-]+): var\(--([a-z-]+)\);`)
	matches := decl.FindAllStringSubmatch(string(css), -1)
	for _, palette := range []Palette{Light, Dark} {
		vars, values := palette.Ondine(), palette.Tokens()
		if len(matches) != len(vars) {
			t.Fatalf("mermaid.css maps %d variables, native %d", len(matches), len(vars))
		}
		for _, m := range matches {
			want, ok := values[strings.TrimPrefix(m[2], "color-")]
			switch m[2] {
			case "font-mono":
				want, ok = "monospace", true
			case "radius-sm":
				want, ok = RadiusSmall, true
			}
			if !ok || vars[m[1]] != want {
				t.Errorf("%s: native %q, mermaid.css var(--%s) = %q", m[1], vars[m[1]], m[2], want)
			}
		}
	}
}

// The chrome is themes.css's Servef value: light's own grey, dark's mix.
func TestChromeMatchesThemes(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "styles", "themes.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "--color-workspace-chrome: light-dark("+ChromeLight+", color-mix(in srgb, var(--color-surface), var(--color-canvas) 50%));") {
		t.Fatal("themes.css's Servef chrome changed")
	}
	if !strings.Contains(string(css), "--color-chrome-hover: light-dark(color-mix(in srgb, var(--color-surface), var(--color-workspace-chrome)), var(--color-hover));") {
		t.Fatal("themes.css's Servef chrome hover changed")
	}
	for palette, want := range map[*Palette][2]string{&Light: {"#e5e7eb", "#f2f3f5"}, &Dark: {"#151515", "#262627"}} {
		if got := [2]string{palette.Chrome(), palette.ChromeHover()}; got != want {
			t.Errorf("%s: %v, want %v", palette.Bg, got, want)
		}
	}
	if Mix("#000000", "#ffffff", 0.5) != "#808080" {
		t.Error(Mix("#000000", "#ffffff", 0.5))
	}
}
