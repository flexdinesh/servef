package document

import (
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/html"
)

func NewHTML() Handler {
	return FormattedCodeHandler{
		CodeHandler:  CodeHandler{Definition: Type{ID: "html", Label: "HTML", Extensions: []string{".html", ".htm"}, Language: "html", DefaultMode: "source"}},
		FormatSource: formatHTML,
	}
}

// Block elements get their own lines; whitespace around them does not render.
var htmlBlockTags = setOf("address", "article", "aside", "base", "blockquote", "body", "caption", "center", "col", "colgroup",
	"dd", "details", "dialog", "dir", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form", "frameset",
	"h1", "h2", "h3", "h4", "h5", "h6", "head", "header", "hgroup", "hr", "html", "legend", "li", "link", "main", "menu",
	"meta", "nav", "noscript", "ol", "optgroup", "option", "p", "pre", "script", "search", "section", "style", "summary",
	"table", "tbody", "td", "template", "textarea", "tfoot", "th", "thead", "title", "tr", "ul")

// Start tags that close an open <p>.
var htmlClosesP = setOf("address", "article", "aside", "blockquote", "center", "details", "dialog", "dir", "div", "dl",
	"fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hgroup", "hr",
	"main", "menu", "nav", "ol", "p", "pre", "search", "section", "table", "ul")

var htmlVoidTags = setOf("area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr")

// Verbatim elements keep their content byte for byte.
var htmlVerbatimTags = setOf("pre", "textarea", "listing", "xmp", "plaintext", "script", "style")

// Start tags that close an open element of the listed names (implied end tags).
var htmlImpliedEnds = map[string]map[string]bool{
	"li": setOf("li"), "dt": setOf("dt", "dd"), "dd": setOf("dt", "dd"), "option": setOf("option"),
	"tr": setOf("tr", "td", "th"), "td": setOf("td", "th"), "th": setOf("td", "th"),
	"thead": setOf("tbody", "tfoot", "tr", "td", "th"), "tbody": setOf("thead", "tbody", "tr", "td", "th"),
	"tfoot": setOf("thead", "tbody", "tr", "td", "th"),
}

var htmlNewlineSpace = regexp.MustCompile(`[ \t\f]*(\r\n|\r|\n)[ \t\r\n\f]*`)

