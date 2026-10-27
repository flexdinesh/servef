package files

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDirectoriesListsWhatScanReadsIncludingEmptyDirectories(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "README.md"))
	writeFile(t, filepath.Join(root, "docs", "guide.md"))
	writeFile(t, filepath.Join(root, "docs", "deep", "too-deep", "x.md"))
	writeFile(t, filepath.Join(root, "node_modules", "pkg", "index.md"))
	writeFile(t, filepath.Join(root, ".git", "HEAD"))
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	dirs, entries, err := Directories(root, 2, []string{"node_modules"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{root, filepath.Join(root, "docs"), filepath.Join(root, "docs", "deep"), filepath.Join(root, "empty")}
	if !reflect.DeepEqual(dirs, want) {
		t.Fatalf("Directories() = %v, want %v", dirs, want)
	}
	// README.md, docs, empty, linked, node_modules; guide.md, deep; too-deep.
	if entries != 8 {
		t.Fatalf("entries = %d, want 8", entries)
	}
}

func TestDirectoriesRejectsMissingRoot(t *testing.T) {
	if _, _, err := Directories(filepath.Join(t.TempDir(), "missing"), -1, nil); err == nil {
		t.Fatal("Directories() error = nil for a missing root")
	}
}
