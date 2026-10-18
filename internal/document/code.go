package document

import (
	"bytes"
	"html"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// CodeHandler is reusable for explicitly registered source languages and image
// families: highlighted source and no HTML preview.
type CodeHandler struct {
	Definition Type
}

func (h CodeHandler) Type() Type                              { return h.Definition }
func (h CodeHandler) Preview([]byte, Context) (string, error) { return "", nil }

// FormattedCodeHandler adds an on-demand, read-only formatted view to
// highlighted source. FormatSource receives a copy of the file bytes.
type FormattedCodeHandler struct {
	CodeHandler
	FormatSource func([]byte) ([]byte, error)
}

func (h FormattedCodeHandler) Format(source []byte) ([]byte, error) {
	return h.FormatSource(append([]byte{}, source...))
}

// HighlightLines returns escaped HTML per displayed source line, preserving
// multiline lexical state. Unknown languages, lexer errors, and lexer panics
// fall back to escaped original text; syntax recognition never blocks viewing.
func HighlightLines(source []byte, language string) []string {
	text, fallback := splitLines(source)
	lexer := lexers.Get(language)
	if lexer == nil || text == "" {
		return fallback
	}
	if lines, ok := highlight(lexer, text, len(fallback)); ok {
		return lines
	}
	return fallback
}

// PlainLines returns escaped HTML per displayed source line without highlighting.
func PlainLines(source []byte) []string {
	_, lines := splitLines(source)
	return lines
}

func splitLines(source []byte) (string, []string) {
	text := strings.ReplaceAll(strings.ReplaceAll(string(source), "\r\n", "\n"), "\r", "\n")
	raw := strings.Split(text, "\n")
	if len(raw) > 1 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	lines := make([]string, len(raw))
	for i, line := range raw {
		lines[i] = html.EscapeString(line)
	}
	return text, lines
}

func highlight(lexer chroma.Lexer, text string, want int) (result []string, ok bool) {
	defer func() {
		if recover() != nil {
			result, ok = nil, false
		}
	}()
	iterator, err := chroma.Coalesce(lexer).Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, text)
	if err != nil {
		return nil, false
	}
	lines := chroma.SplitTokensIntoLines(iterator.Tokens())
	if len(lines) != want {
		return nil, false
	}
	formatter := chromahtml.New(chromahtml.WithClasses(true), chromahtml.ClassPrefix("syntax-"), chromahtml.PreventSurroundingPre(true))
	result = make([]string, len(lines))
	for i, tokens := range lines {
		for j := range tokens {
			tokens[j].Value = strings.TrimSuffix(tokens[j].Value, "\n")
		}
		var output bytes.Buffer
		if err := formatter.Format(&output, styles.Fallback, chroma.Literator(tokens...)); err != nil {
			return nil, false
		}
		result[i] = output.String()
	}
	return result, true
}
