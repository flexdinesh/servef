package document

import (
	"encoding/json"
	"errors"
	"html"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

var htmlTags = regexp.MustCompile(`<[^>]*>`)

func plainHTML(value string) string { return html.UnescapeString(htmlTags.ReplaceAllString(value, "")) }

func plainText() Handler {
	return CodeHandler{Definition: Type{ID: "text", Label: "Plain text", Language: "plaintext", DefaultMode: "source"}}
}

func TestRegistryLookupPrefersFilenameThenExtensionThenFallback(t *testing.T) {
	registry := Builtins()
	for name, want := range map[string]string{
		"README.MD":            "markdown",
		"docs/api.markdown":    "markdown",
		"config.JSON":          "json",
		"compose.YML":          "yaml",
		"config.yaml":          "yaml",
		"logo.svg":             "svg",
		"photo.JPEG":           "image",
		"favicon.ico":          "image",
		"main.go":              "go",
		"app.tsx":              "tsx",
		"deploy/Dockerfile":    "dockerfile",
		"dockerfile":           "dockerfile",
		"GNUmakefile":          "makefile",
		"makefile":             "makefile",
		".env":                 "shell",
		".gitignore":           "text",
		"notes.txt":            "text",
		"README":               "text",
		"archive.tar.gz":       "text",
		"config.jsonc":         "text",
		"Dockerfile.dev":       "text",
		"src/servef.go.backup": "text",
	} {
		if got := registry.Lookup(name).Type().ID; got != want {
			t.Errorf("Lookup(%q) = %s, want %s", name, got, want)
		}
	}
	filename := CodeHandler{Definition: Type{ID: "package", Label: "npm package", Filenames: []string{"package.json"}, Language: "json", DefaultMode: "source"}}
	custom, err := NewRegistry(plainText(), NewJSON(), filename)
	if err != nil {
		t.Fatal(err)
	}
	if got := custom.Lookup("app/Package.JSON").Type().ID; got != "package" {
		t.Fatalf("base name did not take precedence over extension: %s", got)
	}
	if got := custom.Lookup("app/other.json").Type().ID; got != "json" {
		t.Fatalf("extension lookup = %s", got)
	}
}

func TestBuiltinTypesDeclareViewsAndCapabilities(t *testing.T) {
	registry := Builtins()
	types := registry.Types()
	if types[0].ID != "text" || types[0].Label != "Plain text" || types[0].Language != "plaintext" || types[0].PreviewKind != "" || types[0].DefaultMode != "source" || types[0].Formattable {
		t.Fatalf("fallback = %+v", types[0])
	}
	for _, definition := range types {
		formattable := map[string]bool{"json": true, "yaml": true, "html": true, "css": true, "javascript": true}[definition.ID]
		if definition.Formattable != formattable {
			t.Errorf("%s formattable = %t", definition.ID, definition.Formattable)
		}
		if definition.Language != "" && lexers.Get(definition.Language) == nil {
			t.Errorf("%s language %q does not resolve", definition.ID, definition.Language)
		}
		switch definition.ID {
		case "markdown":
			if definition.PreviewKind != PreviewMarkdown || definition.DefaultMode != "source" {
				t.Errorf("markdown = %+v", definition)
			}
		case "svg", "image":
			if definition.PreviewKind != PreviewImage || definition.PreviewLabel != "Preview" || definition.DefaultMode != "preview" {
				t.Errorf("%s = %+v", definition.ID, definition)
			}
		default:
			if definition.PreviewKind != "" || definition.DefaultMode != "source" {
				t.Errorf("%s should be source-only: %+v", definition.ID, definition)
			}
		}
	}
	encoded, err := json.Marshal(registry.Lookup("Dockerfile").Type())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"dockerfile","label":"Dockerfile","extensions":[],"filenames":["Dockerfile"],"language":"docker","previewKind":"","previewLabel":"","defaultMode":"source","formattable":false}`
	if string(encoded) != want {
		t.Fatalf("type JSON = %s, want %s", encoded, want)
	}
}

func TestRegistryRejectsInvalidComposition(t *testing.T) {
	source := func(definition Type) Handler {
		if definition.Label == "" {
			definition.Label = "Other"
		}
		if definition.Language == "" {
			definition.Language = "plaintext"
		}
		if definition.DefaultMode == "" {
			definition.DefaultMode = "source"
		}
		return CodeHandler{Definition: definition}
	}
	formattable := FormattedCodeHandler{CodeHandler: CodeHandler{Definition: Type{ID: "fmt", Label: "Formatted", Extensions: []string{".fmt"}, Language: "plaintext", PreviewKind: PreviewMarkdown, PreviewLabel: "Preview", DefaultMode: "preview"}}, FormatSource: formatJSON}
	tests := map[string][]Handler{
		"missing fallback":                {nil},
		"missing handler":                 {plainText(), nil},
		"duplicate family":                {plainText(), NewJSON(), NewJSON()},
		"ambiguous id":                    {plainText(), newMarkdown(), source(Type{ID: "md", Extensions: []string{".other"}})},
		"duplicate extension":             {plainText(), NewJSON(), source(Type{ID: "other", Extensions: []string{".json"}})},
		"uppercase extension":             {plainText(), source(Type{ID: "other", Extensions: []string{".OTHER"}})},
		"multi-dot extension":             {plainText(), source(Type{ID: "other", Extensions: []string{".d.ts"}})},
		"no selectors":                    {plainText(), source(Type{ID: "other"})},
		"duplicate filename":              {plainText(), source(Type{ID: "a", Filenames: []string{"Makefile"}}), source(Type{ID: "b", Filenames: []string{"makefile"}})},
		"filename clashes with id":        {plainText(), source(Type{ID: "make", Extensions: []string{".mk"}}), source(Type{ID: "b", Filenames: []string{"Make"}})},
		"filename clashes with extension": {plainText(), source(Type{ID: "dotenv", Extensions: []string{".env"}}), source(Type{ID: "b", Filenames: []string{".env"}})},
		"filename with path":              {plainText(), source(Type{ID: "other", Filenames: []string{"docker/Dockerfile"}})},
		"source-only preview":             {plainText(), source(Type{ID: "other", Extensions: []string{".other"}, DefaultMode: "preview"})},
		"unknown preview kind":            {plainText(), source(Type{ID: "other", Extensions: []string{".other"}, PreviewKind: "code", PreviewLabel: "Formatted"})},
		"missing preview label":           {plainText(), source(Type{ID: "other", Extensions: []string{".other"}, PreviewKind: PreviewImage})},
		"unknown language":                {plainText(), source(Type{ID: "other", Extensions: []string{".other"}, Language: "not-a-real-language"})},
		"missing text language":           {plainText(), CodeHandler{Definition: Type{ID: "other", Label: "Other", Extensions: []string{".other"}, DefaultMode: "source"}}},
		"declared formattable":            {plainText(), source(Type{ID: "other", Extensions: []string{".other"}, Formattable: true})},
		"format with preview":             {plainText(), formattable},
		"previewing fallback":             {newMarkdown()},
		"formattable fallback":            {NewJSON()},
	}
	for name, handlers := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRegistry(handlers[0], handlers[1:]...); err == nil {
				t.Fatal("invalid composition accepted")
			}
		})
	}
	image := CodeHandler{Definition: Type{ID: "image", Label: "Image", Extensions: []string{".png"}, PreviewKind: PreviewImage, PreviewLabel: "Preview", DefaultMode: "preview"}}
	if _, err := NewRegistry(plainText(), image); err != nil {
		t.Fatalf("image type without language rejected: %v", err)
	}
}

func TestFormattingIsAnOnDemandCapability(t *testing.T) {
	registry := Builtins()
	for _, name := range []string{"config.json", "config.yaml"} {
		family := registry.Lookup(name)
		preview, err := family.Preview([]byte(`{"a":1}`), Context{})
		if err != nil || preview != "" {
			t.Fatalf("%s eagerly previewed: %q, %v", name, preview, err)
		}
	}
	if _, err := registry.Lookup("main.go").FormatLines([]byte("package main")); !errors.Is(err, ErrNotFormattable) {
		t.Fatalf("source-only format error = %v", err)
	}
}

func TestJSONFormatPreservesValuesAndOriginalSource(t *testing.T) {
	source := []byte(`{"z":900719925474099312345,"small":1e-400,"z":2,"html":"<script>alert(1)</script>"}`)
	original := string(source)
	lines, err := Builtins().Lookup("data.json").FormatLines(source)
	if err != nil {
		t.Fatal(err)
	}
	rendered := strings.Join(lines, "\n")
	plain := plainHTML(rendered)
	for _, fragment := range []string{"\n  \"z\": 900719925474099312345", `"small": 1e-400`, `"z": 2`, `<script>alert(1)</script>`} {
		if !strings.Contains(plain, fragment) {
			t.Errorf("format lost %q: %s", fragment, plain)
		}
	}
	if strings.Contains(rendered, "<script>") || !strings.Contains(rendered, "syntax-") {
		t.Fatalf("unsafe/unhighlighted format: %s", rendered)
	}
	if string(source) != original {
		t.Fatal("formatting mutated source")
	}
}

func TestYAMLFormatRetainsPresentationAndMultipleDocuments(t *testing.T) {
	source := []byte("# configuration\ndefaults: &defaults\n    name: 'yes' # keep string\n    script: |\n      echo hello\ncopy: *defaults\n---\nsecond: 900719925474099312345\n")
	original := string(source)
	lines, err := Builtins().Lookup("data.yml").FormatLines(source)
	if err != nil {
		t.Fatal(err)
	}
	plain := plainHTML(strings.Join(lines, "\n"))
	for _, fragment := range []string{"# configuration", "&defaults", "*defaults", "'yes'", "# keep string", "script: |", "echo hello", "---", "second: 900719925474099312345", "\n  name:"} {
		if !strings.Contains(plain, fragment) {
			t.Errorf("format lost %q: %s", fragment, plain)
		}
	}
	if string(source) != original {
		t.Fatal("formatting mutated source")
	}
}

func TestMalformedFilesRemainSearchable(t *testing.T) {
	registry := Builtins()
	for name, source := range map[string]string{
		"broken.json": `{"missing": searchable-json`,
		"broken.yaml": "first: searchable-yaml\nsecond: [",
	} {
		family := registry.Lookup(name)
		if _, err := family.FormatLines([]byte(source)); err == nil {
			t.Fatal("malformed format succeeded")
		}
		text, err := family.SearchText([]byte(source))
		if err != nil || text != source {
			t.Fatalf("search = %q, %v", text, err)
		}
	}
	for _, source := range []string{"", "# comment only\n"} {
		lines, err := registry.Lookup("empty.yaml").FormatLines([]byte(source))
		if err != nil || strings.TrimSpace(plainHTML(strings.Join(lines, "\n"))) != strings.TrimSpace(source) {
			t.Fatalf("empty/comment YAML = %q, %v", lines, err)
		}
	}
}

func TestHighlightLinesRetainTextAndMultilineState(t *testing.T) {
	for _, source := range []string{"", "\n", "a\r\nb\rc\n", "# Heading\n\nbody", "key: |\n  first\n  second\n"} {
		normalized := strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\r", "\n")
		want := strings.Split(normalized, "\n")
		if len(want) > 1 && want[len(want)-1] == "" {
			want = want[:len(want)-1]
		}
		for _, language := range []string{"markdown", "yaml", "plaintext", "unknown-language"} {
			lines := HighlightLines([]byte(source), language)
			plain := make([]string, len(lines))
			for i, line := range lines {
				plain[i] = plainHTML(line)
			}
			if !reflect.DeepEqual(plain, want) {
				t.Errorf("%s source %q: got %#v, want %#v", language, source, plain, want)
			}
		}
		if plain := PlainLines([]byte(source)); len(plain) != len(want) {
			t.Errorf("plain lines %q = %#v", source, plain)
		}
	}
	lines := HighlightLines([]byte("key: |\n  <script>\n  second\n"), "yaml")
	if len(lines) != 3 || !strings.Contains(lines[1], "syntax-") || strings.Contains(lines[1], "<script>") {
		t.Fatalf("multiline highlight = %#v", lines)
	}
	if plain := PlainLines([]byte("<b>&\n")); len(plain) != 1 || plain[0] != "&lt;b&gt;&amp;" {
		t.Fatalf("plain lines = %#v", plain)
	}
}

type panickingLexer struct{ config *chroma.Config }

func (l panickingLexer) Config() *chroma.Config { return l.config }
func (panickingLexer) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	panic("lexer bug")
}
func (l panickingLexer) SetRegistry(*chroma.LexerRegistry) chroma.Lexer { return l }
func (l panickingLexer) SetAnalyser(func(string) float32) chroma.Lexer  { return l }
func (panickingLexer) AnalyseText(string) float32                       { return 0 }

func TestHighlightLinesRecoversFromLexerPanics(t *testing.T) {
	lexers.Register(panickingLexer{config: &chroma.Config{Name: "servef-panic-test"}})
	if lexers.Get("servef-panic-test") == nil {
		t.Fatal("test lexer not registered")
	}
	lines := HighlightLines([]byte("<a>\nb\n"), "servef-panic-test")
	if !reflect.DeepEqual(lines, []string{"&lt;a&gt;", "b"}) {
		t.Fatalf("panic fallback = %#v", lines)
	}
}

func TestBinarySniffing(t *testing.T) {
	if IsBinary([]byte("plain text\n")) || IsBinary(nil) {
		t.Fatal("text detected as binary")
	}
	if !IsBinary([]byte("PNG\x00data")) {
		t.Fatal("NUL byte not detected")
	}
	late := append([]byte(strings.Repeat("a", SniffLength)), 0)
	if IsBinary(late) {
		t.Fatal("NUL beyond sniff window detected")
	}
}

func TestSourceLanguageCanBeComposedWithoutPreview(t *testing.T) {
	handler := CodeHandler{Definition: Type{ID: "zig", Label: "Zig", Extensions: []string{".zig"}, Language: "zig", DefaultMode: "source"}}
	registry, err := NewRegistry(plainText(), handler)
	if err != nil {
		t.Fatal(err)
	}
	resolved := registry.Lookup("MAIN.ZIG")
	if resolved.Type().ID != "zig" {
		t.Fatal("Zig handler was not composed")
	}
	preview, err := resolved.Preview([]byte("const std = @import(\"std\");\n"), Context{})
	if err != nil || preview != "" || resolved.Type().PreviewKind != "" || resolved.Type().Formattable {
		t.Fatalf("source-only contract = %q, %v", preview, err)
	}
	if !strings.Contains(strings.Join(HighlightLines([]byte("const std = @import(\"std\");\n"), resolved.Type().Language), ""), "syntax-") {
		t.Fatal("source-only handler has no highlighting")
	}
	if registry.Lookup("main.go").Type().ID != "text" {
		t.Fatal("unregistered file did not use fallback")
	}
}

func TestMarkdownLinksTargetInScopeFiles(t *testing.T) {
	handler := Builtins().Lookup("docs/readme.md")
	source := []byte("[config](../config.json) [code](main.go) [dir](guides/) ![image](image.png)")
	for _, entry := range []struct {
		name     string
		supports func(string) bool
		code     bool
	}{
		{name: "all files", supports: func(string) bool { return true }, code: true},
		{name: "markdown only", supports: func(name string) bool { return strings.HasSuffix(name, ".md") }},
	} {
		t.Run(entry.name, func(t *testing.T) {
			rendered, err := handler.Preview(source, Context{Path: "docs/readme.md", Supports: entry.supports})
			if err != nil {
				t.Fatal(err)
			}
			if entry.code != strings.Contains(rendered, `href="/view?path=docs%2Fmain.go"`) || entry.code != strings.Contains(rendered, `href="/view?path=config.json"`) {
				t.Fatalf("link policy = %s", rendered)
			}
			if !strings.Contains(rendered, `href="guides/"`) {
				t.Fatalf("directory link rewritten: %s", rendered)
			}
			if !strings.Contains(rendered, `/api/image?path=docs%2Fimage.png`) {
				t.Fatalf("image lost: %s", rendered)
			}
		})
	}
}

func TestFormatterCannotMutateOriginalBytes(t *testing.T) {
	source := []byte("original")
	handler := FormattedCodeHandler{
		CodeHandler:  CodeHandler{Definition: Type{ID: "mutating", Label: "Mutating", Extensions: []string{".mut"}, Language: "plaintext", DefaultMode: "source"}},
		FormatSource: func(value []byte) ([]byte, error) { value[0] = 'X'; return value, nil },
	}
	registry, err := NewRegistry(plainText(), handler)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Lookup("a.mut").FormatLines(source); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Format(source); err != nil {
		t.Fatal(err)
	}
	if string(source) != "original" {
		t.Fatal("formatter changed original bytes")
	}
}
