package daemon

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRootsFileRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config", "servef")
	path := filepath.Join(dir, "roots.json")
	roots, err := LoadRoots(path)
	if err != nil || len(roots) != 0 {
		t.Fatalf("LoadRoots(missing) = %v, %v; want empty", roots, err)
	}
	want := []string{"/work", "/personal"}
	if err := SaveRoots(path, want); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadRoots(path); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadRoots() = %v, %v; want %v", got, err, want)
	}
	for name, wantMode := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != wantMode {
			t.Errorf("%s mode = %o, want %o", name, got, wantMode)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("directory holds %d entries, want only roots.json", len(entries))
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "{\"version\":1,\"roots\":[\"/work\",\"/personal\"]}\n" {
		t.Fatalf("roots file = %q, %v", content, err)
	}
	if err := SaveRoots(path, nil); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(path); !strings.Contains(string(content), `"roots":[]`) {
		t.Fatalf("empty roots file = %q", content)
	}
}

func TestLoadRootsRejectsMalformedAndUnknownVersions(t *testing.T) {
	for name, content := range map[string]string{
		"malformed": "{",
		"version":   `{"version":2,"roots":[]}`,
		"missing":   `{"roots":["/work"]}`,
	} {
		path := filepath.Join(t.TempDir(), "roots.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadRoots(path); err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("%s: LoadRoots() = %v, want error naming %s", name, err, path)
		}
	}
}
