package trees

import (
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func sample() []Node {
	return []Node{
		{Name: "README.md", Path: "README.md"},
		{Name: "src", Path: "src", Dir: true, Children: []Node{
			{Name: "main.go", Path: "src/main.go"},
			{Name: "app", Path: "src/app", Dir: true, Children: []Node{
				{Name: "App.tsx", Path: "src/app/App.tsx"},
				{Name: "index.ts", Path: "src/app/index.ts"},
			}},
		}},
		{Name: ".github", Path: ".github", Dir: true, Children: []Node{
			{Name: "workflows", Path: ".github/workflows", Dir: true, Children: []Node{
				{Name: "ci.yml", Path: ".github/workflows/ci.yml"},
			}},
		}},
		{Name: "go.mod", Path: "go.mod"},
		{Name: ".gitignore", Path: ".gitignore"},
	}
}

func TestIconsResolveByNameAsPierre(t *testing.T) {
	for name, token := range map[string]string{
		"main.go": "go", "README.md": "markdown", "App.tsx": "react", "index.ts": "typescript",
		"styles.SCSS": "sass", "package.json": "json", ".gitignore": "git", "Dockerfile": "docker",
		"ci.yml": "yml", "logo.png": "image", "notes.txt": "text", "archive.tar.gz": "zip",
		"sample.bin": "default", "Makefile": "default", "no-extension": "default", "src/deep/main.rs": "rust",
	} {
		if got := IconFor(name).Token; got != token {
			t.Errorf("IconFor(%q) = %q, want %q", name, got, token)
		}
	}
}

func TestEveryTokenHasAnIconAndAHue(t *testing.T) {
	tokens := map[string]bool{"default": true}
	for _, table := range []map[string]string{pierreFileNames, pierreExtensions, pierreCompleteOverrides} {
		for _, token := range table {
			tokens[token] = true
		}
	}
	for token := range tokens {
		if _, ok := pierreSymbols[token]; !ok {
			t.Errorf("token %q has no symbol", token)
		}
		if hue, ok := pierreIconColors[token]; ok {
			if _, known := pierrePalette[hue]; !known {
				t.Errorf("token %q has an unknown hue %q", token, hue)
			}
		}
		if w, h := (Icon{Token: token}).SVG().Size(); w != 16 || h != 16 {
			t.Errorf("token %q is %vx%v", token, w, h)
		}
	}
	light, ok := (Icon{Token: "go"}).Color(false)
	if dark, _ := (Icon{Token: "go"}).Color(true); !ok || light == dark {
		t.Error("go has a light and a dark hue")
	}
	if _, ok := (Icon{Token: "font"}).Color(false); ok {
		t.Error("tokens without a hue take the tree's muted ink, as Pierre's")
	}
}

func TestTreeSortsFlattensAndFilters(t *testing.T) {
	tr := New(sample(), Options{Sort: true})
	if got := names(tr.Roots(), tr); got != ".github,src,.gitignore,go.mod,README.md" {
		t.Fatalf("sorted roots %s", got)
	}
	flat := New(sample(), Options{Sort: true, Flatten: true})
	if got := names(flat.Roots(), flat); got != ".github / workflows,src,.gitignore,go.mod,README.md" {
		t.Fatalf("flattened roots %s", got)
	}
	if !flat.IsDir(".github/workflows") || flat.Has(".github") || flat.row(".github") != ".github/workflows" {
		t.Fatal("a chain is one row, its deepest directory's")
	}
	if got := flat.ancestors(".github/workflows/ci.yml"); !slices.Equal(got, []string{".github/workflows"}) {
		t.Fatalf("ancestors %v", got)
	}
	filtered := tr.Filter(func(_, name string) bool { return strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".tsx") })
	if got := names(filtered.Roots(), filtered); got != "src" || filtered.Files() != 2 || filtered.Has("src/main.go") {
		t.Fatalf("filtered %s, %d files", got, filtered.Files())
	}
	paths := FromPaths([]string{"b/c/d.go", "a.go", "b/e.go"}, Options{Sort: true, Flatten: true})
	if got := names(paths.Roots(), paths); got != "b,a.go" || names(paths.Children("b"), paths) != "c,e.go" {
		t.Fatalf("from paths %s / %s", got, names(paths.Children("b"), paths))
	}
}

