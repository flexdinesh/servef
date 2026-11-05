package bridge

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsAppPage(t *testing.T) {
	for raw, want := range map[string]bool{
		"app://local":                      true,
		"app://local/":                     true,
		"app://local/view?project=/a#L1":   true,
		"app://local/api/raw?path=a.svg":   false,
		"app://local/assets/x.js":          false,
		"app://other/":                     false,
		"app://user@local/":                false,
		"https://local/":                   false,
		"mygo://localhost/":                false,
		"file:///local/":                   false,
		"about:blank":                      false,
		"":                                 false,
		"app://local:80/":                  false,
		"app://local.evil.example/":        false,
		"javascript:alert(1)//app://local": false,
	} {
		if got := IsAppPage(raw, "app", "local"); got != want {
			t.Errorf("IsAppPage(%q) = %v", raw, got)
		}
	}
}

func TestParsePoint(t *testing.T) {
	if point, ok := ParsePoint(10.4, 20.6, 100, 100); !ok || point != (Point{10, 21}) {
		t.Errorf("point = %v, %v", point, ok)
	}
	for _, bad := range [][2]any{
		{-1.0, 1.0}, {1.0, -1.0}, {101.0, 1.0}, {1.0, 101.0}, {"1", 1.0}, {nil, 1.0}, {math.NaN(), 1.0}, {math.Inf(1), 1.0},
	} {
		if _, ok := ParsePoint(bad[0], bad[1], 100, 100); ok {
			t.Errorf("ParsePoint(%v) accepted", bad)
		}
	}
}

func item(id, label string) map[string]any { return map[string]any{"id": id, "label": label} }

func TestParseItems(t *testing.T) {
	separator := map[string]any{"separator": true}
	items, ok := ParseItems([]any{separator, item("a", "A"), separator, separator, map[string]any{"id": "b", "label": "B", "enabled": false, "accelerator": "CmdOrCtrl+W"}, separator})
	if !ok || len(items) != 3 || !items[1].Separator || items[0].ID != "a" || !items[0].Enabled || items[2].Enabled || items[2].Accelerator != "CmdOrCtrl+W" {
		t.Errorf("items = %+v, %v", items, ok)
	}
	many := make([]any, maxMenuItems+1)
	for i := range many {
		many[i] = separator
	}
	for name, bad := range map[string]any{
		"not a list":         "a",
		"empty":              []any{},
		"too many":           many,
		"only separators":    []any{separator},
		"not a record":       []any{"a"},
		"separator extras":   []any{map[string]any{"separator": true, "id": "a"}},
		"missing label":      []any{map[string]any{"id": "a"}},
		"blank label":        []any{item("a", "  ")},
		"control in label":   []any{item("a", "a\nb")},
		"long label":         []any{item("a", strings.Repeat("x", maxItemLabel+1))},
		"bad id":             []any{item("a b", "A")},
		"long id":            []any{item(strings.Repeat("a", maxItemID+1), "A")},
		"duplicate id":       []any{item("a", "A"), item("a", "B")},
		"enabled not a bool": []any{map[string]any{"id": "a", "label": "A", "enabled": "yes"}},
		"bad accelerator":    []any{map[string]any{"id": "a", "label": "A", "accelerator": "Hyper+W"}},
		"a bad item spoils":  []any{item("a", "A"), item("b c", "B")},
	} {
		if _, ok := ParseItems(bad); ok {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestParseReveal(t *testing.T) {
	if project, relative, ok := ParseReveal("/a/b", "c/d.md"); !ok || project != "/a/b" || relative != "c/d.md" {
		t.Errorf("ParseReveal = %q %q %v", project, relative, ok)
	}
	if _, _, ok := ParseReveal("/a/b", "c/d/"); !ok {
		t.Error("a directory's trailing slash refused")
	}
	for _, bad := range [][2]any{
		{"a/b", "c"}, {"/a/../b", "c"}, {"/a/b/", "c"}, {"/a\x00", "c"}, {1.0, "c"}, {"/a", 1.0}, {nil, nil},
		{"/a", ""}, {"/a", "/c"}, {"/a", "../c"}, {"/a", "c/../d"}, {"/a", "./c"}, {"/a", "c//d"}, {"/a", `c\d`}, {"/a", "c\x00"},
		{"/a", strings.Repeat("x", maxPathLength+1)}, {"/" + strings.Repeat("x", maxPathLength), "c"},
	} {
		if _, _, ok := ParseReveal(bad[0], bad[1]); ok {
			t.Errorf("ParseReveal(%q) accepted", bad)
		}
	}
}

func TestResolveReveal(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "root")
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filepath.Join(project, "file.md"), filepath.Join(outside, "secret")} {
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(project, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	roots := []string{filepath.Join(base, "missing"), root}

	target, err := ResolveReveal(roots, project, "file.md", filepath.EvalSymlinks)
	if err != nil || target != filepath.Join(project, "file.md") {
		t.Errorf("served file: %q, %v", target, err)
	}
	for name, test := range map[string]struct {
		roots             []string
		project, relative string
		want              error
	}{
		"symlink out of the project":      {roots, project, "escape", ErrPathEscapes},
		"project symlinked out of a root": {roots, filepath.Join(root, "linked"), "secret", ErrNotServed},
		"project outside the roots":       {roots, outside, "secret", ErrNotServed},
		"no roots":                        {nil, project, "file.md", ErrNotServed},
		"root prefix is not a parent":     {[]string{root + "x"}, project, "file.md", ErrNotServed},
		"missing project":                 {roots, filepath.Join(root, "gone"), "a", ErrProjectUnavailable},
		"missing file":                    {roots, project, "gone.md", ErrPathMissing},
		"dot dot":                         {roots, project, "../project/file.md", ErrPathEscapes},
	} {
		if _, err := ResolveReveal(test.roots, test.project, test.relative, filepath.EvalSymlinks); !errors.Is(err, test.want) {
			t.Errorf("%s: %v, want %v", name, err, test.want)
		}
	}
}
