package navigation

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/document"
)

// memorySource is an in-memory project. gate, when set, blocks Files until
// closed so tests can observe an unfinished build.
type memorySource struct {
	mu    sync.Mutex
	files map[string]string
	stamp map[string]time.Time
	gate  chan struct{}
}

func newSource(files map[string]string) *memorySource {
	source := &memorySource{files: map[string]string{}, stamp: map[string]time.Time{}}
	for name, text := range files {
		source.write(name, text)
	}
	return source
}

func (s *memorySource) write(name, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[name] = text
	s.stamp[name] = time.Now().Add(time.Duration(len(s.stamp)) * time.Nanosecond)
}

func (s *memorySource) Files() ([]File, error) {
	if s.gate != nil {
		<-s.gate
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]File, 0, len(s.files))
	for name, text := range s.files {
		result = append(result, File{Path: name, Size: int64(len(text)), Modified: s.stamp[name]})
	}
	return result, nil
}

func (s *memorySource) Read(name string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, ok := s.files[name]
	return []byte(text), ok, nil
}

// position finds the nth (0-based) occurrence of marker in a file.
func position(t *testing.T, source *memorySource, name, marker string, nth int) document.Position {
	t.Helper()
	text := source.files[name]
	offset := -1
	for index := 0; index <= nth; index++ {
		next := strings.Index(text[offset+1:], marker)
		if next < 0 {
			t.Fatalf("%s: marker %q #%d not found", name, marker, nth)
		}
		offset += next + 1
	}
	before := text[:offset]
	return document.Position{Line: strings.Count(before, "\n") + 1, Column: len(before) - strings.LastIndex(before, "\n") - 1}
}

func ready(t *testing.T, source *memorySource) *Index {
	t.Helper()
	index := New(document.Builtins())
	index.interval = 0
	index.warm("project", source)
	deadline := time.Now().Add(10 * time.Second)
	for !index.Ready("project") {
		if time.Now().After(deadline) {
			t.Fatal("index never became ready")
		}
		time.Sleep(time.Millisecond)
	}
	return index
}

type target struct {
	path   string
	marker string
	nth    int
}

func expect(t *testing.T, source *memorySource, result Result, want ...target) {
	t.Helper()
	if len(result.Locations) != len(want) {
		t.Fatalf("locations = %+v, want %d", result.Locations, len(want))
	}
	for index, location := range result.Locations {
		at := position(t, source, want[index].path, want[index].marker, want[index].nth)
		if location.Path != want[index].path || location.Line != at.Line || location.Column != at.Column {
			t.Errorf("location %d = %+v, want %s at %+v", index, location, want[index].path, at)
		}
	}
}

func goProject() *memorySource {
	return newSource(map[string]string{
		"go.mod": "module example.com/app\n",
		"app/main.go": `package app

import (
	"fmt"
	"example.com/app/internal/files"
)

func Run() {
	server := NewServer()
	server.Run()
	fmt.Println(files.Scan(), helper())
	Run()
}
`,
		"app/helper.go":           "package app\n\nfunc helper() string { return \"Run\" }\n\nfunc NewServer() *Server { return &Server{} }\n",
		"app/server.go":           "package app\n\ntype Server struct{}\n\nfunc (s *Server) Run() {}\n",
		"other/helper.go":         "package other\n\nfunc helper() {}\n\nfunc Run() { Run := 1; _ = Run }\n",
		"internal/files/scan.go":  "package files\n\nfunc Scan() int { return 0 }\n",
		"internal/search/scan.go": "package search\n\nfunc Scan() int { return 1 }\n",
	})
}

