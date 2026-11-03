package tree

import (
	"slices"
	"testing"

	"github.com/flexdinesh/servef/pkg/contract"
)

func sample() []contract.TreeNode {
	return []contract.TreeNode{
		{Name: "docs", Path: "docs", IsDir: true, Open: true, Children: []contract.TreeNode{
			{Name: "guide", Path: "docs/guide", IsDir: true, Children: []contract.TreeNode{
				{Name: "a.md", Path: "docs/guide/a.md", FileType: "markdown"},
			}},
			{Name: "empty", Path: "docs/empty", IsDir: true},
		}},
		{Name: "main.go", Path: "main.go", FileType: "go"},
	}
}

func TestIndex(t *testing.T) {
	x := New(sample())
	if !slices.Equal(x.Roots(), []string{"docs", "main.go"}) {
		t.Fatalf("roots %v", x.Roots())
	}
	if !slices.Equal(x.Children("docs"), []string{"docs/guide", "docs/empty"}) {
		t.Fatalf("docs children %v", x.Children("docs"))
	}
	if children := x.Children("docs/empty"); children == nil || len(children) != 0 {
		t.Fatalf("an empty directory has no children but is a directory: %#v", children)
	}
	if x.Children("main.go") != nil || x.Children("missing") != nil {
		t.Fatal("files and unknown paths have nil children")
	}
	entry, ok := x.Entry("docs/guide/a.md")
	if !ok || entry.Name != "a.md" || entry.FileType != "markdown" || entry.IsDir {
		t.Fatalf("entry %+v %v", entry, ok)
	}
	if x.Len() != 5 {
		t.Fatalf("len %d", x.Len())
	}
	if got := x.Ancestors("docs/guide/a.md"); !slices.Equal(got, []string{"docs", "docs/guide"}) {
		t.Fatalf("ancestors %v", got)
	}
	if got := x.Ancestors("main.go"); got != nil {
		t.Fatalf("top-level ancestors %v", got)
	}
}

func TestOpen(t *testing.T) {
	if got := Open(sample()); !slices.Equal(got, []string{"docs"}) {
		t.Fatalf("open %v", got)
	}
}
