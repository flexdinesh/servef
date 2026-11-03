package theme

import (
	"os"
	"path/filepath"
	"regexp"
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
