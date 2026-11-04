// Package docview decides how a document shows from the capabilities the
// server declares on its page (contract.PageData), as the web UI's
// document-view.ts does. It never inspects file names. It is pure: the
// view models of Format, image zoom, Mermaid's viewer, line selection, and
// navigation memory live here too, each mirroring its web file.
package docview

import (
	"fmt"
	"path"

	"github.com/flexdinesh/servef/pkg/contract"
)

// Mode is a view of a document a user can choose.
type Mode string

const (
	Source  Mode = "source"
	Preview Mode = "preview"
	Format  Mode = "format"
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
	// Formatted: the server's formatted text (Text), highlighted as Code.
	Formatted
	// Markdown: the Markdown preview of the source.
	Markdown
	// Image: the image preview, read from the server's image endpoint.
	Image
	// PreviewFailed: the server could not preview the file; Message says
	// why, and Source stays available.
	PreviewFailed
)

// Tone is a notice's color.
type Tone int

const (
	Neutral Tone = iota
	Warning
	Danger
)

// Notice is a bar over the document, as DocumentNotice.tsx.
type Notice struct {
	Tone Tone
	Text string
}

// Note is the page shown for a file without a view, as BinaryDocument.tsx.
type Note struct {
	Title, Meta, Message string
}

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
	// Message explains Failed and PreviewFailed.
	Message string
	// Note explains Binary and TooLarge.
	Note Note
	// Notices sit over Code and Formatted.
	Notices []Notice
	// Text is Formatted's text.
	Text string
	// FormatPending: Format is chosen and its text is on its way; Source
	// shows meanwhile.
	FormatPending bool
}

// Modes lists the views page's document declares, in toolbar order: the
// default leads. A snapshot (page.At) has Source and Format only; only
// images preview binary bytes.
func Modes(page contract.PageData) []Mode {
	kind := page.DocumentType
	if !page.HasFile || kind == nil || page.TooLarge {
		return nil
	}
	var modes []Mode
	if !page.Binary {
		modes = append(modes, Source)
	}
	if kind.PreviewKind != "" && page.At == "" && (!page.Binary || kind.PreviewKind == contract.PreviewImage) {
		if kind.DefaultMode == contract.ModePreview {
			modes = append([]Mode{Preview}, modes...)
		} else {
			modes = append(modes, Preview)
		}
	}
	if !page.Binary && kind.Formattable {
		modes = append(modes, Format)
	}
	return modes
}

// DisabledReason explains why a declared view is unavailable, or "".
func DisabledReason(page contract.PageData, mode Mode) string {
	if mode == Source || !page.PreviewDisabled {
		return ""
	}
	feature := "preview is"
	if mode == Format {
		feature = "formatting is"
	}
	return fmt.Sprintf("Large file (%s): %s off", Bytes(page.FileSize), feature)
}

// ModeLabel is a mode's toolbar label; the server names previews.
func ModeLabel(page contract.PageData, mode Mode) string {
	switch mode {
	case Preview:
		if page.DocumentType != nil && page.DocumentType.PreviewLabel != "" {
			return page.DocumentType.PreviewLabel
		}
		return "Preview"
	case Format:
		return "Format"
	}
	return "Source"
}

// Resolve picks the mode to show: requested when enabled, else the type's
// default, else the first enabled one; "" when none is.
func Resolve(page contract.PageData, requested Mode) Mode {
	var enabled []Mode
	for _, mode := range Modes(page) {
		if DisabledReason(page, mode) == "" {
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

// Requested is the mode a document asks for, as App.tsx: its own choice;
// else Source for a line anchor; else Preview once the user prefers it.
func Requested(chosen Mode, hasLines, preferPreview bool) Mode {
	switch {
	case chosen != "":
		return chosen
	case hasLines:
		return Source
	case preferPreview:
		return Preview
	}
	return ""
}

// Decide returns what the pane shows for page in mode (from Resolve) with
// format, the document's Format state (nil before any).
func Decide(page contract.PageData, mode Mode, format *FormatState) View {
	if page.Error != "" && !page.HasFile {
		if page.Selected == "" && !page.ProjectUnavailable {
			return View{Kind: None}
		}
		return View{Kind: Failed, Message: page.Error}
	}
	if !page.HasFile || page.DocumentType == nil {
		return View{Kind: None}
	}
	name := path.Base(page.Selected) + " · " + Bytes(page.FileSize)
	if page.TooLarge {
		return View{Kind: TooLarge, Note: Note{Title: "Too large to display", Meta: name, Message: "servef shows text files up to 32 MB."}}
	}
	view := View{Modes: Modes(page), Mode: mode, Language: page.DocumentType.Language, Highlight: !page.HighlightDisabled}
	if mode == "" {
		view.Kind, view.Note = Binary, Note{Title: "Binary file", Meta: name, Message: "servef shows text and images only."}
		return view
	}
	if mode == Preview {
		switch {
		case page.DocumentType.PreviewKind == contract.PreviewImage:
			view.Kind = Image
		case page.PreviewError != "":
			view.Kind, view.Message = PreviewFailed, "Could not render this preview: "+page.PreviewError
		default:
			view.Kind = Markdown
		}
		return view
	}
	if page.At != "" {
		view.Notices = append(view.Notices, snapshotNotice(page.At))
	}
	if mode == Format && format != nil && format.Kind == FormatReady {
		view.Kind, view.Text = Formatted, format.Text
		return view
	}
	view.Kind = Code
	if page.PreviewDisabled {
		view.Notices = append(view.Notices, largeFileNotice(page))
	}
	failed := format != nil && format.Kind == FormatFailed
	view.FormatPending = mode == Format && !failed
	if failed {
		view.Notices = append(view.Notices, Notice{Danger, FormatFailureMessage(format.Message)})
	}
	return view
}

func largeFileNotice(page contract.PageData) Notice {
	text := fmt.Sprintf("Large file (%s): preview and formatting are off.", Bytes(page.FileSize))
	if page.SearchExcluded {
		text += " Not included in search."
	}
	return Notice{Warning, text}
}

func snapshotNotice(at string) Notice {
	short := at
	if len(short) > 7 {
		short = short[:7]
	}
	return Notice{Neutral, "Snapshot at " + short + ": read-only, without preview or code navigation."}
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
