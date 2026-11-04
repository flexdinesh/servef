// Package highlight tokenizes source text with Highlights (decision 0029,
// through packages/mygo/highlights: the web app's WebAssembly lexers under
// wazero) and maps its token scopes to servef's syntax categories as the
// web app's theme does (apps/web/src/highlighting/syntax-theme.ts and
// syntax-highlight.ts), so native colors code from the same theme tokens
// and the same token boundaries as the web UI. It knows no UI. It is pure.
package highlight

import (
	"strings"

	"github.com/flexdinesh/servef/packages/mygo/highlights"
)

// Category is a servef syntax role (an hl-<role> class in the web app);
// Plain uses the text color.
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
	// Inserted, Deleted, and Link take the success, destructive, and
	// accent colors (hl-inserted, hl-deleted, hl-link).
	Inserted
	Deleted
	Link
	// Emphasis is plain text in italics, Strong plain text in bold
	// (hl-italic, hl-bold without a color).
	Emphasis
	Strong
)

// Categories lists the categories with a --color-syntax-* token, in theme
// order.
var Categories = []Category{Comment, Keyword, Type, Function, Variable, String, Number, Operator}

// Name is the category's role ("comment" for hl-comment and
// --color-syntax-comment); "" for Plain, Emphasis, and Strong.
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
	case Inserted:
		return "inserted"
	case Deleted:
		return "deleted"
	case Link:
		return "link"
	}
	return ""
}

// Italic reports the categories servef shows in italics.
func (c Category) Italic() bool { return c == Comment || c == Emphasis }

// Bold reports the categories servef shows in bold.
func (c Category) Bold() bool { return c == Strong }

// Span is a run of a line in one category, from the previous span's End (or
// the line's start) to End, a byte offset in the line.
type Span struct {
	End      int
	Category Category
}

// MaxLineLength is syntax-highlight.ts's TOKENIZE_MAX_LINE_LENGTH: lines
// this long (UTF-16 code units) or longer show plain.
const MaxLineLength = 1000

// plainLanguages mean no highlighting (syntax-highlight.ts).
var plainLanguages = map[string]bool{"": true, "text": true, "plain": true, "plaintext": true, "txt": true}

// Supported reports whether language highlights: Highlights knows it and
// it is not a plain name.
func Supported(language string) bool {
	return !plainLanguages[strings.ToLower(language)] && highlights.Supported(language)
}

// Warm compiles Highlights ahead of the first Lines call.
func Warm() error { return highlights.Warm() }

// syntaxTheme is SYNTAX_THEME's style.syntax (syntax-theme.ts): the
// --color-syntax-<role> each scope takes, inherited from its nearest
// parent scope; scopes without one use the foreground.
var syntaxTheme = map[string]highlights.Setting{
	"comment": {Color: "comment", FontStyle: "italic"}, "keyword": {Color: "keyword"}, "preproc": {Color: "keyword"},
	"boolean": {Color: "keyword"}, "operator": {Color: "operator"}, "keyword.operator": {Color: "operator"},
	"type": {Color: "type"}, "constructor": {Color: "type"}, "namespace": {Color: "type"}, "tag": {Color: "type"},
	"selector": {Color: "type"}, "property.json_key": {Color: "type"}, "function": {Color: "function"},
	"constant.builtin": {Color: "function"}, "variable.special": {Color: "function"}, "variable": {Color: "variable"},
	"variable.parameter": {Color: "variable"}, "property": {Color: "variable"}, "attribute": {Color: "variable"},
	"label": {Color: "variable"}, "string": {Color: "string"}, "text.literal": {Color: "string"},
	"number": {Color: "number"}, "constant": {Color: "number"}, "enum": {Color: "number"},
	"diff.plus": {Color: "inserted"}, "diff.minus": {Color: "deleted"}, "link_text": {Color: "link"}, "link_uri": {Color: "link"},
	"emphasis": {FontStyle: "italic"}, "emphasis.strong": {FontWeight: 700}, "title": {FontWeight: 700},
}

// roles are the colors syntax-highlight.ts renders as hl-<role> classes.
var roles = map[string]Category{
	"comment": Comment, "keyword": Keyword, "type": Type, "function": Function, "variable": Variable,
	"string": String, "number": Number, "operator": Operator, "inserted": Inserted, "deleted": Deleted, "link": Link,
}

// scopeCategories maps every Highlights scope to its category, resolved
// once from syntaxTheme as prepareTheme resolves a CSS-palette theme.
var scopeCategories = func() []Category {
	names := highlights.Scopes()
	out := make([]Category, len(names))
	for i, name := range names {
		if i == 0 {
			continue // "none": the foreground
		}
		out[i] = categoryOf(syntaxTheme, name)
	}
	return out
}()

// categoryOf is the category a scope's resolved setting renders as: its
// role, or Emphasis or Strong for plain italic or bold text.
func categoryOf(theme map[string]highlights.Setting, scope string) Category {
	setting, _ := highlights.Resolve(theme, scope)
	if c, ok := roles[setting.Color]; ok {
		return c
	}
	switch {
	case setting.FontStyle == "italic":
		return Emphasis
	case setting.FontWeight >= 600:
		return Strong
	}
	return Plain
}

// Lines tokenizes text in language and returns the spans of each line of
// text split on "\n" (a trailing "\r" stays in its line, unstyled), as
// highlightLines does: line breaks normalized, lines of MaxLineLength or
// more plain, and a line whose tokens do not spell it plain. Unknown
// languages, text with a lone "\r", and tokenizer failures give plain
// lines.
func Lines(text, language string) [][]Span {
	if !Supported(language) || strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\r") {
		return plain(text)
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	tokens, err := highlights.Tokens(language, normalized, highlights.Options{MaxLineLength: MaxLineLength})
	if err != nil {
		return plain(text)
	}
	sources := strings.Split(normalized, "\n")
	if len(tokens) != len(sources) {
		return plain(text)
	}
	lines := make([][]Span, len(sources))
	for i, line := range tokens {
		lines[i] = spans(line, len(sources[i]))
	}
	return lines
}

// spans converts one line's tokens to spans, merging neighbors of a
// category; nil (plain) when the tokens do not cover the line exactly.
func spans(line highlights.Line, width int) []Span {
	var out []Span
	at := line.Start
	for _, token := range line.Tokens {
		if token.Start != at {
			return plainLine(width)
		}
		at = token.End
		end := token.End - line.Start
		category := Plain
		if int(token.Scope) < len(scopeCategories) {
			category = scopeCategories[token.Scope]
		}
		if n := len(out); n > 0 && out[n-1].Category == category {
			out[n-1].End = end
			continue
		}
		out = append(out, Span{End: end, Category: category})
	}
	if at-line.Start != width {
		return plainLine(width)
	}
	return out
}

func plainLine(width int) []Span {
	if width == 0 {
		return nil
	}
	return []Span{{End: width, Category: Plain}}
}

func plain(text string) [][]Span {
	parts := strings.Split(text, "\n")
	lines := make([][]Span, len(parts))
	for i, part := range parts {
		lines[i] = plainLine(len(part))
	}
	return lines
}
