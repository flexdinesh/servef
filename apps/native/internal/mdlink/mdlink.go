// Package mdlink resolves the links and images of a Markdown preview to
// what native opens, with the server's document-link rules
// (internal/document markdown.go): a relative destination is relative to
// the document's directory, a leading "/" to the project root, and the
// result is a project-relative path the server confines to the project. It
// is pure.
package mdlink

import (
	"net/url"
	"path"
	"strings"

	"github.com/flexdinesh/servef/pkg/contract"
)

// Kind is what a destination opens.
type Kind int

const (
	// Ignored: nothing happens (an unsafe scheme, a directory, an escape
	// from the project).
	Ignored Kind = iota
	// External: an http, https, or mailto URL, opened by the system.
	External
	// Anchor: a heading of this document.
	Anchor
	// Document: a project file, opened in the app.
	Document
)

// Link is a resolved destination.
type Link struct {
	Kind Kind
	// URL is External's address.
	URL string
	// Path is Document's project-relative path.
	Path string
	// Fragment is an Anchor's or a Document's heading, without "#".
	Fragment string
}

// Resolve resolves a link destination of the document at docPath.
func Resolve(docPath, destination string) Link {
	target, err := url.Parse(strings.TrimSpace(destination))
	if err != nil {
		return Link{}
	}
	if target.Scheme != "" || target.Host != "" {
		switch strings.ToLower(target.Scheme) {
		case "http", "https", "mailto":
			return Link{Kind: External, URL: target.String()}
		}
		return Link{}
	}
	if target.Path == "" {
		if target.Fragment == "" {
			return Link{}
		}
		return Link{Kind: Anchor, Fragment: target.Fragment}
	}
	// Directory links (trailing slash) never name a file.
	if strings.HasSuffix(target.Path, "/") {
		return Link{}
	}
	resolved, ok := projectPath(docPath, target.Path)
	if !ok {
		return Link{}
	}
	return Link{Kind: Document, Path: resolved, Fragment: target.Fragment}
}

// Image resolves an image destination of the document at docPath to the
// project-relative path the server's image endpoint reads; ok is false for
// remote, data, and escaping destinations, which native never loads.
func Image(docPath, destination string) (string, bool) {
	target, err := url.Parse(strings.TrimSpace(destination))
	if err != nil || target.Scheme != "" || target.Host != "" || target.Path == "" {
		return "", false
	}
	return projectPath(docPath, target.Path)
}

func projectPath(docPath, destination string) (string, bool) {
	var resolved string
	if strings.HasPrefix(destination, "/") {
		resolved = path.Clean(strings.TrimPrefix(destination, "/"))
	} else {
		resolved = path.Clean(path.Join(path.Dir(docPath), destination))
	}
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") {
		return "", false
	}
	return resolved, true
}

// PreviewSource is the part of a Markdown document its preview shows: the
// server's rule (markdownPreviewSource) drops leading frontmatter, between
// "---" or "+++" lines, after an optional byte-order mark. Source views
// keep the original text.
func PreviewSource(source string) string {
	input := strings.TrimPrefix(source, "\xef\xbb\xbf")
	opening, rest, found := strings.Cut(input, "\n")
	opening = strings.TrimSuffix(opening, "\r")
	if !found || (opening != "---" && opening != "+++") {
		return source
	}
	for rest != "" {
		line, next, _ := strings.Cut(rest, "\n")
		if strings.TrimSuffix(line, "\r") == opening {
			return next
		}
		rest = next
	}
	return source
}

// MermaidFamily is the document family the server declares for Mermaid
// files (apps/servef/internal/document/mermaid.go: previewKind markdown,
// previewed as one mermaid fence). The contract has no diagram capability;
// the declared family is the server's own metadata, not a guess from the
// path, and testdata/api/default/page-mermaid.json (checked against the
// server's registry) carries it.
const MermaidFamily = "mermaid"

// Preview is the Markdown a page's preview shows: a Mermaid file's source
// as one mermaid fence, as the server previews it, so the diagram renders
// as in a Markdown document; any other document's PreviewSource.
func Preview(source string, fileType *contract.FileType) string {
	if fileType == nil || fileType.ID != MermaidFamily {
		return PreviewSource(source)
	}
	fence := "```"
	for strings.Contains(source, fence) {
		fence += "`"
	}
	if !strings.HasSuffix(source, "\n") {
		source += "\n"
	}
	return fence + "mermaid\n" + source + fence + "\n"
}
