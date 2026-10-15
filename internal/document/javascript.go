package document

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

func NewJavaScript() Handler {
	return navigableFormattedCode{
		FormattedCodeHandler: FormattedCodeHandler{
			CodeHandler:  CodeHandler{Definition: Type{ID: "javascript", Label: "JavaScript", Extensions: []string{".js", ".mjs", ".cjs"}, Language: "javascript", DefaultMode: "source"}},
			FormatSource: formatJavaScript,
		},
		syntaxNavigator: javaScriptNavigator(javaScriptGrammar),
	}
}

var errJSXNotFormattable = errors.New("JSX is not formattable")

type jsToken struct {
	tt       js.TokenType
	text     string
	space    bool // whitespace preceded the token
	newlines int  // line breaks preceded the token
}

type jsGroup struct {
	tt       js.TokenType // opener: { ( [ or template start
	indent   int          // indent of lines inside the group
	caseBody bool         // inside a switch case body: indent one more
}

// formatJavaScript reindents code by bracket nesting and breaks lines after
// statements and blocks. Tokens are written unchanged and existing line breaks
// are kept, so comments, literals, and automatic semicolon insertion are
// preserved. Breaks are only added after ; { } and before }, which never
// changes how a program parses.
func formatJavaScript(source []byte) ([]byte, error) {
	hashbang, body := splitHashbang(source)
	tokens, err := lexJavaScript(body)
	if err != nil {
		return nil, err
	}
	w := newTextLayout()
	w.Write(hashbang)
	w.Break(1)
	var groups []jsGroup
	inner := func() int {
		if len(groups) == 0 {
			return 0
		}
		if top := groups[len(groups)-1]; top.caseBody {
			return top.indent + 1
		}
		return groups[len(groups)-1].indent
	}
	inTemplate := func() bool {
		for _, group := range groups {
			if group.tt == js.TemplateStartToken {
				return true
			}
		}
		return false
	}
	caseColon := false // a case/default label awaits its colon
	for i, token := range tokens {
		// Added breaks never precede ++/--: a line break there changes parsing.
		if token.tt == js.IncrToken || token.tt == js.DecrToken {
			w.breaks = 0
		}
		if token.newlines > 0 {
			w.Break(token.newlines)
		} else if token.space {
			w.Space()
		}

		closes := token.tt == js.CloseBraceToken || token.tt == js.CloseParenToken || token.tt == js.CloseBracketToken ||
			token.tt == js.TemplateMiddleToken || token.tt == js.TemplateEndToken
		w.indent = inner()
		if (token.tt == js.CaseToken || token.tt == js.DefaultToken) && len(groups) > 0 {
			w.indent = groups[len(groups)-1].indent
		}
		if closes && len(groups) > 0 {
			w.indent = groups[len(groups)-1].indent - 1
			groups = groups[:len(groups)-1]
		}
		breaks := !inTemplate()

		switch token.tt {
		case js.CommentToken, js.CommentLineTerminatorToken:
			if token.newlines == 0 && w.breaks > 0 {
				w.WriteTrailing(token.text)
			} else {
				w.Write(token.text)
			}
			continue
		case js.OpenBraceToken:
			if !isJSGroupOpener(w.last) {
				w.Space()
			}
		case js.CloseBraceToken:
			if w.last == "{" {
				w.breaks = 0
			} else if breaks {
				w.Break(1)
			}
		case js.ElseToken, js.CatchToken, js.FinallyToken, js.WhileToken:
			if w.last == "}" && w.breaks == 0 {
				w.Space()
			}
		}
		w.Write(token.text)

		switch token.tt {
		case js.OpenBraceToken, js.OpenParenToken, js.OpenBracketToken, js.TemplateStartToken, js.TemplateMiddleToken:
			groups = append(groups, jsGroup{tt: token.tt, indent: w.lineIndent + 1})
			if token.tt == js.TemplateMiddleToken {
				groups[len(groups)-1].tt = js.TemplateStartToken
			}
		}
		if !breaks {
			continue
		}
		statement := len(groups) == 0 || groups[len(groups)-1].tt == js.OpenBraceToken
		switch token.tt {
		case js.OpenBraceToken:
			w.Break(1)
		case js.CloseBraceToken:
			if i+1 < len(tokens) && startsJSStatement(tokens[i+1].tt) {
				w.Break(1)
			}
		case js.SemicolonToken:
			if statement {
				w.Break(1)
			} else {
				w.Space()
			}
		case js.CommaToken:
			w.Space()
		case js.CaseToken:
			caseColon = statement
		case js.DefaultToken:
			caseColon = statement && (i == 0 || tokens[i-1].tt != js.ExportToken)
		case js.ColonToken:
			if caseColon && statement {
				caseColon = false
				groups[len(groups)-1].caseBody = true
				w.Break(1)
			}
		}
	}
	return w.Bytes(), nil
}

