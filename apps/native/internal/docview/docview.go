// Package docview decides how a document shows from the capabilities the
// server declares on its page (contract.PageData), as the web UI's
// document-view.ts does. It never inspects file names. It is pure.
//
// Native M1 has Source and Preview; Format is not offered yet.
package docview

import (
	"fmt"

	"github.com/flexdinesh/servef/pkg/contract"
)

// Mode is a view of a document a user can choose.
type Mode string

const (
	Source  Mode = "source"
	Preview Mode = "preview"
)

// Kind is what the document pane shows.
type Kind int

const (
	// None: no document is selected.
	None Kind = iota
	// Failed: the document could not be read; Message says why.
	Failed
	// TooLarge: the file is over the read limit; no content was sent.
	TooLarge
	// Binary: the file has no text source and no preview.
	Binary
	// Code: the source text, highlighted when Highlight is true.
	Code
	// Markdown: the Markdown preview of the source.
	Markdown
	// Image: the image preview, read from the server's image endpoint.
	Image
)

// View is the document pane's content.
type View struct {
	Kind Kind
	// Modes are the views the document offers, in toolbar order, and Mode
	// the one shown; empty without a choice.
	Modes []Mode
	Mode  Mode
	// Language is the declared highlight language ("text" for none).
	Language string
	// Highlight is false when the server disabled highlighting.
	Highlight bool
	// Message explains Failed, TooLarge, and Binary, and a disabled preview.
	Message string
}

// Modes lists the views page's document declares, in toolbar order: the
// default leads. A snapshot (page.At) has Source only here.
func Modes(page contract.PageData) []Mode {
	kind := page.DocumentType
	if !page.HasFile || kind == nil || page.TooLarge {
		return nil
	}
	var modes []Mode
	if !page.Binary {
		modes = append(modes, Source)
	}
	if previewable(page) {
		if kind.DefaultMode == contract.ModePreview {
			modes = append([]Mode{Preview}, modes...)
		} else {
			modes = append(modes, Preview)
		}
	}
	return modes
}

// previewable: a declared preview this client renders, outside snapshots;
// only images preview binary bytes.
func previewable(page contract.PageData) bool {
	switch page.DocumentType.PreviewKind {
	case contract.PreviewMarkdown:
		return page.At == "" && !page.Binary
	case contract.PreviewImage:
		return page.At == ""
	}
	return false
}

// Resolve picks the mode to show: requested when enabled, else the type's
// default, else the first enabled one; "" when none is.
func Resolve(page contract.PageData, requested Mode) Mode {
	var enabled []Mode
	for _, mode := range Modes(page) {
		if mode == Source || !page.PreviewDisabled {
			enabled = append(enabled, mode)
		}
	}
	has := func(mode Mode) bool {
		for _, m := range enabled {
			if m == mode {
				return true
			}
		}
		return false
	}
	if requested != "" && has(requested) {
		return requested
	}
	if page.DocumentType != nil {
		if fallback := Mode(page.DocumentType.DefaultMode); has(fallback) {
			return fallback
		}
	}
	if len(enabled) > 0 {
		return enabled[0]
	}
	return ""
}

// Decide returns what the pane shows for page with requested as the user's
// last choice of mode.
func Decide(page contract.PageData, requested Mode) View {
	if page.Error != "" && !page.HasFile {
		if page.Selected == "" && !page.ProjectUnavailable {
			return View{Kind: None}
		}
		return View{Kind: Failed, Message: page.Error}
	}
	if !page.HasFile || page.DocumentType == nil {
		return View{Kind: None}
	}
	if page.TooLarge {
		return View{Kind: TooLarge, Message: fmt.Sprintf("This file is too large to show (%s).", Bytes(page.FileSize))}
	}
	view := View{Modes: Modes(page), Language: page.DocumentType.Language, Highlight: !page.HighlightDisabled}
	view.Mode = Resolve(page, requested)
	if page.PreviewDisabled && len(view.Modes) > 1 {
		view.Message = fmt.Sprintf("Large file (%s): preview is off", Bytes(page.FileSize))
	}
	switch {
	case view.Mode == "":
		view.Kind, view.Message = Binary, "Binary file: no text to show."
	case view.Mode == Preview && page.DocumentType.PreviewKind == contract.PreviewImage:
		view.Kind = Image
	case view.Mode == Preview:
		view.Kind = Markdown
	default:
		view.Kind = Code
	}
	return view
}

// Bytes formats a size as the web UI's formatBytes does.
func Bytes(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	case size < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
	}
}

// Empty is the document pane's text when no file is open.
func Empty(fileCount int) string {
	if fileCount == 1 {
		return "1 file. Choose it in the tree."
	}
	return fmt.Sprintf("%d files. Choose one in the tree.", fileCount)
}
