package highlight

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// categoryAt returns the category covering byte offset in line.
func categoryAt(line []Span, offset int) Category {
	for _, span := range line {
		if offset < span.End {
			return span.Category
		}
	}
	return Plain
}

func TestLinesGo(t *testing.T) {
	text := "package main\n\n// Greet says hi.\nfunc Greet(n int) string {\n\treturn \"hi\" + 42\n}"
	lines := Lines(text, "go")
	if len(lines) != 6 {
		t.Fatalf("got %d lines, want 6", len(lines))
	}
	for _, tc := range []struct {
		line   int
		needle string
		want   Category
	}{
		{0, "package", Keyword},
		{2, "// Greet", Comment},
		{3, "func", Keyword},
		{3, "Greet", Function},
		{3, "int", Type},
		{4, "\"hi\"", String},
		{4, "42", Number},
		{4, "+", Operator},
	} {
		source := strings.Split(text, "\n")[tc.line]
		offset := strings.Index(source, tc.needle)
		if got := categoryAt(lines[tc.line], offset); got != tc.want {
			t.Errorf("line %d %q: %v, want %v (spans %v)", tc.line, tc.needle, got, tc.want, lines[tc.line])
		}
	}
	if lines[1] != nil {
		t.Errorf("empty line has spans %v", lines[1])
	}
	// Spans end within their line and grow.
	for i, line := range lines {
		width := len(strings.Split(text, "\n")[i])
		last := 0
		for _, span := range line {
			if span.End <= last || span.End > width {
				t.Fatalf("line %d spans %v exceed %d", i, line, width)
			}
			last = span.End
		}
	}
}

func TestLinesPlain(t *testing.T) {
	lines := Lines("a\nbb\n", "text")
	want := [][]Span{{{End: 1}}, {{End: 2}}, nil}
	if len(lines) != len(want) {
		t.Fatalf("lines %v", lines)
	}
	for i := range want {
		if len(lines[i]) != len(want[i]) || (len(want[i]) > 0 && lines[i][0] != want[i][0]) {
			t.Fatalf("line %d: %v, want %v", i, lines[i], want[i])
		}
	}
	if got := Lines("x := 1", "klingon"); len(got) != 1 || got[0][0].Category != Plain {
		t.Fatalf("unknown language is plain: %v", got)
	}
}

func TestLinesKeepLineCount(t *testing.T) {
	for _, text := range []string{"", "\n", "a\r\nb\r\n", "x = 1\n\n\n"} {
		if got, want := len(Lines(text, "python")), strings.Count(text, "\n")+1; got != want {
			t.Errorf("%q: %d lines, want %d", text, got, want)
		}
	}
}

func TestClassify(t *testing.T) {
	for typ, want := range map[chroma.TokenType]Category{
		chroma.CommentSingle: Comment, chroma.CommentPreproc: Keyword, chroma.KeywordConstant: Keyword,
		chroma.KeywordType: Type, chroma.NameTag: Type, chroma.NameBuiltin: Function,
		chroma.NameAttribute: Variable, chroma.LiteralStringDouble: String, chroma.LiteralStringEscape: String,
		chroma.LiteralNumberFloat: Number, chroma.NameConstant: Number, chroma.Operator: Operator,
		chroma.Punctuation: Plain, chroma.Text: Plain, chroma.Name: Plain,
	} {
		if got := Classify(typ); got != want {
			t.Errorf("Classify(%v) = %v, want %v", typ, got, want)
		}
	}
}

// Every language servef declares either highlights or is "text"; every
// mapped lexer exists in Chroma.
func TestEveryDeclaredLanguage(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "testdata", "document-families.json"))
	if err != nil {
		t.Fatal(err)
	}
	var families []struct {
		ID       string `json:"id"`
		Language string `json:"language"`
	}
	if err := json.Unmarshal(raw, &families); err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.Language == "" || family.Language == "text" {
			continue
		}
		if !Supported(family.Language) {
			t.Errorf("family %s declares %q, which native does not highlight", family.ID, family.Language)
		}
	}
	for language, name := range lexerNames {
		if lexers.Get(name) == nil {
			t.Errorf("%s maps to missing Chroma lexer %q", language, name)
		}
	}
}

func TestCategoryNames(t *testing.T) {
	var names []string
	for _, c := range Categories {
		names = append(names, c.Name())
	}
	if got := strings.Join(names, ","); got != "comment,keyword,type,function,variable,string,number,operator" {
		t.Fatalf("names %s", got)
	}
	if Plain.Name() != "" {
		t.Fatal("plain has no token")
	}
}