func setOf(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

type htmlNode struct {
	name     string // element name; empty for text and other leaves
	open     string // start tag, or the leaf's raw text
	close    string // end tag; empty when implied or missing
	children []*htmlNode
	raw      string // verbatim element content
	text     bool   // text leaf; whitespace may be reindented
	block    bool
}

// formatHTML puts block elements on their own indented lines. Tags,
// attributes, text, comments, and verbatim content (pre, textarea, script,
// style, inline SVG) are kept; only whitespace adjacent to block elements and
// line indentation change. The lexer lowercases tag and attribute names.
func formatHTML(source []byte) ([]byte, error) {
	root, err := parseHTML(source)
	if err != nil {
		return nil, err
	}
	w := newTextLayout()
	writeHTMLChildren(w, root.children, 0)
	return w.Bytes(), nil
}

func parseHTML(source []byte) (*htmlNode, error) {
	lexer := html.NewLexer(parse.NewInputBytes(source))
	root := &htmlNode{}
	stack := []*htmlNode{root}
	var verbatim *htmlNode // element collecting raw content
	var raw strings.Builder
	nested := 0
	for {
		tt, data := lexer.Next()
		if tt == html.ErrorToken {
			if err := lexer.Err(); !errors.Is(err, io.EOF) {
				return nil, err
			}
			if verbatim != nil {
				verbatim.raw = raw.String()
			}
			markHTMLBlocks(root)
			return root, nil
		}
		name := string(lexer.Text())
		if verbatim != nil {
			// Inside verbatim elements, only the matching end tag ends collection.
			if tt == html.EndTagToken && name == verbatim.name && nested == 0 {
				verbatim.raw, verbatim.close = raw.String(), string(data)
				verbatim = nil
				continue
			}
			if tt == html.StartTagToken && name == verbatim.name {
				nested++
			} else if tt == html.EndTagToken && name == verbatim.name {
				nested--
			}
			raw.Write(data)
			continue
		}
		parent := stack[len(stack)-1]
		switch tt {
		case html.StartTagToken:
			open, void, err := readHTMLStartTag(lexer, data)
			if err != nil {
				return nil, err
			}
			for len(stack) > 1 {
				top := stack[len(stack)-1].name
				if !htmlImpliedEnds[name][top] && (top != "p" || !htmlClosesP[name]) {
					break
				}
				stack = stack[:len(stack)-1]
			}
			parent = stack[len(stack)-1]
			node := &htmlNode{name: name, open: open}
			parent.children = append(parent.children, node)
			switch {
			case void || htmlVoidTags[name]:
			case htmlVerbatimTags[name]:
				verbatim, nested = node, 0
				raw.Reset()
			default:
				stack = append(stack, node)
			}
		case html.EndTagToken:
			match := len(stack) - 1
			for match > 0 && stack[match].name != name {
				match--
			}
			if match == 0 {
				// A stray end tag stays as text so nothing is dropped.
				parent.children = append(parent.children, &htmlNode{open: string(data)})
				continue
			}
			stack[match].close = string(data)
			stack = stack[:match]
		case html.DoctypeToken:
			parent.children = append(parent.children, &htmlNode{open: string(data), block: true})
		case html.TextToken:
			parent.children = append(parent.children, &htmlNode{open: string(data), text: true})
		default:
			// Comments, SVG, MathML, and other raw leaves.
			parent.children = append(parent.children, &htmlNode{open: string(data)})
		}
	}
}

// readHTMLStartTag rebuilds a start tag from the lexer's tag, attribute, and
// close tokens. Attribute text is kept; whitespace between attributes becomes
// one space.
func readHTMLStartTag(lexer *html.Lexer, tag []byte) (string, bool, error) {
	var open strings.Builder
	open.Write(tag)
	for {
		tt, data := lexer.Next()
		switch tt {
		case html.AttributeToken:
			open.WriteByte(' ')
			open.WriteString(strings.TrimLeft(string(data), " \t\r\n\f"))
		case html.StartTagCloseToken, html.StartTagVoidToken:
			open.Write(data)
			return open.String(), tt == html.StartTagVoidToken, nil
		default:
			if err := lexer.Err(); !errors.Is(err, io.EOF) {
				return "", false, err
			}
			return open.String(), false, nil
		}
	}
}

func markHTMLBlocks(node *htmlNode) bool {
	for _, child := range node.children {
		if markHTMLBlocks(child) {
			node.block = true
		}
	}
	if htmlBlockTags[node.name] {
		node.block = true
	}
	return node.block
}

func writeHTMLChildren(w *textLayout, children []*htmlNode, depth int) {
	var run []*htmlNode
	flush := func() {
		writeHTMLInline(w, run, depth)
		run = nil
	}
	for _, child := range children {
		if !child.block {
			run = append(run, child)
			continue
		}
		flush()
		writeHTMLBlock(w, child, depth)
	}
	flush()
}

func writeHTMLBlock(w *textLayout, node *htmlNode, depth int) {
	w.indent = depth
	w.Break(1)
	switch {
	case node.name == "" || htmlVoidTags[node.name]:
		w.Write(node.open)
	case htmlVerbatimTags[node.name]:
		w.Write(node.open + node.raw + node.close)
	case !hasHTMLBlockChild(node) && !spansLines(inlineHTML(node.children)):
		w.Write(node.open)
		writeHTMLInline(w, node.children, depth)
		w.Write(node.close)
	default:
		w.Write(node.open)
		w.Break(1)
		writeHTMLChildren(w, node.children, depth+1)
		w.indent = depth
		w.Break(1)
		w.Write(node.close)
	}
	w.Break(1)
}

func hasHTMLBlockChild(node *htmlNode) bool {
	for _, child := range node.children {
		if child.block {
			return true
		}
	}
	return false
}

// writeHTMLInline writes inline content, trimming whitespace at the run's
// edges (beside block elements) and reindenting text line breaks.
func writeHTMLInline(w *textLayout, run []*htmlNode, depth int) {
	pieces := inlineHTML(run)
	if len(pieces) > 0 && !pieces[0].verbatim {
		pieces[0].text = strings.TrimLeft(pieces[0].text, htmlSpace)
	}
	if last := len(pieces) - 1; last >= 0 && !pieces[last].verbatim {
		pieces[last].text = strings.TrimRight(pieces[last].text, htmlSpace)
	}
	w.indent = depth
	for _, piece := range pieces {
		if piece.verbatim {
			w.Write(piece.text)
			continue
		}
		for i, line := range strings.Split(piece.text, "\n") {
			if i > 0 {
				w.Break(1)
			}
			w.Write(line)
		}
	}
}

const htmlSpace = " \t\r\n\f"

// htmlPiece is inline output: text whose line breaks may be reindented, or
// verbatim tags, comments, and raw content.
type htmlPiece struct {
	text     string
	verbatim bool
}

// inlineHTML renders inline nodes. In text, whitespace containing a line break
// becomes a single "\n" so lines can be reindented; other text is unchanged.
func inlineHTML(run []*htmlNode) []htmlPiece {
	var pieces []htmlPiece
	verbatim := func(text string) {
		if text != "" {
			pieces = append(pieces, htmlPiece{text: text, verbatim: true})
		}
	}
	for _, node := range run {
		switch {
		case node.text:
			pieces = append(pieces, htmlPiece{text: htmlNewlineSpace.ReplaceAllString(node.open, "\n")})
		case node.name == "":
			verbatim(node.open)
		default:
			verbatim(node.open)
			if htmlVerbatimTags[node.name] {
				verbatim(node.raw)
			} else {
				pieces = append(pieces, inlineHTML(node.children)...)
			}
			verbatim(node.close)
		}
	}
	return pieces
}

func spansLines(pieces []htmlPiece) bool {
	for _, piece := range pieces {
		if strings.ContainsAny(piece.text, "\r\n") {
			return true
		}
	}
	return false
}
