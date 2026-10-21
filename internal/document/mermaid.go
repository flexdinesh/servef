package document

import (
	"strings"

	"github.com/yuin/goldmark/util"
)

// mermaidHandler previews a Mermaid file as one escaped mermaid fence, the
// same markup Markdown emits, so the UI's Markdown preview renders the diagram.
// Chroma has no Mermaid lexer; Source shows plain text.
type mermaidHandler struct{}

func newMermaid() Handler { return mermaidHandler{} }

func (mermaidHandler) Type() Type {
	return Type{ID: "mermaid", Label: "Mermaid", Extensions: []string{".mmd", ".mermaid"}, Language: "plaintext", PreviewKind: PreviewMarkdown, PreviewLabel: "Preview", DefaultMode: "preview"}
}

func (mermaidHandler) Preview(source []byte, _ Context) (string, error) {
	var preview strings.Builder
	preview.WriteString(`<pre><code class="language-mermaid">`)
	preview.Write(util.EscapeHTML(source))
	preview.WriteString("</code></pre>\n")
	return preview.String(), nil
}
