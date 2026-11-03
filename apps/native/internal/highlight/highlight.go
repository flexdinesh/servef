// Package highlight tokenizes source text with Chroma and maps its tokens to
// servef's eight syntax categories (decision 0029: comment, keyword, type,
// function, variable, string, number, operator), so native colors code
// from the same theme tokens as the web UI. It knows no UI. It is pure.
//
// Chroma is not Highlights: token boundaries and some categories differ
// from the web UI (decision 0082, known gaps).
package highlight

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Category is a servef syntax category; Plain uses the text color.
type Category uint8

const (
	Plain Category = iota
	Comment
	Keyword
	Type
	Function
	Variable
	String
	Number
	Operator
)

// Categories lists every category but Plain, in theme order.
var Categories = []Category{Comment, Keyword, Type, Function, Variable, String, Number, Operator}

// Name is the category's servef token name ("comment" for
// --color-syntax-comment); "" for Plain.
func (c Category) Name() string {
	switch c {
	case Comment:
		return "comment"
	case Keyword:
		return "keyword"
	case Type:
		return "type"
	case Function:
		return "function"
	case Variable:
		return "variable"
	case String:
		return "string"
	case Number:
		return "number"
	case Operator:
		return "operator"
	}
	return ""
}

// Span is a run of a line in one category, from the previous span's End (or
// the line's start) to End, a byte offset in the line.
type Span struct {
	End      int
	Category Category
}

// lexerNames maps every declared servef language (testdata/
// document-families.json) to its Chroma lexer. A language not listed is
// shown plain: native never guesses a language.
var lexerNames = map[string]string{
	"bash": "Bash", "c": "C", "cpp": "C++", "css": "CSS", "diff": "Diff",
	"dockerfile": "Docker", "go": "Go", "html": "HTML", "java": "Java",
	"js": "JavaScript", "json": "JSON", "jsx": "react", "kotlin": "Kotlin",
	"lua": "Lua", "makefile": "Makefile", "markdown": "markdown", "php": "PHP",
	"powershell": "PowerShell", "python": "Python", "ruby": "Ruby",
	"rust": "Rust", "scss": "SCSS", "sql": "SQL", "swift": "Swift",
	"terraform": "Terraform", "toml": "TOML", "ts": "TypeScript",
	"tsx": "TypeScript", "xml": "XML", "yaml": "YAML",
}

// Supported reports whether language highlights; "text" and "" do not.
func Supported(language string) bool {
	_, ok := lexerNames[language]
	return ok
}

func lexer(language string) chroma.Lexer {
	name, ok := lexerNames[language]
	if !ok {
		return nil
	}
	return lexers.Get(name)
}

// Lines tokenizes text in language and returns the spans of each line of
// text split on "\n" (a trailing "\r" stays in its line). Unknown
// languages, and tokenizer failures or panics, give plain lines.
func Lines(text, language string) (lines [][]Span) {
	defer func() {
		if recover() != nil {
			lines = plain(text)
		}
	}()
	l := lexer(language)
	if l == nil {
		return plain(text)
	}
	iterator, err := chroma.Coalesce(l).Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, text)
	if err != nil {
		return plain(text)
	}
	lines = make([][]Span, 1, strings.Count(text, "\n")+1)
	offset := 0
	for token := iterator(); token != chroma.EOF; token = iterator() {
		category := Classify(token.Type)
		value := token.Value
		for {
			newline := strings.IndexByte(value, '\n')
			if newline < 0 {
				break
			}
			offset = appendSpan(lines, offset+newline, category, offset)
			lines = append(lines, nil)
			offset = 0
			value = value[newline+1:]
		}
		offset = appendSpan(lines, offset+len(value), category, offset)
	}
	// Chroma may add a final newline; keep exactly the lines of text.
	if want := strings.Count(text, "\n") + 1; len(lines) > want {
		lines = lines[:want]
	} else {
		for len(lines) < want {
			lines = append(lines, nil)
		}
	}
	return lines
}

// appendSpan extends the last line with a span ending at end, merging it
// into the previous span of the same category; empty spans are dropped.
func appendSpan(lines [][]Span, end int, category Category, start int) int {
	if end <= start {
		return end
	}
	last := len(lines) - 1
	line := lines[last]
	if n := len(line); n > 0 && line[n-1].Category == category {
		line[n-1].End = end
	} else {
		line = append(line, Span{End: end, Category: category})
	}
	lines[last] = line
	return end
}

func plain(text string) [][]Span {
	parts := strings.Split(text, "\n")
	lines := make([][]Span, len(parts))
	for i, part := range parts {
		if part != "" {
			lines[i] = []Span{{End: len(part), Category: Plain}}
		}
	}
	return lines
}

// Classify maps a Chroma token type to a servef category, following
// servef's Highlights theme (apps/web/src/highlighting/syntax-theme.ts):
// preprocessor and boolean as keyword, namespaces and tags as type,
// builtins as function, properties and attributes as variable, constants
// as number.
func Classify(t chroma.TokenType) Category {
	switch t {
	case chroma.CommentPreproc, chroma.CommentPreprocFile:
		return Keyword
	case chroma.KeywordType, chroma.NameClass, chroma.NameNamespace, chroma.NameTag, chroma.NameException:
		return Type
	case chroma.NameFunction, chroma.NameFunctionMagic, chroma.NameBuiltin, chroma.NameBuiltinPseudo, chroma.NameDecorator:
		return Function
	case chroma.NameConstant, chroma.NameEntity, chroma.LiteralDate:
		return Number
	case chroma.NameAttribute, chroma.NameProperty, chroma.NameLabel, chroma.NameVariable,
		chroma.NameVariableClass, chroma.NameVariableGlobal, chroma.NameVariableInstance, chroma.NameVariableMagic:
		return Variable
	case chroma.OperatorWord:
		return Keyword
	}
	switch {
	case t.InCategory(chroma.Comment):
		return Comment
	case t.InCategory(chroma.Keyword):
		return Keyword
	case t.InSubCategory(chroma.LiteralString):
		return String
	case t.InSubCategory(chroma.LiteralNumber):
		return Number
	case t.InCategory(chroma.Operator):
		return Operator
	}
	return Plain
}
