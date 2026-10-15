package document

import (
	"strings"
	"testing"
)

// at returns the position of the nth (0-based) occurrence of marker.
func at(t *testing.T, source, marker string, nth int) Position {
	t.Helper()
	offset := -1
	for index := 0; index <= nth; index++ {
		next := strings.Index(source[offset+1:], marker)
		if next < 0 {
			t.Fatalf("marker %q #%d not found", marker, nth)
		}
		offset += next + 1
	}
	before := source[:offset]
	line := strings.Count(before, "\n") + 1
	lineStart := strings.LastIndex(before, "\n") + 1
	return Position{Line: line, Column: utf16Length([]byte(before[lineStart:]))}
}

func navigatorFor(t *testing.T, name string) Navigator {
	t.Helper()
	navigator, ok := Builtins().Lookup(name).Navigator()
	if !ok {
		t.Fatalf("%s is not navigable", name)
	}
	return navigator
}

func bind(t *testing.T, navigator Navigator, source string, position Position) Binding {
	t.Helper()
	binding, ok, err := navigator.Bind([]byte(source), position)
	if err != nil || !ok {
		t.Fatalf("Bind(%v) = %v, %v", position, ok, err)
	}
	return binding
}

func symbolNames(symbols []Symbol) map[string]Symbol {
	result := make(map[string]Symbol, len(symbols))
	for _, symbol := range symbols {
		result[symbol.Name] = symbol
	}
	return result
}

const goSample = `package app

import (
	"fmt"
	str "strings"
	"example.com/app/internal/files"
)

// Server serves New and Run.
type Server struct{ Name string }
type Runner interface{ Run() error }

const Limit = 4

var count int

func New(name string) *Server {
	s := &Server{Name: name}
	if x := count; x > 0 {
		x := "inner"
		fmt.Println(x, str.ToUpper("New"))
	}
	files.Scan(s.Name)
	return s
}

func (s *Server) Run() error { return nil }
`

func TestGoSymbolsIndexPackageLevelDefinitionsOnly(t *testing.T) {
	symbols, err := navigatorFor(t, "a.go").Symbols([]byte(goSample))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolNames(symbols)
	for name, want := range map[string]Symbol{
		"Server": {Kind: "type"}, "Runner": {Kind: "interface"}, "Limit": {Kind: "constant"}, "count": {Kind: "variable"},
		"New": {Kind: "function"}, "Run": {Kind: "method", Member: true}, "Name": {Kind: "field", Member: true},
	} {
		got, ok := byName[name]
		if !ok || got.Kind != want.Kind || got.Member != want.Member {
			t.Errorf("symbol %s = %+v (found %t), want kind %s member %t", name, got, ok, want.Kind, want.Member)
		}
	}
	for _, local := range []string{"s", "x", "name"} {
		if _, ok := byName[local]; ok {
			t.Errorf("local %s indexed", local)
		}
	}
	if got := byName["New"].Position; got != at(t, goSample, "New(name", 0) {
		t.Errorf("New position = %+v", got)
	}
}

