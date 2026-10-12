package files

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNameFilterMatchesBaseNamesCaseInsensitively(t *testing.T) {
	filter, err := NewNameFilter([]string{"*.MD", "dockerfile", "config.?ml", "[ab]*.go"})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{
		"README.md":         true,
		"docs/guide.Md":     true,
		"Dockerfile":        true,
		"deploy/DOCKERFILE": true,
		"config.yml":        true,
		"config.yaml":       false,
		"api.go":            true,
		"main.go":           false,
		"docs/readme.txt":   false,
		"md":                false,
	} {
		if got := filter.Match(name); got != want {
			t.Errorf("Match(%q) = %t, want %t", name, got, want)
		}
	}
	if got := filter.Patterns(); !reflect.DeepEqual(got, []string{"*.MD", "dockerfile", "config.?ml", "[ab]*.go"}) {
		t.Fatalf("patterns = %v", got)
	}
	all := AllNames()
	for _, name := range []string{".env", "Makefile", "image.PNG", "no-extension"} {
		if !all.Match(name) {
			t.Errorf("default filter rejected %q", name)
		}
	}
	if !reflect.DeepEqual(all.Patterns(), []string{"*"}) {
		t.Fatalf("default patterns = %v", all.Patterns())
	}
}

func TestNameFilterRejectsInvalidPatterns(t *testing.T) {
	for _, patterns := range [][]string{nil, {""}, {"*.md", ""}, {"[a-"}, {"\\"}, {"docs/*.md"}} {
		if _, err := NewNameFilter(patterns); err == nil {
			t.Errorf("NewNameFilter(%q) accepted", patterns)
		}
	}
}

func TestScanUsesNameFilterForEveryFileFormat(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"README.md", "main.go", "Dockerfile", "data.bin", "nested/app.GO"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	filter, err := NewNameFilter([]string{"*.go", "Dockerfile"})
	if err != nil {
		t.Fatal(err)
	}
	index, err := Scan(root, -1, nil, filter.Match)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"nested/app.GO", "Dockerfile", "main.go"}; !reflect.DeepEqual(index.Files, want) {
		t.Fatalf("files = %v, want %v", index.Files, want)
	}
	all, err := Scan(root, -1, nil, AllNames().Match)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Files) != 5 {
		t.Fatalf("default scope files = %v", all.Files)
	}
}
