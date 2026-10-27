// Package document owns file families and their viewing behavior.
package document

import (
	"bytes"
	"net/url"
	"path"
	"strings"
	"sync"

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

func newMarkdown() Handler { return newMarkdownHandler() }

func newMarkdownHandler() *markdownHandler {
	return &markdownHandler{markdown: goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(
			util.Prioritized(&escapedHTMLRenderer{}, 500),
		)),
	)}
}

var narrationMarkdown = sync.OnceValue(newMarkdownHandler)

// RenderMarkdown renders Markdown that is not a file, such as walkthrough
// narration, as a Markdown preview does: raw HTML escaped, local links and
// images rewritten against context.
func RenderMarkdown(source []byte, context Context) (string, error) {
	return narrationMarkdown().Preview(source, context)
}

func (*markdownHandler) Type() Type {
	return Type{ID: "markdown", Label: "Markdown", Extensions: []string{".md", ".markdown"}, Language: "markdown", PreviewKind: "markdown", PreviewLabel: "Preview", DefaultMode: "source"}
}

func (a *markdownHandler) Preview(source []byte, context Context) (string, error) {
	var rendered bytes.Buffer
	if err := a.renderMarkdown(source, context, &rendered); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func (a *markdownHandler) renderMarkdown(source []byte, context Context, output *bytes.Buffer) error {
	source = markdownPreviewSource(source)
	document := a.markdown.Parser().Parse(text.NewReader(source))
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch value := node.(type) {
			case *ast.Link:
				value.Destination = rewriteDocumentLink(value.Destination, context)
			case *ast.Image:
				value.Destination = rewriteMarkdownImage(value.Destination, context)
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return err
	}
	return a.markdown.Renderer().Render(output, source, document)
}

// Frontmatter is only recognized at the start and with a closing delimiter.
// Slice preview input so source viewing and search retain the original bytes.
func markdownPreviewSource(source []byte) []byte {
	input := bytes.TrimPrefix(source, []byte("\xef\xbb\xbf"))
	opening, rest, found := bytes.Cut(input, []byte("\n"))
	opening = bytes.TrimSuffix(opening, []byte("\r"))
	if !found || (!bytes.Equal(opening, []byte("---")) && !bytes.Equal(opening, []byte("+++"))) {
		return source
	}
	for len(rest) > 0 {
		line, next, _ := bytes.Cut(rest, []byte("\n"))
		if bytes.Equal(bytes.TrimSuffix(line, []byte("\r")), opening) {
			return next
		}
		rest = next
	}
	return source
}

func rewriteMarkdownImage(destination []byte, context Context) []byte {
	target, err := url.Parse(string(destination))
	if err != nil || target.Scheme != "" || target.Host != "" || target.Path == "" {
		return destination
	}

	imagePath := target.Path
	if strings.HasPrefix(imagePath, "/") {
		imagePath = strings.TrimPrefix(imagePath, "/")
	} else {
		imagePath = path.Join(path.Dir(context.Path), imagePath)
	}
	imagePath = path.Clean(imagePath)

	query := target.Query()
	query.Set("path", imagePath)
	return []byte((&url.URL{
		Path:     "/api/image",
		RawQuery: projectQuery(query, context.Project),
		Fragment: target.Fragment,
	}).String())
}

func rewriteDocumentLink(destination []byte, context Context) []byte {
	target, err := url.Parse(string(destination))
	// Directory links (trailing slash) never name a file.
	if err != nil || target.Scheme != "" || target.Host != "" || target.Path == "" || strings.HasSuffix(target.Path, "/") {
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
		RawQuery: projectQuery(query, context.Project),
		Fragment: target.Fragment,
	}).String())
}

// projectQuery encodes a rewritten link's query with the current project
// first, matching client URLs; links cannot name another project.
func projectQuery(query url.Values, project string) string {
	query.Del("project")
	if project == "" {
		return query.Encode()
	}
	return url.Values{"project": {project}}.Encode() + "&" + query.Encode()
}

type escapedHTMLRenderer struct{}

func (*escapedHTMLRenderer) RegisterFuncs(register renderer.NodeRendererFuncRegisterer) {
	register.Register(ast.KindHTMLBlock, renderEscapedHTMLBlock)
	register.Register(ast.KindRawHTML, renderEscapedRawHTML)
}

func renderEscapedHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.Write(util.EscapeHTML(withoutHTMLComments(node.(*ast.HTMLBlock).Text(source))))
	}
	return ast.WalkContinue, nil
}

func renderEscapedRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	raw := node.(*ast.RawHTML)
	_, _ = w.Write(util.EscapeHTML(withoutHTMLComments(raw.Text(source))))
	return ast.WalkSkipChildren, nil
}

// Goldmark identifies HTML outside code. Respect tag quotes so comment-like
// attribute values remain literal, while comments inside HTML blocks disappear.
func withoutHTMLComments(source []byte) []byte {
	var output bytes.Buffer
	inTag := false
	var quote byte
	for i := 0; i < len(source); i++ {
		if !inTag && bytes.HasPrefix(source[i:], []byte("<!--")) {
			end := bytes.Index(source[i+4:], []byte("-->"))
			if end < 0 {
				break
			}
			i += end + 6
			continue
		}
		value := source[i]
		if inTag {
			if quote != 0 {
				if value == quote {
					quote = 0
				}
			} else if value == '\'' || value == '"' {
				quote = value
			} else if value == '>' {
				inTag = false
			}
		} else if value == '<' && i+1 < len(source) {
			next := source[i+1]
			inTag = next == '/' || next == '!' || next == '?' || (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z')
		}
		output.WriteByte(value)
	}
	return output.Bytes()
}
