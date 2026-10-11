// Package document owns explicitly supported file types and their viewing behavior.
package document

import (
	"bytes"
	"net/url"
	"path"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type markdownHandler struct {
	markdown goldmark.Markdown
}

func newMarkdown() Handler {
	return &markdownHandler{markdown: goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(
			util.Prioritized(&escapedHTMLRenderer{}, 500),
			util.Prioritized(newFencedCodeRenderer(), 500),
		)),
	)}
}

func (*markdownHandler) Type() Type {
	return Type{ID: "markdown", Label: "Markdown", Extensions: []string{".md", ".markdown"}, Language: "markdown", PreviewKind: "markdown", PreviewLabel: "Preview", DefaultMode: "preview"}
}

func (a *markdownHandler) Preview(source []byte, context Context) (string, error) {
	var rendered bytes.Buffer
	if err := a.renderMarkdown(source, context, &rendered); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func (a *markdownHandler) SearchText(source []byte) (string, error) {
	document := a.markdown.Parser().Parse(text.NewReader(source))
	parts := make([]string, 0)
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch value := node.(type) {
		case *ast.Text:
			parts = append(parts, string(value.Value(source)))
		case *ast.String:
			parts = append(parts, string(value.Value))
		case *ast.AutoLink:
			parts = append(parts, string(value.Text(source)))
		case *ast.CodeBlock:
			parts = append(parts, string(value.Lines().Value(source)))
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock:
			parts = append(parts, string(value.Lines().Value(source)))
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			parts = append(parts, string(value.Text(source)))
			return ast.WalkSkipChildren, nil
		case *ast.HTMLBlock:
			parts = append(parts, string(value.Text(source)))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " "), nil
}

func (a *markdownHandler) renderMarkdown(source []byte, context Context, output *bytes.Buffer) error {
	document := a.markdown.Parser().Parse(text.NewReader(source))
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch value := node.(type) {
			case *ast.Link:
				value.Destination = rewriteDocumentLink(value.Destination, context)
			case *ast.Image:
				value.Destination = rewriteMarkdownImage(value.Destination, context.Path)
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return err
	}
	return a.markdown.Renderer().Render(output, source, document)
}

func rewriteMarkdownImage(destination []byte, selected string) []byte {
	target, err := url.Parse(string(destination))
	if err != nil || target.Scheme != "" || target.Host != "" || target.Path == "" {
		return destination
	}

	imagePath := target.Path
	if strings.HasPrefix(imagePath, "/") {
		imagePath = strings.TrimPrefix(imagePath, "/")
	} else {
		imagePath = path.Join(path.Dir(selected), imagePath)
	}
	imagePath = path.Clean(imagePath)

	query := target.Query()
	query.Set("path", imagePath)
	return []byte((&url.URL{
		Path:     "/api/image",
		RawQuery: query.Encode(),
		Fragment: target.Fragment,
	}).String())
}

func rewriteDocumentLink(destination []byte, context Context) []byte {
	target, err := url.Parse(string(destination))
	if err != nil || target.Scheme != "" || target.Host != "" || target.Path == "" {
		return destination
	}
	if context.Supports == nil || !context.Supports(target.Path) {
		return destination
	}

	markdownPath := target.Path
	if strings.HasPrefix(markdownPath, "/") {
		markdownPath = strings.TrimPrefix(markdownPath, "/")
	} else {
		markdownPath = path.Join(path.Dir(context.Path), markdownPath)
	}
	markdownPath = path.Clean(markdownPath)

	query := target.Query()
	query.Set("path", markdownPath)
	return []byte((&url.URL{
		Path:     "/view",
		RawQuery: query.Encode(),
		Fragment: target.Fragment,
	}).String())
}

type escapedHTMLRenderer struct{}

func (*escapedHTMLRenderer) RegisterFuncs(register renderer.NodeRendererFuncRegisterer) {
	register.Register(ast.KindHTMLBlock, renderEscapedHTMLBlock)
	register.Register(ast.KindRawHTML, renderEscapedRawHTML)
}

func renderEscapedHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	block := node.(*ast.HTMLBlock)
	if entering {
		for i := 0; i < block.Lines().Len(); i++ {
			line := block.Lines().At(i)
			_, _ = w.Write(util.EscapeHTML(line.Value(source)))
		}
	} else if block.HasClosure() {
		_, _ = w.Write(util.EscapeHTML(block.ClosureLine.Value(source)))
	}
	return ast.WalkContinue, nil
}

func renderEscapedRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	raw := node.(*ast.RawHTML)
	for i := 0; i < raw.Segments.Len(); i++ {
		segment := raw.Segments.At(i)
		_, _ = w.Write(util.EscapeHTML(segment.Value(source)))
	}
	return ast.WalkSkipChildren, nil
}