// Siblings sort as the web tree's path store sorts them; the lists are
// path-store's compareSegmentValues output for the same names.
func TestSortMatchesThePathStore(t *testing.T) {
	for _, want := range [][]string{
		{"-x.md", ".env", "_u.md", "a.md", "B.md", "file2.md", "file10.md"},
		{".github", ".gitignore", "9", "10", "a09", "A9", "a9", "a10", "a.md", "B.md", "b.md", "go.mod", "README.md", "x1y2", "x1y10", "z", "Zed", "~t", "ä.md"},
	} {
		nodes := make([]Node, len(want))
		for i, name := range want {
			nodes[len(want)-1-i] = Node{Name: name, Path: name}
		}
		if got := New(nodes, Options{Sort: true}).Roots(); !slices.Equal(got, want) {
			t.Fatalf("sorted %q, want %q", got, want)
		}
	}
	mixed := New([]Node{{Name: "a.md", Path: "a.md"}, {Name: "z", Path: "z", Dir: true}, {Name: ".env", Path: ".env"}}, Options{Sort: true})
	if got := mixed.Roots(); !slices.Equal(got, []string{"z", ".env", "a.md"}) {
		t.Fatalf("folders first: %q", got)
	}
}

// harness hosts a tree view in a Tester.
type harness struct {
	t     *testing.T
	tt    *ui.Tester
	state *State
	props Props
	last  Result
	log   []Result
}

func newHarness(t *testing.T, tr *Tree) *harness {
	h := &harness{t: t, state: &State{}, props: Props{Tree: tr, Label: "Files"}}
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			h.last = View(c, h.state, h.props)
			if h.last.Activated != "" || h.last.Toggled != "" || h.last.Moved {
				h.log = append(h.log, h.last)
			}
		})
	}, 320, 400)
	h.tt.Frame()
	return h
}

func (h *harness) click(text string) {
	h.t.Helper()
	if err := h.tt.Click(text); err != nil {
		h.t.Fatalf("click %q: %v; texts %q", text, err, h.tt.Texts())
	}
	h.tt.Frame()
}

func (h *harness) key(key ui.Key) {
	h.tt.Key(0, key)
	h.tt.Frame()
}

func (h *harness) frame() { h.tt.Frame() }

func (h *harness) rows() string { return names(h.state.Rows(), h.props.Tree) }

func TestClicksToggleFoldersAndSelectFiles(t *testing.T) {
	h := newHarness(t, New(sample(), Options{Sort: true}))
	if got := h.rows(); got != ".github,src,.gitignore,go.mod,README.md" {
		t.Fatalf("rows %s", got)
	}
	h.click("src")
	if got := h.rows(); got != ".github,src,app,main.go,.gitignore,go.mod,README.md" {
		t.Fatalf("after opening src %s", got)
	}
	h.click("main.go")
	if h.state.Selected != "src/main.go" || h.last.Activated != "" {
		t.Fatalf("selected %q", h.state.Selected)
	}
	if got := h.log[len(h.log)-1]; got.Activated != "src/main.go" || got.Submitted {
		t.Fatalf("result %+v", got)
	}
	if h.state.Focused() != "src/main.go" {
		t.Fatalf("focused %q", h.state.Focused())
	}
	h.click("src")
	// A click selects a folder too, as on Pierre's row button.
	if got := h.rows(); got != ".github,src,.gitignore,go.mod,README.md" || h.state.Selected != "src" {
		t.Fatalf("after closing src %s, selected %q", got, h.state.Selected)
	}
}

