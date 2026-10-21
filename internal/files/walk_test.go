package files

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func walked(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	warnings, err := WalkUnignored(context.Background(), root, NewIgnoreRules(root), func(name string, _ fs.DirEntry) error {
		names = append(names, name)
		return nil
	})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("WalkUnignored() = %v, %v", warnings, err)
	}
	return names
}

func TestWalkUnignoredPrunesIgnoredGitAndNestedRepositories(t *testing.T) {
	home := isolateGitConfig(t)
	writeContent(t, filepath.Join(home, ".config", "git", "ignore"), "global.md\n")
	root := canonicalTempDir(t)
	writeContent(t, filepath.Join(root, ".git", "info", "exclude"), "local.md\n")
	writeContent(t, filepath.Join(root, ".gitignore"), "node_modules/\n*.log\n")
	writeContent(t, filepath.Join(root, "docs", ".gitignore"), "draft.md\n")
	for _, name := range []string{
		"README.md", "global.md", "local.md", "debug.log", ".hidden/kept.md",
		"node_modules/pkg/index.js", "docs/guide.md", "docs/draft.md",
		"nested/.git/HEAD", "nested/inner.md", "deep/sub/file.md",
	} {
		writeFile(t, filepath.Join(root, name))
	}
	if err := os.Symlink("docs", filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}

	// Depth, names, and exclusions do not apply; symlinks are listed, not
	// followed.
	want := []string{".gitignore", ".hidden/kept.md", "README.md", "alias", "deep/sub/file.md", "docs/.gitignore", "docs/guide.md"}
	if got := walked(t, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("walked = %v, want %v", got, want)
	}
}

func TestWalkUnignoredStopsWhenCancelled(t *testing.T) {
	root := canonicalTempDir(t)
	writeFile(t, filepath.Join(root, "a.md"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := WalkUnignored(ctx, root, NewIgnoreRules(root), func(string, fs.DirEntry) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WalkUnignored() error = %v, want context.Canceled", err)
	}
}

func TestIgnoreRulesAnswerSinglePaths(t *testing.T) {
	isolateGitConfig(t)
	root := canonicalTempDir(t)
	writeContent(t, filepath.Join(root, ".git", "info", "exclude"), "local.md\n")
	writeContent(t, filepath.Join(root, ".gitignore"), "build/\n*.gen.md\n!keep.gen.md\n")
	writeContent(t, filepath.Join(root, "docs", ".gitignore"), "draft.md\n")
	// An ignored directory's patterns never re-include its files.
	writeContent(t, filepath.Join(root, "build", ".gitignore"), "!out.md\n")

	rules := NewIgnoreRules(root)
	for name, want := range map[string]bool{
		"README.md":      false,
		"local.md":       true,
		"api.gen.md":     true,
		"keep.gen.md":    false,
		"build/out.md":   true,
		"build/a/b.md":   true,
		"docs/draft.md":  true,
		"docs/guide.md":  false,
		"draft.md":       false,
		".git/config":    true,
		"src/.git":       true,
		"other/build.md": false,
	} {
		if got := rules.Ignored(name, false); got != want {
			t.Errorf("Ignored(%q) = %v, want %v", name, got, want)
		}
	}
	if !rules.Ignored("build", true) || rules.Ignored("build", false) {
		t.Error("directory-only pattern should match only the directory")
	}
}

func TestResolveFileConfinesToRoot(t *testing.T) {
	parent := canonicalTempDir(t)
	root := filepath.Join(parent, "root")
	writeFile(t, filepath.Join(root, "docs", "a.md"))
	writeFile(t, filepath.Join(parent, "outside.md"))
	if err := os.Symlink("docs/a.md", filepath.Join(root, "inside-link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(parent, "outside.md"), filepath.Join(root, "outside-link.md")); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]string{
		"docs/a.md":      filepath.Join(root, "docs", "a.md"),
		"inside-link.md": filepath.Join(root, "docs", "a.md"),
	} {
		if got, err := ResolveFile(root, name); err != nil || got != want {
			t.Errorf("ResolveFile(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	for _, name := range []string{"../outside.md", "outside-link.md", filepath.Join(parent, "outside.md"), "", ".", "docs/../../outside.md"} {
		if _, err := ResolveFile(root, name); !errors.Is(err, ErrPathEscape) {
			t.Errorf("ResolveFile(%q) error = %v, want ErrPathEscape", name, err)
		}
	}
	for _, name := range []string{"docs", "missing.md"} {
		if _, err := ResolveFile(root, name); err == nil || errors.Is(err, ErrPathEscape) {
			t.Errorf("ResolveFile(%q) error = %v, want a non-escape error", name, err)
		}
	}
}
