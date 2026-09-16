package files

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestScanSharedMarkdownFixtures(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "markdown")
	index, err := Scan(root, -1, []string{"vendor"})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	wantFiles := []string{
		"guides/code-blocks.md",
		"guides/diagrams.md",
		"guides/getting-started.md",
		"reference/topics/search.md",
		"reference/api.markdown",
		"README.md",
	}
	if !reflect.DeepEqual(index.Files, wantFiles) {
		t.Fatalf("Files = %v, want %v", index.Files, wantFiles)
	}
	if got, want := childNames(index.Tree), []string{"guides", "reference", "README.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root children = %v, want %v", got, want)
	}
	if got, want := childNames(index.Tree.Children[1]), []string{"topics", "api.markdown"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reference children = %v, want %v", got, want)
	}
	if got, want := childNames(index.Tree.Children[1].Children[0]), []string{"search.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("topics children = %v, want %v", got, want)
	}
	if len(index.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", index.Warnings)
	}
}

func TestScanBuildsOrderedDepthLimitedTree(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "b.MARKDOWN"))
	writeFile(t, filepath.Join(root, "A.md"))
	writeFile(t, filepath.Join(root, "ignored.txt"))
	writeFile(t, filepath.Join(root, ".notes", "hidden.md"))
	writeFile(t, filepath.Join(root, "alpha", "one.md"))
	writeFile(t, filepath.Join(root, "alpha", "nested", "too-deep.md"))
	writeFile(t, filepath.Join(root, "Zoo", "z.md"))
	writeFile(t, filepath.Join(root, "node_modules", "excluded.md"))

	index, err := Scan(root, 1, []string{"some/path/node_modules"})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if got, want := childNames(index.Tree), []string{".notes", "alpha", "Zoo", "A.md", "b.MARKDOWN"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root children = %v, want %v", got, want)
	}
	if got, want := childNames(index.Tree.Children[1]), []string{"one.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("alpha children = %v, want %v", got, want)
	}
	if got, want := index.Files, []string{".notes/hidden.md", "alpha/one.md", "Zoo/z.md", "A.md", "b.MARKDOWN"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Files = %v, want %v", got, want)
	}
	if len(index.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", index.Warnings)
	}
}

func TestScanDepthZeroIncludesOnlyRootMarkdown(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "root.md"))
	writeFile(t, filepath.Join(root, "child", "child.md"))

	index, err := Scan(root, 0, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got, want := childNames(index.Tree), []string{"root.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root children = %v, want %v", got, want)
	}
	if got, want := index.Files, []string{"root.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Files = %v, want %v", got, want)
	}
}

func TestScanPrunesDirectoriesWithoutIndexedMarkdown(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "only-text", "notes.txt"))
	writeFile(t, filepath.Join(root, "kept", "nested", "readme.md"))

	index, err := Scan(root, -1, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got, want := childNames(index.Tree), []string{"kept"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root children = %v, want %v", got, want)
	}
	if got, want := index.Files, []string{"kept/nested/readme.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Files = %v, want %v", got, want)
	}
}

func TestScanNegativeDepthIsUnlimitedAndExcludesNamesAnywhere(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a", "b", "deep.markdown"))
	writeFile(t, filepath.Join(root, "a", "vendor", "skip.md"))

	index, err := Scan(root, -1, []string{"vendor"})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got, want := index.Files, []string{"a/b/deep.markdown"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Files = %v, want %v", got, want)
	}
}

func TestScanSkipsFileAndDirectorySymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated privileges on Windows")
	}

	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(root, "real.md"))
	writeFile(t, filepath.Join(outside, "outside.md"))
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.md"), filepath.Join(root, "linked.md")); err != nil {
		t.Fatal(err)
	}

	index, err := Scan(root, -1, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got, want := index.Files, []string{"real.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Files = %v, want %v", got, want)
	}
}

func TestResolveAllowsIndexedFilesAndRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "readme.md"))

	index, err := Scan(root, -1, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	got, err := index.Resolve("docs/readme.md")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(root, "docs", "readme.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}

	if _, err := index.Resolve("docs/missing.md"); !errors.Is(err, ErrNotIndexed) {
		t.Fatalf("Resolve(unindexed) error = %v, want ErrNotIndexed", err)
	}
	if _, err := index.Resolve("../outside.md"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("Resolve(traversal) error = %v, want ErrPathEscape", err)
	}
	if _, err := index.Resolve(filepath.Join(root, "docs", "readme.md")); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("Resolve(absolute) error = %v, want ErrPathEscape", err)
	}
}

func TestResolveDetectsFileReplacedByEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated privileges on Windows")
	}

	root := t.TempDir()
	outside := t.TempDir()
	indexed := filepath.Join(root, "indexed.md")
	writeFile(t, indexed)
	writeFile(t, filepath.Join(outside, "outside.md"))

	index, err := Scan(root, 0, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if err := os.Remove(indexed); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.md"), indexed); err != nil {
		t.Fatal(err)
	}

	if _, err := index.Resolve("indexed.md"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("Resolve() error = %v, want ErrPathEscape", err)
	}
}

func TestScanRejectsInvalidRoots(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "missing"), 1, nil); err == nil {
		t.Fatal("Scan(missing) error = nil, want error")
	}

	root := t.TempDir()
	file := filepath.Join(root, "file.md")
	writeFile(t, file)
	if _, err := Scan(file, 1, nil); err == nil {
		t.Fatal("Scan(file) error = nil, want error")
	}
}

func childNames(entry Entry) []string {
	names := make([]string, len(entry.Children))
	for n := range entry.Children {
		names[n] = entry.Children[n].Name
	}
	return names
}

func writeFile(t *testing.T, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("# Markdown\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
