package document

import (
	"html"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var htmlTags = regexp.MustCompile(`<[^>]*>`)

func plainHTML(value string) string { return html.UnescapeString(htmlTags.ReplaceAllString(value, "")) }

func TestRegistryExplicitFamiliesAndAliases(t *testing.T) {
	registry := Builtins()
	for _, name := range []string{"README.MD", "api.markdown", "config.JSON", "compose.YML", "config.yaml"} {
		if !registry.Supports(name) {
			t.Errorf("unsupported %s", name)
		}
	}
	for _, name := range []string{"main.go", "app.ts", "notes.txt", "config.jsonc", "README"} {
		if registry.Supports(name) {
			t.Errorf("unexpected support for %s", name)
		}
	}
	for _, selector := range []string{"md", ".MD", "markdown"} {
		selected, err := registry.Select([]string{selector})
		if err != nil {
			t.Fatal(err)
		}
		if !selected.Supports("readme.markdown") || selected.Supports("config.json") || len(selected.Types()) != 1 {
			t.Fatalf("selection %s = %+v", selector, selected.Types())
		}
	}
	selected, err := registry.Select([]string{"YML", "json", "yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Types()) != 2 || !selected.Supports("config.yaml") || !selected.Supports("config.yml") || selected.Supports("readme.md") {
		t.Fatalf("selection = %+v", selected.Types())
	}
	if _, err := registry.Select([]string{"go"}); err == nil {
		t.Fatal("unknown selector accepted")
	}
}

func TestRegistryRejectsAmbiguousSupport(t *testing.T) {
	if _, err := NewRegistry(nil); err == nil {
		t.Fatal("missing handler accepted")
	}
	ambiguous := CodeHandler{Definition: Type{ID: "md", Label: "Other", Extensions: []string{".other"}, DefaultMode: "source"}}
	if _, err := NewRegistry(newMarkdown(), ambiguous); err == nil {
		t.Fatal("ambiguous selector accepted")
	}
	json := NewJSON()
	if _, err := NewRegistry(json, json); err == nil {
		t.Fatal("duplicate family accepted")
	}
	conflicting := CodeHandler{Definition: Type{ID: "other", Label: "Other", Extensions: []string{".json"}, DefaultMode: "source"}}
	if _, err := NewRegistry(json, conflicting); err == nil {
		t.Fatal("ambiguous extension accepted")
	}
	conflicting.Definition.Extensions = []string{".other"}
	conflicting.Definition.DefaultMode = "preview"
	if _, err := NewRegistry(conflicting); err == nil {
		t.Fatal("source-only handler defaulted to preview")
	}
}

func TestJSONPreviewPreservesValuesAndOriginalSource(t *testing.T) {
	source := []byte(`{"z":900719925474099312345,"small":1e-400,"z":2,"html":"<script>alert(1)</script>"}`)
	original := string(source)
	handler := NewJSON()
	rendered, err := handler.Preview(source, Context{})
	if err != nil {
		t.Fatal(err)
	}
	plain := plainHTML(rendered)
	for _, fragment := range []string{"\n  \"z\": 900719925474099312345", `"small": 1e-400`, `"z": 2`, `<script>alert(1)</script>`} {
		if !strings.Contains(plain, fragment) {
			t.Errorf("preview lost %q: %s", fragment, plain)
		}
	}
	if strings.Contains(rendered, "<script>") || !strings.Contains(rendered, "syntax-") {
		t.Fatalf("unsafe/unhighlighted preview: %s", rendered)
	}
	if string(source) != original {
		t.Fatal("formatting mutated source")
	}
}

func TestYAMLPreviewRetainsPresentationAndMultipleDocuments(t *testing.T) {
	source := []byte("# configuration\ndefaults: &defaults\n    name: 'yes' # keep string\n    script: |\n      echo hello\ncopy: *defaults\n---\nsecond: 900719925474099312345\n")
	original := string(source)
	rendered, err := NewYAML().Preview(source, Context{})
	if err != nil {
		t.Fatal(err)
	}
	plain := plainHTML(rendered)
	for _, fragment := range []string{"# configuration", "&defaults", "*defaults", "'yes'", "# keep string", "script: |", "echo hello", "---", "second: 900719925474099312345", "\n  name:"} {
		if !strings.Contains(plain, fragment) {
			t.Errorf("preview lost %q: %s", fragment, plain)
		}
	}
	if string(source) != original {
		t.Fatal("formatting mutated source")
	}
}

func TestMalformedFilesRemainSearchable(t *testing.T) {
	for _, entry := range []struct {
		handler Handler
		source  string
	}{
		{NewJSON(), `{"missing": searchable-json`},
		{NewYAML(), "first: searchable-yaml\nsecond: ["},
	} {
		if _, err := entry.handler.Preview([]byte(entry.source), Context{}); err == nil {
			t.Fatal("malformed preview succeeded")
		}
		text, err := entry.handler.SearchText([]byte(entry.source))
		if err != nil || text != entry.source {
			t.Fatalf("search = %q, %v", text, err)
		}
	}
	for _, source := range []string{"", "# comment only\n"} {
		rendered, err := NewYAML().Preview([]byte(source), Context{})
		if err != nil || strings.TrimSpace(plainHTML(rendered)) != strings.TrimSpace(source) {
			t.Fatalf("empty/comment YAML = %q, %v", rendered, err)
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
		for _, language := range []string{"markdown", "yaml", "unknown-language"} {
			lines := HighlightLines([]byte(source), language)
			plain := make([]string, len(lines))
			for i, line := range lines {
				plain[i] = plainHTML(line)
			}
			if !reflect.DeepEqual(plain, want) {
				t.Errorf("%s source %q: got %#v, want %#v", language, source, plain, want)
			}
		}
	}
	lines := HighlightLines([]byte("key: |\n  <script>\n  second\n"), "yaml")
	if len(lines) != 3 || !strings.Contains(lines[1], "syntax-") || strings.Contains(lines[1], "<script>") {
		t.Fatalf("multiline highlight = %#v", lines)
	}
}

func TestSourceLanguageCanBeComposedWithoutPreview(t *testing.T) {
	handler := CodeHandler{Definition: Type{ID: "go", Label: "Go", Extensions: []string{".go"}, Language: "go", DefaultMode: "source"}}
	registry, err := NewRegistry(handler)
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := registry.Lookup("MAIN.GO")
	if !ok {
		t.Fatal("Go handler was not composed")
	}
	preview, err := resolved.Preview([]byte("package main\n"), Context{})
	if err != nil || preview != "" || resolved.Type().PreviewKind != "" {
		t.Fatalf("source-only contract = %q, %v", preview, err)
	}
	if !strings.Contains(strings.Join(HighlightLines([]byte("package main\n"), resolved.Type().Language), ""), "syntax-") {
		t.Fatal("source-only handler has no highlighting")
	}
}

func TestMarkdownLinksRespectSelectedScope(t *testing.T) {
	for _, selectors := range [][]string{nil, {"md"}} {
		registry, err := Builtins().Select(selectors)
		if err != nil {
			t.Fatal(err)
		}
		handler, _ := registry.Lookup("docs/readme.md")
		rendered, err := handler.Preview([]byte("[config](../config.json) ![image](image.png)"), Context{Path: "docs/readme.md", Supports: registry.Supports})
		if err != nil {
			t.Fatal(err)
		}
		if (len(selectors) == 0) != strings.Contains(rendered, `href="/view?path=config.json"`) {
			t.Fatalf("link policy = %s", rendered)
		}
		if !strings.Contains(rendered, `/api/image?path=docs%2Fimage.png`) {
			t.Fatalf("image lost: %s", rendered)
		}
	}
}

func TestFormatterCannotMutateOriginalBytes(t *testing.T) {
	source := []byte("original")
	handler := CodeHandler{Definition: Type{Language: "text"}, Format: func(value []byte) ([]byte, error) { value[0] = 'X'; return value, nil }}
	if _, err := handler.Preview(source, Context{}); err != nil {
		t.Fatal(err)
	}
	if string(source) != "original" {
		t.Fatal("formatter changed original bytes")
	}
}