func splitHashbang(source []byte) (string, []byte) {
	if !bytes.HasPrefix(source, []byte("#!")) {
		return "", source
	}
	end := bytes.IndexAny(source, "\r\n")
	if end < 0 {
		return string(source), nil
	}
	return string(source[:end]), source[end:]
}

func lexJavaScript(source []byte) ([]jsToken, error) {
	lexer := js.NewLexer(parse.NewInputBytes(source))
	var tokens []jsToken
	var pending jsToken
	var previous js.TokenType = js.ErrorToken // no previous significant token
	for {
		tt, data := lexer.Next()
		switch tt {
		case js.ErrorToken:
			if err := lexer.Err(); !errors.Is(err, io.EOF) {
				return nil, err
			}
			return tokens, nil
		case js.WhitespaceToken:
			pending.space = true
			continue
		case js.LineTerminatorToken:
			// \r\n, \r, \n, U+2028, and U+2029 each end a line.
			text := strings.ReplaceAll(string(data), "\r\n", "\n")
			pending.newlines += max(1, strings.Count(text, "\n")+strings.Count(text, "\r"))
			continue
		case js.DivToken, js.DivEqToken:
			if expectsJSOperand(previous) {
				if tt, data = lexer.RegExp(); tt == js.ErrorToken {
					return nil, lexer.Err()
				}
			}
		case js.LtToken:
			// `<` where an operand is expected starts JSX, which this
			// formatter cannot lay out without changing text whitespace.
			if expectsJSOperand(previous) {
				return nil, errJSXNotFormattable
			}
		}
		pending.tt, pending.text = tt, string(data)
		tokens = append(tokens, pending)
		pending = jsToken{}
		if tt != js.CommentToken && tt != js.CommentLineTerminatorToken {
			previous = tt
		}
	}
}

// expectsJSOperand reports whether a / after previous starts a regular
// expression rather than a division.
func expectsJSOperand(previous js.TokenType) bool {
	switch previous {
	case js.ErrorToken, js.CloseBraceToken:
		return true
	case js.CloseParenToken, js.CloseBracketToken, js.IncrToken, js.DecrToken,
		js.StringToken, js.TemplateToken, js.TemplateEndToken, js.RegExpToken, js.PrivateIdentifierToken,
		js.ThisToken, js.SuperToken, js.NullToken, js.TrueToken, js.FalseToken:
		return false
	}
	return !js.IsNumeric(previous) && !js.IsIdentifier(previous)
}

// startsJSStatement reports whether a token after } begins a new statement
// rather than continuing the expression or statement (else, catch, ...).
func startsJSStatement(next js.TokenType) bool {
	switch next {
	case js.ElseToken, js.CatchToken, js.FinallyToken, js.WhileToken, js.InToken, js.InstanceofToken, js.OfToken, js.AsToken, js.FromToken:
		return false
	}
	return js.IsIdentifierName(next)
}

func isJSGroupOpener(last string) bool {
	return last == "" || last == "(" || last == "[" || last == "{" || last == "..." || strings.HasSuffix(last, "${")
}
