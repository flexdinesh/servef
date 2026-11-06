package document

import (
	"errors"
	"io"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

func NewCSS() Handler {
	return FormattedCodeHandler{
		CodeHandler:  CodeHandler{Definition: Type{ID: "css", Label: "CSS", Extensions: []string{".css"}, Language: "css", DefaultMode: "source"}},
		FormatSource: formatCSS,
	}
}

type cssToken struct {
	tt       css.TokenType
	text     string
	space    bool // whitespace preceded the token
	newlines int  // line breaks in that whitespace
}

// formatCSS reindents rules and puts each declaration on its own line. Tokens,
// including comments, strings, and values, are written unchanged; whitespace
// runs collapse to one space or line break.
func formatCSS(source []byte) ([]byte, error) {
	tokens, err := lexCSS(source)
	if err != nil {
		return nil, err
	}
	w := newTextLayout()
	depth, parens := 0, 0
	declaration := false // a declaration colon was seen in this statement
	for i, token := range tokens {
		if token.newlines >= 2 && parens == 0 {
			w.Break(2)
		} else if token.space {
			w.Space()
		}
		switch token.tt {
		case css.CommentToken:
			if token.newlines == 0 && w.breaks > 0 {
				w.WriteTrailing(token.text)
			} else {
				w.Write(token.text)
			}
		case css.LeftBraceToken:
			w.Space()
			w.Write(token.text)
			depth++
			w.indent = depth
			w.Break(1)
			declaration = false
		case css.RightBraceToken:
			depth = max(depth-1, 0)
			w.indent = depth
			if w.last == "{" {
				w.breaks = 0
			} else {
				w.Break(1)
			}
			w.Write(token.text)
			if depth == 0 {
				w.Break(2)
			} else {
				w.Break(1)
			}
			declaration = false
		case css.SemicolonToken:
			w.Write(token.text)
			if parens == 0 {
				w.Break(1)
				declaration = false
			}
		case css.ColonToken:
			w.Write(token.text)
			// Space only declaration colons; selector colons (a:hover) are
			// significant. A declaration ends at ; or } before any {.
			if depth > 0 && parens == 0 && !declaration && isCSSDeclaration(tokens[i+1:]) {
				declaration = true
				w.Space()
			}
		case css.FunctionToken, css.LeftParenthesisToken:
			parens++
			w.Write(token.text)
		case css.RightParenthesisToken:
			parens = max(parens-1, 0)
			w.Write(token.text)
		default:
			w.Write(token.text)
		}
	}
	return w.Bytes(), nil
}

func lexCSS(source []byte) ([]cssToken, error) {
	lexer := css.NewLexer(parse.NewInputBytes(source))
	var tokens []cssToken
	var pending cssToken
	for {
		tt, data := lexer.Next()
		switch tt {
		case css.ErrorToken:
			if err := lexer.Err(); !errors.Is(err, io.EOF) {
				return nil, err
			}
			return tokens, nil
		case css.WhitespaceToken:
			pending.space = true
			pending.newlines += strings.Count(string(data), "\n")
		default:
			pending.tt, pending.text = tt, string(data)
			tokens = append(tokens, pending)
			pending = cssToken{}
		}
	}
}

func isCSSDeclaration(rest []cssToken) bool {
	parens := 0
	for _, token := range rest {
		switch token.tt {
		case css.FunctionToken, css.LeftParenthesisToken:
			parens++
		case css.RightParenthesisToken:
			parens--
		case css.LeftBraceToken:
			return false
		case css.SemicolonToken, css.RightBraceToken:
			if parens <= 0 {
				return true
			}
		}
	}
	return true
}