func TestKeyboardNavigatesAsPierre(t *testing.T) {
	h := newHarness(t, New(sample(), Options{Sort: true}))
	h.click(".github") // opens it, cursor on it
	h.key(ui.KeyLeft)  // closes the open folder
	if h.state.IsOpen(".github") {
		t.Fatal("Left closes an open folder")
	}
	h.key(ui.KeyRight) // opens a closed folder
	if !h.state.IsOpen(".github") || h.state.Focused() != ".github" {
		t.Fatalf("Right opens; focused %q", h.state.Focused())
	}
	h.key(ui.KeyRight) // moves into it
	if h.state.Focused() != ".github/workflows" {
		t.Fatalf("Right on an open folder goes to its child, at %q", h.state.Focused())
	}
	h.key(ui.KeyRight) // opens workflows
	h.key(ui.KeyDown)
	if h.state.Focused() != ".github/workflows/ci.yml" {
		t.Fatalf("Down at %q", h.state.Focused())
	}
	if h.state.Selected != ".github" {
		t.Fatalf("only the click selects: the cursor's moves do not, as Pierre's focus; selected %q", h.state.Selected)
	}
	h.key(ui.KeyLeft) // a file goes to its parent
	if h.state.Focused() != ".github/workflows" {
		t.Fatalf("Left on a file at %q", h.state.Focused())
	}
	h.key(ui.KeyEnd)
	if h.state.Focused() != "README.md" {
		t.Fatalf("End at %q", h.state.Focused())
	}
	h.key(ui.KeyEnter)
	if h.state.Selected != "README.md" {
		t.Fatalf("Enter selects; selected %q", h.state.Selected)
	}
	if got := h.log[len(h.log)-1]; got.Activated != "README.md" || !got.Submitted {
		t.Fatalf("Enter result %+v", got)
	}
	h.key(ui.KeyHome)
	h.key(ui.KeySpace) // toggles .github closed
	if h.state.IsOpen(".github") || h.state.Focused() != ".github" {
		t.Fatalf("Space toggles the folder; focused %q", h.state.Focused())
	}
	// Pierre has no type-ahead: letters leave the cursor.
	h.key(ui.KeyG)
	h.key(ui.KeyO)
	if h.state.Focused() != ".github" {
		t.Fatalf("letters moved the cursor to %q", h.state.Focused())
	}
	h.key(ui.KeyUp)
	if h.state.Focused() != ".github" {
		t.Fatalf("Up at the top stays; at %q", h.state.Focused())
	}
	if h.state.Selected != ".github" {
		t.Fatalf("the selection is the activated row, not the cursor's; %q", h.state.Selected)
	}
}

// From no cursor, Down moves as Pierre's moveFocus: from the first row.
func TestDownFromNoCursor(t *testing.T) {
	h := newHarness(t, New(sample(), Options{Sort: true}))
	h.state.focusList = true
	h.frame()
	h.key(ui.KeyDown)
	if h.state.Focused() != "src" || h.state.Selected != "" {
		t.Fatalf("focused %q, selected %q", h.state.Focused(), h.state.Selected)
	}
}

// A flattened chain is its deepest directory's row: opening it opens the
// whole chain, so a change that breaks the chain shows its folders open;
// closing it closes the deepest only, and a chain a change forms shows
// its deepest directory's state, as Pierre's path store does.
func TestFlattenedChainsKeepPierresOpenState(t *testing.T) {
	chain := func(extra ...Node) *Tree {
		children := append([]Node{{Name: "app", Path: "src/app", Dir: true, Children: []Node{{Name: "main.go", Path: "src/app/main.go"}}}}, extra...)
		return New([]Node{{Name: "src", Path: "src", Dir: true, Children: children}}, Options{Sort: true, Flatten: true})
	}
	h := newHarness(t, chain())
	h.click("src / app")
	if got := h.state.Rows(); !slices.Equal(got, []string{"src/app", "src/app/main.go"}) {
		t.Fatalf("chain rows %v", got)
	}
	h.props.Tree = chain(Node{Name: "x.md", Path: "src/x.md"})
	h.frame()
	if got := h.state.Rows(); !slices.Equal(got, []string{"src", "src/app", "src/app/main.go", "src/x.md"}) {
		t.Fatalf("a broken chain keeps its folders open: %v", got)
	}
	h.click("app")
	h.props.Tree = chain()
	h.frame()
	if got := h.state.Rows(); !slices.Equal(got, []string{"src/app"}) || h.state.IsOpen("src/app") || !h.state.IsOpen("src") {
		t.Fatalf("a formed chain shows its deepest folder's state: %v", got)
	}
}

