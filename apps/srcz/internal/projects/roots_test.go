package projects

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

// rootsFixture makes sibling directories a and b (a has a child "inner"), a
// file b/file, and a symlink "link" to a.
func rootsFixture(t *testing.T) (parent, a, b string) {
	t.Helper()
	parent = canonicalTemp(t)
	a, b = filepath.Join(parent, "a"), filepath.Join(parent, "b")
	writeFile(t, filepath.Join(a, "inner", "file"), "")
	writeFile(t, filepath.Join(b, "file"), "")
	if err := os.Symlink(a, filepath.Join(parent, "link")); err != nil {
		t.Fatal(err)
	}
	return parent, a, b
}

// The web app's guards reject null: an empty set must encode as a list.
func TestRootSetListWithNoRootsEncodesAsAList(t *testing.T) {
	var set RootSet
	encoded, err := json.Marshal(set.List())
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("List() encodes as %s; want []", encoded)
	}
}

func TestNewRootSetKeepsOrderAndDeduplicates(t *testing.T) {
	parent, a, b := rootsFixture(t)
	set, err := NewRootSet([]string{b, filepath.Join(parent, "link"), a, b + "/"})
	if err != nil {
		t.Fatal(err)
	}
	if got := set.List(); !reflect.DeepEqual(got, []string{b, a}) {
		t.Fatalf("List() = %v; want [b a]", got)
	}
}

func TestNewRootSetRejectsInvalidRoots(t *testing.T) {
	parent, a, b := rootsFixture(t)
	for _, paths := range [][]string{
		{a, filepath.Join(a, "inner")},
		{filepath.Join(a, "inner"), a},
		{filepath.Join(parent, "missing")},
		{filepath.Join(b, "file")},
		{},
	} {
		if _, err := NewRootSet(paths); err == nil {
			t.Errorf("NewRootSet(%v) error = nil", paths)
		}
	}
}

func TestRootSetAddAppendsSibling(t *testing.T) {
	_, a, b := rootsFixture(t)
	set, err := NewRootSet([]string{a})
	if err != nil {
		t.Fatal(err)
	}
	root, added, err := set.Add(b + "/")
	if err != nil || !added || root != b {
		t.Fatalf("Add(b) = %q, %v, %v; want %q, true, nil", root, added, err, b)
	}
	if got := set.List(); !reflect.DeepEqual(got, []string{a, b}) {
		t.Fatalf("List() = %v; want [a b]", got)
	}
}

