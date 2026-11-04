package view

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
)

// codeFont is the source view's font (--font-mono, --text-code), as the
// editor's default; the gutter draws its numbers in it.
const (
	codeFont     = "monospace"
	codeFontSize = 13
	// sourceLineHeight is --source-line-height.
	sourceLineHeight = 22.0
	// gutterStart is --source-gutter-start; gutterEnd is --source-gutter-end
	// less the 8 DIPs the editor keeps before its text.
	gutterStart = 22
	gutterEnd   = 14 - 8
)

// source shows code read-only with the web's gutter of line numbers; find
// and copy are the editor's. Source selects lines from its gutter (decision
// 0008); formatted text's numbers do not address the file. A reread keeps
// the view where it stands.
func (a *App) source(c *ui.Context, code *editor.Theme, palette theme.Palette) {
	m := a.Model
	d := &a.doc
	key, kind := m.OpenPlaceKey(), m.View.Kind
	switch {
	case a.code == nil || d.codeKey != key || d.codeKind != kind:
		// A reread in Format flips the kind while its text is on the way: the
		// new editor starts where the last stood, unless a reset asked otherwise.
		if a.code != nil && d.codeKey == key && !d.restoring {
			d.restore, d.restoring = a.code.ScrollY(), true
		}
		label := "File"
		if m.Page.DocumentType != nil && m.Page.DocumentType.Label != "" {
			label = m.Page.DocumentType.Label
		}
		suffix := " source"
		if kind == docview.Formatted {
			suffix = " formatted"
		}
		buffer := editor.NewBuffer(m.CodeText())
		buffer.SetLanguage(syntaxLanguage(m.View.Language, nil))
		a.code = &editor.State{Buffer: buffer, ReadOnly: true, LineNumbers: editor.LineNumbersOff, NoFolding: true, NoActiveLine: true, LineHeight: sourceLineHeight / codeFontSize, Label: label + suffix}
		a.codeVersion, a.codeLines = m.CodeVersion, -1
		d.codeKey, d.codeKind = key, kind
		d.codeEpoch++
		d.lineAnchor, d.lineDrag = 0, nil
	case a.codeVersion != m.CodeVersion:
		a.code.Buffer.Reset(m.CodeText())
		a.codeVersion = m.CodeVersion
	}
	if m.Lines != nil && m.LinesVersion == a.codeVersion && a.codeLines != a.codeVersion {
		a.code.Buffer.SetLanguage(syntaxLanguage(m.View.Language, m.Lines))
		a.codeLines = a.codeVersion
	}
	a.code.Theme = code
	d.scrolled = kind

	selectable := kind == docview.Code
	count := shownLineCount(a.code.Buffer)
	var lines *docview.LineRange
	if selectable {
		lines = docview.ClampLines(m.Selection(), count)
		d.shownLines = lines
		if d.lineDrag != nil {
			lines = d.lineDrag
		}
	}
	a.code.LineBackgrounds = nil
	if lines != nil {
		a.code.LineBackgrounds = []editor.LineBackground{{First: lines.Start - 1, Last: lines.End - 1, Color: ui.Hex(palette.AccentSubtle)}}
	}
	a.code.OnInput = nil
	if selectable {
		a.code.OnInput = func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputKeyDown || ev.Key != ui.KeyEscape || m.Selection() == nil {
				return false
			}
			a.clearLines()
			return true
		}
		a.code.ContextMenu = func(menu *ui.Menu) { a.sourceMenu(c, menu) }
	} else {
		a.code.ContextMenu = nil
	}

	width := a.gutterWidth(c, count)
	ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Clip().Children(func() {
		e := editor.View(c.Key(d.codeEpoch), a.code).Grow(1).Margin(0, 0, 0, width)
		bounds := e.LayoutBounds()
		a.placeCode(c, bounds.H)
		a.gutter(c, palette, width, bounds.H, count, selectable, lines)
	})
}

// shownLineCount is the rows the web shows: a final newline adds none.
func shownLineCount(buffer *editor.Buffer) int {
	count := buffer.LineCount()
	if count > 1 && buffer.Line(count-1) == "" {
		count--
	}
	return count
}

// gutterWidth is source.css's: the digits of the last line and its padding.
func (a *App) gutterWidth(c *ui.Context, count int) float32 {
	d := &a.doc
	if d.charWidth == 0 {
		d.charWidth, _ = c.MeasureText(0, ui.Span{Text: "0", Font: codeFont, Size: codeFontSize})
	}
	return float32(len(strconv.Itoa(count)))*d.charWidth + gutterStart + gutterEnd
}

// placeCode applies, once the editor has laid its lines out, the scroll
// the document remembers and then a line a link asked to see.
func (a *App) placeCode(c *ui.Context, height float32) {
	m := a.Model
	d := &a.doc
	waiting := d.restoring || (m.RevealKey == d.codeKey && m.LineRevealVersion != d.revealVersion)
	if !waiting {
		return
	}
	// The source shown until Format's text arrives may be too short to hold
	// a scroll meant for that text, and would clamp it away.
	if d.restoring && d.restore > 0 && m.View.FormatPending {
		return
	}
	if height <= 0 || a.code.LineTop(a.code.Buffer.LineCount()) <= 0 {
		c.Invalidate()
		return
	}
	if d.restoring {
		a.code.SetScrollY(d.restore)
		d.restoring = false
	}
	if m.RevealKey == d.codeKey && m.LineRevealVersion != d.revealVersion {
		d.revealVersion = m.LineRevealVersion
		line := min(m.RevealLine, shownLineCount(a.code.Buffer))
		top, bottom := a.code.LineTop(line-1), a.code.LineTop(line)
		scroll := a.code.ScrollY()
		// As revealLine: into view near the top third, unless visible.
		if top < scroll || bottom > scroll+float64(height) {
			a.code.SetScrollY(top - float64(height)/3)
		}
	}
}