func TestGoBindResolvesLocalsImportsAndGlobals(t *testing.T) {
	navigator := navigatorFor(t, "a.go")

	returned := bind(t, navigator, goSample, at(t, goSample, "s\n}", 0))
	if returned.Local == nil || returned.Local.Definition != at(t, goSample, "s :=", 0) || returned.Definition {
		t.Fatalf("return s = %+v", returned)
	}
	parameter := bind(t, navigator, goSample, at(t, goSample, "name}", 0))
	if parameter.Local == nil || parameter.Local.Kind != "parameter" || parameter.Local.Definition != at(t, goSample, "name string", 0) {
		t.Fatalf("name use = %+v", parameter)
	}
	// The inner x shadows the if-initializer x.
	inner := bind(t, navigator, goSample, at(t, goSample, "x, str", 0))
	if inner.Local == nil || inner.Local.Definition != at(t, goSample, `x := "inner"`, 0) {
		t.Fatalf("shadowed x = %+v", inner)
	}
	println := bind(t, navigator, goSample, at(t, goSample, "Println", 0))
	if println.Import == nil || println.Import.Name != "Println" || println.Import.Module != "fmt" || println.Member {
		t.Fatalf("fmt.Println = %+v", println)
	}
	aliased := bind(t, navigator, goSample, at(t, goSample, "ToUpper", 0))
	if aliased.Import == nil || aliased.Import.Module != "strings" {
		t.Fatalf("str.ToUpper = %+v", aliased)
	}
	scan := bind(t, navigator, goSample, at(t, goSample, "Scan", 0))
	if scan.Import == nil || scan.Import.Module != "example.com/app/internal/files" || scan.Import.Statement != at(t, goSample, `"example.com`, 0) {
		t.Fatalf("files.Scan = %+v", scan)
	}
	pkg := bind(t, navigator, goSample, at(t, goSample, "files.Scan", 0))
	if pkg.Local == nil || pkg.Local.Kind != "import" {
		t.Fatalf("files package = %+v", pkg)
	}
	server := bind(t, navigator, goSample, at(t, goSample, "Server{Name", 0))
	if server.Local != nil || server.Import != nil || server.Member || server.Name != "Server" {
		t.Fatalf("Server use = %+v", server)
	}
	field := bind(t, navigator, goSample, at(t, goSample, "Name: name", 0))
	if !field.Member || field.Local != nil {
		t.Fatalf("composite key = %+v", field)
	}
	selector := bind(t, navigator, goSample, at(t, goSample, "Name)", 0))
	if !selector.Member || selector.Import != nil {
		t.Fatalf("s.Name = %+v", selector)
	}
	definition := bind(t, navigator, goSample, at(t, goSample, "New(name", 0))
	if !definition.Definition || definition.Local != nil {
		t.Fatalf("New declaration = %+v", definition)
	}
	if _, ok, err := navigator.Bind([]byte(goSample), at(t, goSample, "Server serves", 0)); ok || err != nil {
		t.Fatalf("comment bound = %t, %v", ok, err)
	}
	if _, ok, _ := navigator.Bind([]byte(goSample), at(t, goSample, `"New"`, 0)); ok {
		t.Fatal("string literal bound")
	}
}

func TestGoOccurrencesSkipStringsCommentsAndMarkLocals(t *testing.T) {
	navigator := navigatorFor(t, "a.go")
	occurrences, err := navigator.Occurrences([]byte(goSample), "New")
	if err != nil {
		t.Fatal(err)
	}
	if len(occurrences) != 1 || !occurrences[0].Definition {
		t.Fatalf("New occurrences = %+v, want only the declaration", occurrences)
	}
	locals, err := navigator.Occurrences([]byte(goSample), "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(locals) != 4 {
		t.Fatalf("s occurrences = %+v", locals)
	}
	for _, occurrence := range locals {
		if !occurrence.Local {
			t.Fatalf("s occurrence not local: %+v", occurrence)
		}
	}
	scans, err := navigator.Occurrences([]byte(goSample), "Scan")
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) != 1 || scans[0].Member || scans[0].Local {
		t.Fatalf("files.Scan occurrence = %+v, want a package-level reference", scans)
	}
}

func TestGoProvidesMatchesPackageDirectoryBySuffix(t *testing.T) {
	navigator := navigatorFor(t, "a.go")
	for _, tc := range []struct {
		module, candidate string
		want              bool
	}{
		{"example.com/app/internal/files", "internal/files/scan.go", true},
		{"internal/files", "internal/files/scan.go", true},
		{"example.com/app/internal/files", "internal/web/server.go", false},
		{"example.com/app/xfiles", "files/scan.go", false},
		{"fmt", "main.go", false},
	} {
		if got := navigator.Provides("cmd/root.go", tc.module, tc.candidate); got != tc.want {
			t.Errorf("Provides(%q, %q) = %t, want %t", tc.module, tc.candidate, got, tc.want)
		}
	}
}

