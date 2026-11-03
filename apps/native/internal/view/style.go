package view

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/flexdinesh/servef/apps/native/internal/highlight"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
)

// tokenKinds maps servef's syntax categories to the editor's token kinds.
var tokenKinds = map[highlight.Category]editor.TokenKind{
	highlight.Plain:    editor.TokenText,
	highlight.Comment:  editor.TokenComment,
	highlight.Keyword:  editor.TokenKeyword,
	highlight.Type:     editor.TokenType,
	highlight.Function: editor.TokenFunction,
	highlight.Variable: editor.TokenVariable,
	highlight.String:   editor.TokenString,
	highlight.Number:   editor.TokenNumber,
	highlight.Operator: editor.TokenOperator,
}

// windowTheme is the system's light or dark theme with servef's colors.
func windowTheme(system *ui.Theme) (*ui.Theme, theme.Palette) {
	palette := theme.For(system.Dark)
	t := *system
	t.Background = ui.Hex(palette.Bg)
	t.Surface = ui.Hex(palette.SurfaceSecondary)
	t.Border = ui.Hex(palette.Border)
	t.Text = ui.Hex(palette.TextPrimary)
	t.TextMuted = ui.Hex(palette.TextMuted)
	t.Accent = ui.Hex(palette.Accent)
	t.Danger = ui.Hex(palette.Destructive)
	t.Warning = ui.Hex(palette.Warning)
	t.Success = ui.Hex(palette.Success)
	return &t, palette
}

// codeTheme colors code from servef's syntax tokens on its document
// surface. Kinds servef has no category for keep the editor's defaults.
func codeTheme(t *ui.Theme, palette theme.Palette) *editor.Theme {
	th := editor.DefaultTheme(t)
	th.Background = ui.Hex(palette.Surface)
	th.Text = ui.Hex(palette.TextPrimary)
	th.LineNumber = ui.Hex(palette.TextMuted)
	th.ActiveLineNumber = ui.Hex(palette.TextPrimary)
	th.Added = ui.Hex(palette.Success)
	th.Deleted = ui.Hex(palette.Destructive)
	th.Modified = ui.Hex(palette.Warning)
	for _, category := range highlight.Categories {
		th.Tokens[tokenKinds[category]] = editor.TokenStyle{Color: ui.Hex(palette.Syntax[category]), Italic: theme.Italic(category)}
	}
	return th
}

// markdownTheme shows Markdown on servef's document surface; code blocks
// take the code theme.
func markdownTheme(palette theme.Palette, code *editor.Theme) *markdown.Theme {
	return &markdown.Theme{
		Text:                ui.Hex(palette.TextPrimary),
		Link:                ui.Hex(palette.Accent),
		CodeBlockBackground: ui.Hex(palette.SurfaceSecondary),
		CodeBlockBorder:     ui.Hex(palette.Border),
		CodeColors:          code,
	}
}

// chromaHighlighter gives the editor tokens computed off the UI thread by
// package highlight (Chroma). The text is read-only; lines is nil until
// they are ready, and the lines then show plain.
type chromaHighlighter struct {
	lines [][]highlight.Span
}

func (h *chromaHighlighter) Edit(editor.Snapshot, editor.Snapshot, []editor.Change) {}

func (h *chromaHighlighter) Highlight(int, time.Duration) bool { return true }

func (h *chromaHighlighter) Tokens(i int, text string, dst []editor.Token) ([]editor.Token, bool) {
	if i < 0 || i >= len(h.lines) || len(h.lines[i]) == 0 {
		return append(dst, editor.Token{End: len(text), Kind: editor.TokenText}), true
	}
	for _, span := range h.lines[i] {
		end := min(span.End, len(text))
		dst = append(dst, editor.Token{End: end, Kind: tokenKinds[span.Category]})
		if end == len(text) {
			return dst, true
		}
	}
	return append(dst, editor.Token{End: len(text), Kind: editor.TokenText}), true
}

// chromaLanguage is a language whose highlighter shows lines.
func chromaLanguage(name string, lines [][]highlight.Span) *editor.Language {
	return &editor.Language{
		Name: name,
		Highlighter: func(editor.Snapshot, func()) editor.Highlighter {
			return &chromaHighlighter{lines: lines}
		},
	}
}
