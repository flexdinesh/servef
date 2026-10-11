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

// CodeHandler is reusable for explicitly registered source languages. A nil
// Format means source-only support; formatters operate on a copy of file bytes.
type CodeHandler struct {
	Definition Type
	Format     func([]byte) ([]byte, error)
}

func (h CodeHandler) Type() Type                               { return h.Definition }
func (h CodeHandler) SearchText(source []byte) (string, error) { return string(source), nil }
func (h CodeHandler) Preview(source []byte, _ Context) (string, error) {
	if h.Format == nil {
		return "", nil
	}
	formatted, err := h.Format(append([]byte{}, source...))
	if err != nil {
		return "", err
	}
	lines := HighlightLines(formatted, h.Definition.Language)
	return `<pre><code class="language-` + html.EscapeString(h.Definition.Language) + ` syntax-highlight">` + strings.Join(lines, "\n") + "</code></pre>\n", nil
}

// HighlightLines returns escaped HTML per displayed source line, preserving
// multiline lexical state. Unknown languages and lexer errors fall back to
// escaped original text; syntax recognition never blocks viewing a file.
func HighlightLines(source []byte, language string) []string {
	text := strings.ReplaceAll(strings.ReplaceAll(string(source), "\r\n", "\n"), "\r", "\n")
	raw := strings.Split(text, "\n")
	if len(raw) > 1 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	fallback := make([]string, len(raw))
	for i, line := range raw {
		fallback[i] = html.EscapeString(line)
	}
	lexer := lexers.Get(language)
	if lexer == nil || text == "" {
		return fallback
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, text)
	if err != nil {
		return fallback
	}
	lines := chroma.SplitTokensIntoLines(iterator.Tokens())
	if len(lines) != len(raw) {
		return fallback
	}
	formatter := chromahtml.New(chromahtml.WithClasses(true), chromahtml.ClassPrefix("syntax-"), chromahtml.PreventSurroundingPre(true))
	result := make([]string, len(lines))
	for i, tokens := range lines {
		for j := range tokens {
			tokens[j].Value = strings.TrimSuffix(tokens[j].Value, "\n")
		}
		var output bytes.Buffer
		if err := formatter.Format(&output, styles.Fallback, chroma.Literator(tokens...)); err != nil {
			return fallback
		}
		result[i] = output.String()
	}
	return result
}
