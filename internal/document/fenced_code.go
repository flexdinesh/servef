package document

import (
	"bytes"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

type fencedCodeRenderer struct {
	formatter *chromahtml.Formatter
}

func newFencedCodeRenderer() *fencedCodeRenderer {
	return &fencedCodeRenderer{formatter: chromahtml.New(
		chromahtml.WithClasses(true),
		chromahtml.ClassPrefix("syntax-"),
		chromahtml.PreventSurroundingPre(true),
	)}
}

func (r *fencedCodeRenderer) RegisterFuncs(register renderer.NodeRendererFuncRegisterer) {
	register.Register(ast.KindFencedCodeBlock, r.render)
}

func (r *fencedCodeRenderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	block := node.(*ast.FencedCodeBlock)
	language := block.Language(source)
	if len(language) == 0 || bytes.EqualFold(language, []byte("mermaid")) {
		return renderPlainFencedCodeBlock(w, source, block)
	}
	lexer := lexers.Get(string(language))
	if lexer == nil {
		return renderPlainFencedCodeBlock(w, source, block)
	}

	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, string(block.Lines().Value(source)))
	if err != nil {
		return renderPlainFencedCodeBlock(w, source, block)
	}
	if _, err := w.WriteString(`<pre><code class="language-`); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.Write(util.EscapeHTML(language)); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.WriteString(` syntax-highlight">`); err != nil {
		return ast.WalkStop, err
	}
	if err := r.formatter.Format(w, styles.Fallback, iterator); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.WriteString("</code></pre>\n"); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkContinue, nil
}

func renderPlainFencedCodeBlock(w util.BufWriter, source []byte, block *ast.FencedCodeBlock) (ast.WalkStatus, error) {
	if _, err := w.WriteString("<pre><code"); err != nil {
		return ast.WalkStop, err
	}
	language := block.Language(source)
	if len(language) > 0 {
		if _, err := w.WriteString(` class="language-`); err != nil {
			return ast.WalkStop, err
		}
		if _, err := w.Write(util.EscapeHTML(language)); err != nil {
			return ast.WalkStop, err
		}
		if err := w.WriteByte('"'); err != nil {
			return ast.WalkStop, err
		}
	}
	if err := w.WriteByte('>'); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.Write(util.EscapeHTML(block.Lines().Value(source))); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.WriteString("</code></pre>\n"); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkContinue, nil
}
