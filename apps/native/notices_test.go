package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Release packages carry the licenses of the embedded wasm modules, of
// wazero, and of Pierre's tree icons (scripts/notices.sh, run by
// scripts/package.sh).
func TestThirdPartyNotices(t *testing.T) {
	out := filepath.Join(t.TempDir(), "THIRD_PARTY_NOTICES.txt")
	cmd := exec.Command("sh", "scripts/notices.sh", out)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("notices.sh: %v\n%s", err, output)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"@pierre/highlights 0.2.0 (highlights.wasm): LICENSE.md", "@pierre/highlights 0.2.0: NOTICE.md",
		"ondine 0.0.3 (all.generated.wasm): LICENSE", "github.com/tetratelabs/wazero v1.12.0: LICENSE",
		"@pierre/trees 1.0.0-beta.6 (file icons): LICENSE.md", "@pierre/trees 1.0.0-beta.6: NOTICE.md",
		"Apache License", "MIT License",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("THIRD_PARTY_NOTICES.txt lacks %q", want)
		}
	}
}
