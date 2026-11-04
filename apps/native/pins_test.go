package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/flexdinesh/servef/apps/native/internal/view"
	"github.com/flexdinesh/servef/packages/mygo/highlights"
	"github.com/flexdinesh/servef/packages/mygo/ondine"
)

// The embedded Highlights and Ondine modules are the versions the web app
// locks (decision 0088): pin.go's version and integrity equal
// pnpm-lock.yaml's package entries. A bump needs mise run native:wasm-sync.
func TestWasmPinsMatchTheLockfile(t *testing.T) {
	lock, err := os.ReadFile(filepath.Join("..", "..", "pnpm-lock.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pin := range []struct{ name, version, integrity string }{
		{"@pierre/highlights", highlights.Version, highlights.Integrity},
		{"ondine", ondine.Version, ondine.Integrity},
	} {
		// "  '@pierre/highlights@0.2.0':" or "  ondine@0.0.3:", then its
		// resolution line, in the packages section.
		entry := regexp.MustCompile(`(?m)^  '?` + regexp.QuoteMeta(pin.name) + `@([^':]+)'?:\n    resolution: \{integrity: ([^}]+)\}`)
		matches := entry.FindAllSubmatch(lock, -1)
		if len(matches) != 1 {
			t.Errorf("pnpm-lock.yaml has %d resolved entries for %s, want 1", len(matches), pin.name)
			continue
		}
		if got := string(matches[0][1]); got != pin.version {
			t.Errorf("%s: lockfile %s, pin.go %s (run mise run native:wasm-sync)", pin.name, got, pin.version)
		}
		if got := string(matches[0][2]); got != pin.integrity {
			t.Errorf("%s: lockfile integrity %s, pin.go %s (run mise run native:wasm-sync)", pin.name, got, pin.integrity)
		}
	}
}

// The shell's Lucide icons are the release the web app locks; a bump
// needs scripts/sync-lucide-icons.mjs.
func TestLucideIconsMatchTheLockfile(t *testing.T) {
	lock, err := os.ReadFile(filepath.Join("..", "..", "pnpm-lock.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`(?m)^  lucide-react@([^:(]+):\n    resolution:`).FindAllSubmatch(lock, -1)
	if len(matches) != 1 || string(matches[0][1]) != view.LucideVersion {
		t.Fatalf("pnpm-lock.yaml lucide-react %q, icons %s (run node apps/native/scripts/sync-lucide-icons.mjs apps/web)", matches, view.LucideVersion)
	}
}