// gutter draws the visible lines' numbers beside the editor, and turns
// presses into line selections as SourceDocument's gutter does: click,
// Shift-click, and drag; a drag shows locally until release.
func (a *App) gutter(c *ui.Context, palette theme.Palette, width, height float32, count int, selectable bool, lines *docview.LineRange) {
	d := &a.doc
	state := a.code
	lineHeight := float32(state.LineTop(1) - state.LineTop(0))
	if lineHeight <= 0 {
		c.Invalidate()
		return
	}
	lineAt := func(y float32) int {
		return max(1, min(int((float64(y)+state.ScrollY())/float64(lineHeight))+1, count))
	}
	g := ui.Box(c).Key("gutter").Absolute().Left(0).Top(0).Bottom(0).Width(width).Background(ui.Hex(palette.Surface)).Clip().Label("Line numbers")
	if selectable {
		g.Cursor(ui.CursorPointer).Focusable().FocusRing(false)
		if d.focusGutter {
			d.focusGutter = false
			g.Focus()
		}
		if g.Shortcut(0, ui.KeyEscape) && a.Model.Selection() != nil {
			a.clearLines()
		}
	}
	g.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputScroll:
			state.SetScrollY(state.ScrollY() + float64(ev.DY))
			return true
		case ui.InputPointerDown:
			if !selectable || ev.Button != 0 {
				return false
			}
			a.pressGutter(lineAt(ev.Y), ev.Mods&ui.Shift != 0, count)
			return true
		case ui.InputPointerMove:
			if d.lineDrag == nil {
				return false
			}
			next := docview.SpanLines(d.lineAnchor, lineAt(ev.Y))
			d.lineDrag = &next
			return true
		case ui.InputPointerUp:
			if d.lineDrag == nil {
				return false
			}
			a.finishDrag(count)
			return true
		}
		return false
	})
	scroll := float32(state.ScrollY())
	first := int(scroll / lineHeight)
	last := min(count-1, int((scroll+height)/lineHeight))
	muted, selectedInk := ui.Hex(palette.TextMuted), ui.Hex(palette.AccentHover)
	g.Children(func() {
		for i := first; i <= last; i++ {
			line := i + 1
			selected := lines != nil && line >= lines.Start && line <= lines.End
			row := ui.Row(c.Key(line)).Absolute().Left(0).Right(0).Top(float32(i)*lineHeight-scroll).Height(lineHeight).AlignItems(ui.Center).Padding(0, gutterEnd, 0, 0)
			if selected {
				row.Background(ui.Hex(palette.AccentSubtle))
			}
			row.Children(func() {
				ui.Spacer(c)
				number := ui.Text(c, strconv.Itoa(line)).Font(codeFont).FontSize(codeFontSize).TextColor(muted).Unselectable().SingleLine()
				if selected {
					number.TextColor(selectedInk).Bold()
				}
			})
		}
	})
	// The floating copy button sits on the selection's first line, over the
	// gutter so a press reaches it.
	if selectable && lines != nil && d.lineDrag == nil && lines.Start-1 >= first && lines.Start-1 <= last {
		reference := docview.LineReference(a.Model.Page.Selected, *lines)
		a.copyButton(c, reference, "Copy "+reference).Absolute().Left(2).Top(float32(lines.Start-1)*lineHeight - scroll + (lineHeight-24)/2)
	}
}

// pressGutter starts a selection at line.
func (a *App) pressGutter(line int, extend bool, count int) {
	d := &a.doc
	anchor, next := docview.StartLineSelection(docview.ClampLines(a.Model.Selection(), count), d.lineAnchor, line, extend)
	d.lineAnchor, d.lineDrag, d.focusGutter = anchor, &next, true
}

// finishDrag commits a drag, once, on release.
func (a *App) finishDrag(count int) {
	d := &a.doc
	next := d.lineDrag
	d.lineDrag = nil
	current := docview.ClampLines(a.Model.Selection(), count)
	if current == nil || *current != *next {
		a.Model.SelectLines(next)
	}
}

func (a *App) clearLines() {
	a.doc.lineAnchor, a.doc.lineDrag = 0, nil
	a.Model.SelectLines(nil)
}

// sourceMenu adds the document's copies to the editor's context menu: the
// web's address bar and status bar hold them.
func (a *App) sourceMenu(c *ui.Context, menu *ui.Menu) {
	m := a.Model
	menu.Separator()
	if menu.Item("Copy Path").Chosen() {
		c.WriteClipboard(m.Page.Selected)
	}
	if lines := a.doc.shownLines; lines != nil {
		label := fmt.Sprintf("Copy %s", docview.LineReference(m.Page.Selected, *lines))
		if menu.Item(label).Chosen() {
			c.WriteClipboard(docview.LineReference(m.Page.Selected, *lines))
		}
	}
	if link := m.DocumentLink(); link != "" && menu.Item("Copy Link").Chosen() {
		c.WriteClipboard(link)
	}
}
