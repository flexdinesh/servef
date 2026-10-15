package files

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeContent(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanMarksGitignoredFilesButKeepsThemBrowsable(t *testing.T) {
	root := t.TempDir()
	writeContent(t, filepath.Join(root, ".gitignore"), "# build output\ndist/\n*.gen.md\n!keep.gen.md\n")
	writeContent(t, filepath.Join(root, ".git", "info", "exclude"), "local.md\n")
	writeContent(t, filepath.Join(root, "docs", ".gitignore"), "draft.md\n")
	for _, name := range []string{
		"README.md", "local.md", "api.gen.md", "keep.gen.md",
		"dist/out.md", "dist/nested/deep.md",
		"docs/guide.md", "docs/draft.md", "draft.md",
	} {
		writeFile(t, filepath.Join(root, name))
	}

	index, err := Scan(root, -1, []string{".git"}, markdownFile)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(index.Files) != 9 {
		t.Fatalf("Files = %v, want all nine files browsable", index.Files)
	}
	var ignored []string
	for _, name := range index.Files {
		if index.Ignored(name) {
			ignored = append(ignored, name)
		}
	}
	want := []string{"dist/nested/deep.md", "dist/out.md", "docs/draft.md", "api.gen.md", "local.md"}
	if !reflect.DeepEqual(ignored, want) {
		t.Fatalf("ignored = %v, want %v", ignored, want)
	}
}

func TestScanSkipsExcludeFileOutsideRepository(t *testing.T) {
	root := t.TempDir()
	// A .git file marks a linked worktree; its exclude file lives elsewhere.
	writeContent(t, filepath.Join(root, ".git"), "gitdir: /elsewhere\n")
	writeFile(t, filepath.Join(root, "notes.md"))

	index, err := Scan(root, -1, nil, markdownFile)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if index.Ignored("notes.md") {
		t.Fatal("notes.md ignored without any gitignore rule")
	}
}