const tsSample = `import def, { foo, bar as baz } from "./lib/b"
import * as ns from "@/lib/ns"

export interface Props { name: string }
export type Id = string
export enum Color { Red }

export default class Widget {
  count = 1
  render(x: number): string {
    const y = x + foo() + baz
    return ns.q(y, this.render, "foo", def)
  }
}

// foo in a comment
export function make<T>(p: T): Widget { return new Widget() }
export const arrow = (a: number) => a
`

func TestJavaScriptFamiliesShareOneGroup(t *testing.T) {
	for _, name := range []string{"a.js", "a.jsx", "a.ts", "a.tsx"} {
		if group := navigatorFor(t, name).NavigationGroup(); group != "javascript" {
			t.Errorf("%s group = %q", name, group)
		}
	}
}

func TestTypeScriptSymbolsIndexModuleLevelAndMembers(t *testing.T) {
	symbols, err := navigatorFor(t, "a.ts").Symbols([]byte(tsSample))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolNames(symbols)
	for name, want := range map[string]Symbol{
		"Props": {Kind: "interface"}, "name": {Kind: "property", Member: true}, "Id": {Kind: "type"}, "Color": {Kind: "enum"},
		"Widget": {Kind: "class"}, "count": {Kind: "property", Member: true}, "render": {Kind: "method", Member: true},
		"make": {Kind: "function"}, "arrow": {Kind: "constant"}, "default": {Kind: "default export"},
	} {
		got, ok := byName[name]
		if !ok || got.Kind != want.Kind || got.Member != want.Member {
			t.Errorf("symbol %s = %+v (found %t), want kind %s member %t", name, got, ok, want.Kind, want.Member)
		}
	}
	for _, local := range []string{"x", "y", "p", "a", "T", "foo", "def"} {
		if _, ok := byName[local]; ok {
			t.Errorf("local %s indexed", local)
		}
	}
	if byName["default"].Position != byName["Widget"].Position {
		t.Errorf("default export at %+v, want the class name", byName["default"].Position)
	}
}

func TestTypeScriptBindResolvesImportsLocalsAndMembers(t *testing.T) {
	navigator := navigatorFor(t, "a.ts")
	for _, tc := range []struct {
		marker string
		name   string
		module string
	}{
		{"foo()", "foo", "./lib/b"},
		{"baz\n", "bar", "./lib/b"},
		{"def)", "default", "./lib/b"},
		{"q(y", "q", "@/lib/ns"},
	} {
		binding := bind(t, navigator, tsSample, at(t, tsSample, tc.marker, 0))
		if binding.Import == nil || binding.Import.Name != tc.name || binding.Import.Module != tc.module || binding.Member {
			t.Errorf("%s = %+v", tc.marker, binding)
		}
	}
	y := bind(t, navigator, tsSample, at(t, tsSample, "y, this", 0))
	if y.Local == nil || y.Local.Kind != "constant" || y.Local.Definition != at(t, tsSample, "y = x", 0) {
		t.Fatalf("y = %+v", y)
	}
	x := bind(t, navigator, tsSample, at(t, tsSample, "x + foo", 0))
	if x.Local == nil || x.Local.Kind != "parameter" {
		t.Fatalf("x = %+v", x)
	}
	render := bind(t, navigator, tsSample, at(t, tsSample, "render,", 0))
	if !render.Member || render.Local != nil {
		t.Fatalf("this.render = %+v", render)
	}
	widget := bind(t, navigator, tsSample, at(t, tsSample, "Widget()", 0))
	if widget.Local != nil || widget.Import != nil || widget.Member {
		t.Fatalf("new Widget = %+v", widget)
	}
	namespace := bind(t, navigator, tsSample, at(t, tsSample, "ns.q", 0))
	if namespace.Local == nil || namespace.Local.Kind != "import" {
		t.Fatalf("ns = %+v", namespace)
	}
	occurrences, err := navigator.Occurrences([]byte(tsSample), "foo")
	if err != nil {
		t.Fatal(err)
	}
	if len(occurrences) != 2 || occurrences[0].Local || occurrences[1].Local {
		t.Fatalf("foo occurrences = %+v, want import and call only", occurrences)
	}
}