// Reset forgets the open folders, selection, and cursor.
func TestResetForgetsState(t *testing.T) {
	h := newHarness(t, New(sample(), Options{Sort: true}))
	h.click("src")
	h.click("main.go")
	h.state.Reset()
	h.frame()
	if h.state.Selected != "" || h.state.Focused() != "" || h.state.IsOpen("src") || len(h.state.Rows()) != 5 {
		t.Fatalf("selected %q, focused %q, rows %v", h.state.Selected, h.state.Focused(), h.state.Rows())
	}
}

// Rows are keyed by path, and decorations resolve once per tree and map.
func TestRowsKeyByPathAndDecorationsResolveOnce(t *testing.T) {
	tr := New(sample(), Options{Sort: true})
	h := newHarness(t, tr)
	h.props.Decorations = map[string]Decoration{"src/main.go": {Status: Modified}}
	h.frame()
	if h.state.list.Key == nil || h.state.list.Key(1) != "src" {
		t.Fatal("rows key by path")
	}
	first := h.state.marks
	h.frame()
	if reflect.ValueOf(first).UnsafePointer() != reflect.ValueOf(h.state.marks).UnsafePointer() {
		t.Fatal("the same tree and map resolve once")
	}
	h.props.Decorations = map[string]Decoration{"README.md": {Status: Added}}
	h.frame()
	if h.state.marks["src"].contains || h.state.marks["README.md"].Status != Added {
		t.Fatalf("a new map resolves again: %+v", h.state.marks)
	}
}

func TestStateSurvivesRefreshAndFilter(t *testing.T) {
	h := newHarness(t, New(sample(), Options{Sort: true}))
	h.click("src")
	h.click("app")
	h.click("index.ts")
	// A live update brings a new tree with one more file above.
	refreshed := append(sample(), Node{Name: "AGENTS.md", Path: "AGENTS.md"})
	refreshed[1].Children = append(refreshed[1].Children, Node{Name: "util.go", Path: "src/util.go"})
	h.props.Tree = New(refreshed, Options{Sort: true})
	h.tt.Frame()
	if !h.state.IsOpen("src/app") || h.state.Selected != "src/app/index.ts" || h.state.Focused() != "src/app/index.ts" {
		t.Fatalf("after refresh open %v selected %q focused %q", h.state.OpenPaths(), h.state.Selected, h.state.Focused())
	}
	if !h.tt.HasText("util.go") {
		t.Fatal("the new file shows")
	}
	// A filter hides rows only; clearing it restores them, still open.
	full := h.props.Tree
	h.props.Tree = full.Filter(func(_, name string) bool { return strings.HasSuffix(name, ".md") })
	h.tt.Frame()
	if got := h.rows(); got != "AGENTS.md,README.md" {
		t.Fatalf("filtered rows %s", got)
	}
	if h.state.Selected != "src/app/index.ts" {
		t.Fatal("filtering keeps the selection")
	}
	h.props.Tree = full
	h.tt.Frame()
	if !h.state.IsOpen("src") || !h.state.IsOpen("src/app") || !h.tt.HasText("index.ts") {
		t.Fatalf("clearing the filter restores expansion; open %v", h.state.OpenPaths())
	}
}

func TestRevealOpensAncestorsWithoutMovingTheCursor(t *testing.T) {
	h := newHarness(t, New(sample(), Options{Sort: true}))
	h.state.Reveal(h.props.Tree, "src/app/App.tsx")
	h.state.Selected = "src/app/App.tsx"
	h.tt.Frame()
	if !h.tt.HasText("App.tsx") || h.state.Focused() != "" {
		t.Fatalf("revealed; focused %q", h.state.Focused())
	}
	h.state.CollapseAll()
	h.tt.Frame()
	if h.state.AnyOpen(h.props.Tree) || h.tt.HasText("App.tsx") {
		t.Fatal("collapse all")
	}
	h.state.ExpandAll(h.props.Tree)
	h.tt.Frame()
	if !h.tt.HasText("ci.yml") {
		t.Fatal("expand all")
	}
}