func TestDefinitionResolvesAcrossGoFiles(t *testing.T) {
	source := goProject()
	index := ready(t, source)
	definition := func(marker string, nth int) Result {
		t.Helper()
		result, err := index.Definition("project", source, "app/main.go", position(t, source, "app/main.go", marker, nth), true)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	// Same package wins over another package's helper.
	expect(t, source, definition("helper()", 0), target{"app/helper.go", "helper", 0})
	// An import path picks its package directory.
	expect(t, source, definition("Scan", 0), target{"internal/files/scan.go", "Scan", 0})
	// A selector on a value finds the method, not the same-named function.
	expect(t, source, definition("Run()\n\tfmt", 0), target{"app/server.go", "Run", 0})
	// A plain call finds the function in the same file.
	expect(t, source, definition("Run()\n}", 0), target{"app/main.go", "Run", 0})
	// A local resolves within the file.
	expect(t, source, definition("server.Run", 0), target{"app/main.go", "server :=", 0})
	// Modules outside the project resolve to the import.
	expect(t, source, definition("Println", 0), target{"app/main.go", `"fmt"`, 0})
	// A definition resolves to itself.
	expect(t, source, definition("Run() {", 0), target{"app/main.go", "Run() {", 0})
	if result := definition("\n\nfunc Run", 0); result.Symbol != "" || len(result.Locations) != 0 {
		t.Fatalf("whitespace = %+v", result)
	}
}

func TestDefinitionResolvesJavaScriptModules(t *testing.T) {
	source := newSource(map[string]string{
		"src/app.tsx":         "import Widget, { make, shared } from './lib/widget'\nimport { util } from './utils'\nexport const App = () => <Widget value={make(util(shared))} />\n",
		"src/lib/widget.ts":   "export default class Widget {}\nexport function make() {}\n",
		"src/lib/shared.js":   "export const shared = 1\n",
		"src/utils/index.ts":  "export { util } from './impl'\n",
		"src/utils/impl.ts":   "export function util() {}\n",
		"src/other/widget.ts": "export function make() {}\n",
	})
	index := ready(t, source)
	definition := func(marker string) Result {
		t.Helper()
		result, err := index.Definition("project", source, "src/app.tsx", position(t, source, "src/app.tsx", marker, 0), true)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	expect(t, source, definition("make("), target{"src/lib/widget.ts", "make", 0})
	expect(t, source, definition("Widget value"), target{"src/lib/widget.ts", "Widget", 0})
	// The barrel re-exports util; the name search finds the implementation.
	expect(t, source, definition("util("), target{"src/utils/impl.ts", "util", 0})
	// The module does not define shared; the name search finds it elsewhere.
	expect(t, source, definition("shared))"), target{"src/lib/shared.js", "shared", 0})
}

func TestFirstUseIndexesAndAnswersBeforeTheBuildFinishes(t *testing.T) {
	source := goProject()
	index := New(document.Builtins())
	// Hold the background build so the first answers come without it.
	building := index.project("project")
	building.refresh.Lock()
	definition, err := index.Definition("project", source, "app/main.go", position(t, source, "app/main.go", "Scan", 0), true)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, source, definition, target{"internal/files/scan.go", "Scan", 0})
	references, err := index.References(context.Background(), "project", source, "app/main.go", position(t, source, "app/main.go", "Scan", 0), true)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, source, references, target{"app/main.go", "Scan", 0}, target{"internal/files/scan.go", "Scan", 0})
	if index.Ready("project") {
		t.Fatal("ready before the build ran")
	}
	building.refresh.Unlock()
	deadline := time.Now().Add(10 * time.Second)
	for !index.Ready("project") {
		if time.Now().After(deadline) {
			t.Fatal("first use did not start indexing")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDefinitionSeesEditedFiles(t *testing.T) {
	source := goProject()
	index := ready(t, source)
	source.write("app/helper.go", "package app\n\n// moved\n\nfunc helper() string { return \"\" }\n")
	result, err := index.Definition("project", source, "app/main.go", position(t, source, "app/main.go", "helper()", 0), true)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, source, result, target{"app/helper.go", "helper", 0})
}

func TestDefinitionRejectsFilesWithoutNavigation(t *testing.T) {
	source := newSource(map[string]string{"notes.md": "# Notes\n"})
	if _, err := New(document.Builtins()).Definition("project", source, "notes.md", document.Position{Line: 1}, true); err != ErrNotNavigable {
		t.Fatalf("error = %v, want ErrNotNavigable", err)
	}
	if _, err := New(document.Builtins()).Definition("project", source, "gone.go", document.Position{Line: 1}, true); err != ErrUnavailable {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestReferencesFindProjectUsesBySymbol(t *testing.T) {
	source := goProject()
	index := ready(t, source)
	references := func(name, marker string, nth int) Result {
		t.Helper()
		result, err := index.References(context.Background(), "project", source, name, position(t, source, name, marker, nth), true)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	// The package's Run function: not the method, another package's Run, the
	// shadowing local, or the string "Run".
	expect(t, source, references("app/main.go", "Run() {", 0),
		target{"app/main.go", "Run() {", 0}, target{"app/main.go", "Run()\n}", 0})
	// The method and its uses.
	expect(t, source, references("app/server.go", "Run", 0),
		target{"app/server.go", "Run", 0}, target{"app/main.go", "Run()\n\tfmt", 0})
	// A local stays in its scope.
	expect(t, source, references("app/main.go", "server :=", 0),
		target{"app/main.go", "server :=", 0}, target{"app/main.go", "server.Run", 0})
	// The definition and the use through its import, not the other package.
	result := references("internal/files/scan.go", "Scan", 0)
	if result.Symbol != "Scan" || result.Locations[0].Kind != "definition" {
		t.Fatalf("result = %+v", result)
	}
	expect(t, source, result, target{"internal/files/scan.go", "Scan", 0}, target{"app/main.go", "Scan", 0})
	// From a use, the same answer.
	expect(t, source, references("app/main.go", "Scan", 0),
		target{"app/main.go", "Scan", 0}, target{"internal/files/scan.go", "Scan", 0})
	// Outside the project: uses of the same import.
	expect(t, source, references("app/main.go", "Println", 0), target{"app/main.go", "Println", 0})
}

func TestReferencesFollowImportedNames(t *testing.T) {
	source := newSource(map[string]string{
		"a.ts": "import { make as build } from './b'\nbuild()\n",
		"b.ts": "export function make() {}\nmake()\n",
		"c.ts": "const make = 1\nconsole.log(make, 'make')\n",
	})
	result, err := ready(t, source).References(context.Background(), "project", source, "a.ts", position(t, source, "a.ts", "build()", 0), true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Symbol != "make" {
		t.Fatalf("symbol = %q", result.Symbol)
	}
	// The original name in the import, then b.ts; c.ts's make is its own.
	expect(t, source, result, target{"a.ts", "make", 0}, target{"b.ts", "make", 0}, target{"b.ts", "make()", 1})
}

func TestSnippetTrimsAndBoundsLines(t *testing.T) {
	lines := splitLines([]byte("\tfunc x() {}\r\n" + strings.Repeat("é", maxSnippet+5)))
	if got := snippet(lines, 1); got != "func x() {}" {
		t.Fatalf("snippet = %q", got)
	}
	if got := snippet(lines, 2); !strings.HasSuffix(got, "…") || len([]rune(got)) != maxSnippet+1 {
		t.Fatalf("long snippet = %q", got)
	}
	if snippet(lines, 0) != "" || snippet(lines, 9) != "" {
		t.Fatal("out-of-range snippet")
	}
}

func TestMinifiedBundlesAreNotIndexed(t *testing.T) {
	bundle := "function render(){}" + strings.Repeat(";render()", minifiedLine/9+1) + "\n"
	source := newSource(map[string]string{
		"dist/app.js":   bundle,
		"src/use.js":    "render()\n",
		"src/render.js": "export function render() {}\n",
	})
	index := ready(t, source)
	result, err := index.Definition("project", source, "src/use.js", position(t, source, "src/use.js", "render", 0), true)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, source, result, target{"src/render.js", "render", 0})
	references, err := index.References(context.Background(), "project", source, "src/render.js", position(t, source, "src/render.js", "render", 0), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, location := range references.Locations {
		if location.Path == "dist/app.js" {
			t.Fatalf("references include the minified bundle: %+v", location)
		}
	}
}

func TestWithoutBuildingProjectsAreScannedNotIndexed(t *testing.T) {
	source := goProject()
	index := New(document.Builtins())
	result, err := index.Definition("project", source, "app/main.go", position(t, source, "app/main.go", "Scan", 0), false)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, source, result, target{"internal/files/scan.go", "Scan", 0})
	time.Sleep(20 * time.Millisecond)
	if index.Ready("project") {
		t.Fatal("a request without build indexed the project")
	}
}

func TestWordAtFindsNamesByUTF16Column(t *testing.T) {
	text := []byte("package a\n\tvar 𝒳, héllo_1 = $x\n")
	for _, test := range []struct {
		column int
		want   string
	}{
		{1, "var"}, {5, "𝒳"}, {9, "héllo_1"}, {15, "héllo_1"}, {19, "$x"}, {8, ""},
	} {
		if got := WordAt(text, document.Position{Line: 2, Column: test.column}); got != test.want {
			t.Errorf("column %d = %q, want %q", test.column, got, test.want)
		}
	}
	if got := WordAt(text, document.Position{Line: 9, Column: 0}); got != "" {
		t.Errorf("past the end = %q", got)
	}
}