func TestTSXAndJSXBindComponents(t *testing.T) {
	const source = "import { List } from './List'\nexport function App({ items }: Props) {\n  return <List items={items} />\n}\n"
	for _, name := range []string{"app.tsx", "app.jsx"} {
		code := source
		if name == "app.jsx" {
			code = strings.Replace(source, ": Props", "", 1)
		}
		navigator := navigatorFor(t, name)
		component := bind(t, navigator, code, at(t, code, "List items", 0))
		if component.Import == nil || component.Import.Name != "List" || component.Import.Module != "./List" {
			t.Errorf("%s component = %+v", name, component)
		}
		items := bind(t, navigator, code, at(t, code, "items}", 0))
		if items.Local == nil || items.Local.Kind != "parameter" {
			t.Errorf("%s items = %+v", name, items)
		}
		attribute := bind(t, navigator, code, at(t, code, "items=", 0))
		if !attribute.Member {
			t.Errorf("%s attribute = %+v", name, attribute)
		}
	}
}

func TestJavaScriptProvidesResolvesModules(t *testing.T) {
	navigator := navigatorFor(t, "a.ts")
	for _, tc := range []struct {
		importer, module, candidate string
		want                        bool
	}{
		{"src/app.ts", "./lib/b", "src/lib/b.ts", true},
		{"src/app.ts", "./lib/b", "src/lib/b.tsx", true},
		{"src/app.ts", "./lib/b", "src/lib/b/index.ts", true},
		{"src/app.ts", "./lib/b.js", "src/lib/b.ts", true},
		{"src/app.ts", "../types", "types.d.ts", true},
		{"src/app.ts", "./lib/b", "src/other/b.ts", false},
		{"src/app.ts", "./lib/b", "src/lib/b.css", false},
		{"web/src/App.tsx", "@/components/ui/button", "web/src/components/ui/button.tsx", true},
		{"web/src/App.tsx", "react", "react.ts", false},
	} {
		if got := navigator.Provides(tc.importer, tc.module, tc.candidate); got != tc.want {
			t.Errorf("Provides(%q, %q, %q) = %t, want %t", tc.importer, tc.module, tc.candidate, got, tc.want)
		}
	}
}

func TestNavigationColumnsCountUTF16(t *testing.T) {
	const source = "const s = \"😀é\"; const value = s\n"
	navigator := navigatorFor(t, "a.js")
	symbols, err := navigator.Symbols([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	// 😀 is two UTF-16 code units and é one, so value starts at column 23.
	if got := symbolNames(symbols)["value"].Position; got != (Position{Line: 1, Column: 23}) {
		t.Fatalf("value position = %+v", got)
	}
	use := bind(t, navigator, source, Position{Line: 1, Column: 31})
	if use.Name != "s" || use.Position != (Position{Line: 1, Column: 31}) {
		t.Fatalf("use = %+v", use)
	}
}

func TestNavigationToleratesMalformedSource(t *testing.T) {
	for name, source := range map[string]string{
		"a.go": "package app\n\nfunc Good() {}\n\nfunc broken( {\n",
		"a.ts": "export function good() {}\nexport class {{{ \nconst x = \n",
	} {
		navigator := navigatorFor(t, name)
		symbols, err := navigator.Symbols([]byte(source))
		if err != nil {
			t.Fatalf("%s Symbols error = %v", name, err)
		}
		found := false
		for _, symbol := range symbols {
			found = found || strings.EqualFold(symbol.Name, "good")
		}
		if !found {
			t.Errorf("%s symbols = %+v, want the valid definition", name, symbols)
		}
		if _, _, err := navigator.Bind([]byte(source), Position{Line: 99, Column: 4}); err != nil {
			t.Errorf("%s Bind past end error = %v", name, err)
		}
	}
	if _, err := navigatorFor(t, "a.ts").Symbols([]byte{0xff, 0xfe, 0x00}); err != nil {
		t.Errorf("invalid UTF-8 error = %v", err)
	}
}