func TestDecorationsShowLettersPartsDotsAndIgnoredInheritance(t *testing.T) {
	tr := New(sample(), Options{Sort: true})
	h := newHarness(t, tr)
	h.state.SetOpen([]string{"src", "src/app", ".github", ".github/workflows"})
	h.props.Decorations = map[string]Decoration{
		"src/main.go":     {Status: Modified, Parts: []Part{{Text: "+3"}, {Text: " "}, {Text: "−1"}}, Title: "3 additions"},
		"src/app/App.tsx": {Status: Added, Viewed: true},
		".github":         {Status: Ignored},
		"go.mod":          {Dimmed: true},
	}
	h.tt.Frame()
	for _, text := range []string{"M", "A", "+3", "−1"} {
		if !h.tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, h.tt.Texts())
		}
	}
	marks := decorations(tr, h.props.Decorations)
	if !marks["src"].contains || !marks["src/app"].contains || marks[".github"].contains {
		t.Fatal("folders holding changes carry the dot; ignored ones do not")
	}
	if marks[".github/workflows/ci.yml"].status() != Ignored || marks["src/main.go"].status() != Modified {
		t.Fatal("an ignored directory's rows are ignored")
	}
	if Ignored.Letter() != "" || Renamed.Letter() != "R" || Untracked.Letter() != "U" || Deleted.Letter() != "D" {
		t.Fatal("letters")
	}
}

func TestMatchesAndContextMenu(t *testing.T) {
	h := newHarness(t, New(sample(), Options{Sort: true}))
	h.props.Match = func(_, name string) [][2]int {
		if i := strings.Index(name, "go"); i >= 0 {
			return [][2]int{{i, i + 2}}
		}
		return nil
	}
	var chosen string
	h.props.Menu = func(m *ui.Menu, path string) {
		if m.Item("Copy Path").Chosen() {
			chosen = path
		}
	}
	h.tt.Frame()
	if !h.tt.HasText("go.mod") {
		t.Fatalf("a matched name keeps its text; %q", h.tt.Texts())
	}
	if err := h.tt.RightClick("go.mod"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if !slices.Contains(h.tt.Menu(), "Copy Path") {
		t.Fatalf("menu %q", h.tt.Menu())
	}
	if err := h.tt.ChooseMenuItem("Copy Path"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if chosen != "go.mod" {
		t.Fatalf("menu acted on %q", chosen)
	}
}

// TestShot writes the tree's light and dark look to $TREES_SHOTS, for
// reading by eye (shots are artifacts, never goldens).
func TestShot(t *testing.T) {
	dir := os.Getenv("TREES_SHOTS")
	if dir == "" {
		t.Skip("TREES_SHOTS unset")
	}
	var nodes []Node
	for _, name := range []string{"main.go", "App.tsx", "index.ts", "styles.css", "theme.scss", "page.html", "data.json", "notes.md", "config.yml", "Dockerfile", "package.json", ".gitignore", "lib.rs", "app.py", "query.sql", "logo.png", "font.woff2", "archive.zip", "run.sh", "unknown.qqq", "vite.config.ts", "README.txt"} {
		nodes = append(nodes, Node{Name: name, Path: "files/" + name})
	}
	tr := New([]Node{{Name: "files", Path: "files", Dir: true, Children: nodes}, {Name: "closed", Path: "closed", Dir: true, Children: []Node{{Name: "x.go", Path: "closed/x.go"}}}}, Options{Sort: true})
	for _, dark := range []bool{false, true} {
		h := newHarness(t, tr)
		h.tt.SetSize(320, 640)
		h.tt.SetDark(dark)
		h.props.Theme = DefaultTheme(dark)
		h.state.SetOpen([]string{"files"})
		h.state.Selected = "files/index.ts"
		h.props.Decorations = map[string]Decoration{"files/main.go": {Status: Modified}, "files/App.tsx": {Status: Added, Parts: []Part{{Text: "+12"}}}, "files/unknown.qqq": {Status: Ignored}, "files/notes.md": {Viewed: true}}
		h.tt.Frame()
		h.tt.Frame()
		name := map[bool]string{false: "light.png", true: "dark.png"}[dark]
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, h.tt.Image()); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
}
