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