func TestRootSetAddExistingRoot(t *testing.T) {
	parent, a, b := rootsFixture(t)
	set, err := NewRootSet([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{a, filepath.Join(parent, "link")} {
		root, added, err := set.Add(path)
		if err != nil || added || root != a {
			t.Errorf("Add(%q) = %q, %v, %v; want %q, false, nil", path, root, added, err, a)
		}
	}
	if got := set.List(); !reflect.DeepEqual(got, []string{a, b}) {
		t.Fatalf("List() = %v; want [a b]", got)
	}
}

func TestRootSetAddRejectsInvalidRoots(t *testing.T) {
	parent, a, b := rootsFixture(t)
	for name, test := range map[string]struct {
		roots []string
		path  string
	}{
		"child of a root":  {[]string{a}, filepath.Join(a, "inner")},
		"parent of a root": {[]string{filepath.Join(a, "inner")}, a},
		"missing":          {[]string{a}, filepath.Join(parent, "missing")},
		"not a directory":  {[]string{a}, filepath.Join(b, "file")},
	} {
		set, err := NewRootSet(test.roots)
		if err != nil {
			t.Fatal(err)
		}
		if root, added, err := set.Add(test.path); err == nil {
			t.Errorf("%s: Add(%q) = %q, %v, nil; want error", name, test.path, root, added)
		}
		if got := set.List(); !reflect.DeepEqual(got, test.roots) {
			t.Errorf("%s: List() = %v; want unchanged %v", name, got, test.roots)
		}
	}
}

func TestRootSetListReturnsCopy(t *testing.T) {
	_, a, b := rootsFixture(t)
	set, err := NewRootSet([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	list := set.List()
	list[0] = "changed"
	if got := set.List(); !reflect.DeepEqual(got, []string{a, b}) {
		t.Fatalf("List() after mutating a copy = %v; want [a b]", got)
	}
}

func TestRootSetConcurrentAddAndList(t *testing.T) {
	parent := canonicalTemp(t)
	first := filepath.Join(parent, "root-0")
	writeFile(t, filepath.Join(first, "file"), "")
	set, err := NewRootSet([]string{first})
	if err != nil {
		t.Fatal(err)
	}
	const count = 16
	paths := make([]string, count)
	for i := range paths {
		paths[i] = filepath.Join(parent, fmt.Sprintf("root-%d", i+1))
		writeFile(t, filepath.Join(paths[i], "file"), "")
	}

	// Two goroutines add each path while others list; exactly one Add per
	// path reports added.
	var wg sync.WaitGroup
	added := make(chan string, 2*count)
	for _, path := range paths {
		for range 2 {
			wg.Add(2)
			go func() {
				defer wg.Done()
				root, ok, err := set.Add(path)
				if err != nil {
					t.Errorf("Add(%q) error = %v", path, err)
				}
				if ok {
					added <- root
				}
			}()
			go func() {
				defer wg.Done()
				if got := set.List(); len(got) == 0 || got[0] != first {
					t.Errorf("List() = %v; want %q first", got, first)
				}
			}()
		}
	}
	wg.Wait()
	close(added)
	wins := map[string]int{}
	for root := range added {
		wins[root]++
	}
	for _, path := range paths {
		if wins[path] != 1 {
			t.Errorf("Add(%q) reported added %d times; want 1", path, wins[path])
		}
	}

	got := set.List()
	if len(got) != count+1 || got[0] != first {
		t.Fatalf("List() = %v; want %d roots starting with %q", got, count+1, first)
	}
	seen := map[string]bool{}
	for _, root := range got {
		if seen[root] {
			t.Fatalf("List() = %v; %q appears twice", got, root)
		}
		seen[root] = true
	}
}

func TestRootSetZeroValueIsEmptyAndGrows(t *testing.T) {
	_, a, b := rootsFixture(t)
	var set RootSet
	if got := set.List(); len(got) != 0 {
		t.Fatalf("zero RootSet List() = %v, want empty", got)
	}
	for _, path := range []string{a, b, a} {
		if _, _, err := set.Add(path); err != nil {
			t.Fatal(err)
		}
	}
	if got := set.List(); !reflect.DeepEqual(got, []string{a, b}) {
		t.Fatalf("List() = %v, want %v", got, []string{a, b})
	}
}

func TestRootSetCoverWithinOrEqualToRootAddsNothing(t *testing.T) {
	parent, a, b := rootsFixture(t)
	set, err := NewRootSet([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{a, filepath.Join(a, "inner"), filepath.Join(parent, "link", "inner"), b + "/"} {
		want := a
		if path == b+"/" {
			want = b
		}
		root, added, replaced, err := set.Cover(path)
		if err != nil || added || replaced != nil || root != want {
			t.Errorf("Cover(%q) = %q, %v, %v, %v; want %q, false, nil, nil", path, root, added, replaced, err, want)
		}
	}
	if got := set.List(); !reflect.DeepEqual(got, []string{a, b}) {
		t.Fatalf("List() = %v; want unchanged [a b]", got)
	}
}

func TestRootSetCoverAddsOutsidePath(t *testing.T) {
	parent, a, _ := rootsFixture(t)
	other := filepath.Join(parent, "other")
	writeFile(t, filepath.Join(other, "file"), "")
	set, err := NewRootSet([]string{a})
	if err != nil {
		t.Fatal(err)
	}
	root, added, replaced, err := set.Cover(other + "/")
	if err != nil || !added || replaced != nil || root != other {
		t.Fatalf("Cover(other) = %q, %v, %v, %v; want %q, true, nil, nil", root, added, replaced, err, other)
	}
	if got := set.List(); !reflect.DeepEqual(got, []string{a, other}) {
		t.Fatalf("List() = %v; want [a other]", got)
	}
}

func TestRootSetCoverReplacesRootsInside(t *testing.T) {
	parent, a, b := rootsFixture(t)
	sibling := canonicalTemp(t)
	inner := filepath.Join(a, "inner")
	set, err := NewRootSet([]string{inner, sibling, b})
	if err != nil {
		t.Fatal(err)
	}
	root, added, replaced, err := set.Cover(parent)
	if err != nil || !added || root != parent || !reflect.DeepEqual(replaced, []string{inner, b}) {
		t.Fatalf("Cover(parent) = %q, %v, %v, %v; want %q, true, [inner b], nil", root, added, replaced, err, parent)
	}
	if got := set.List(); !reflect.DeepEqual(got, []string{parent, sibling}) {
		t.Fatalf("List() = %v; want [parent sibling]", got)
	}
	if root, added, _, err := set.Cover(a); err != nil || added || root != parent {
		t.Fatalf("Cover(a) after replace = %q, %v, %v; want parent, false", root, added, err)
	}
}

func TestRootSetCoverRejectsInvalidPaths(t *testing.T) {
	parent, a, b := rootsFixture(t)
	set, err := NewRootSet([]string{a})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(parent, "missing"), filepath.Join(b, "file")} {
		if root, added, replaced, err := set.Cover(path); err == nil {
			t.Errorf("Cover(%q) = %q, %v, %v, nil; want error", path, root, added, replaced)
		}
	}
	if got := set.List(); !reflect.DeepEqual(got, []string{a}) {
		t.Fatalf("List() = %v; want unchanged [a]", got)
	}
}
