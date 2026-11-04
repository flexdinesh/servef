package view

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/mdlink"
	"github.com/flexdinesh/servef/apps/native/internal/tabs"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
)

const toolbarHeight = 44

// documentState is the document pane's view state: where each view stands
// for the document it last showed. The model keeps what outlives a view.
type documentState struct {
	// shown is the place key of the document on screen, shownPath its path,
	// resetVersion the model's last scroll reset. restore, when restoring, is the scroll the
	// next view drawn starts at.
	shown        string
	shownPath    string
	resetVersion uint64
	restore      float64
	restoring    bool
	// scrolled is the kind of view whose scroll stands for the document.
	scrolled docview.Kind

	// code is which document and kind a.code shows.
	codeKey       string
	codeKind      docview.Kind
	codeEpoch     int
	charWidth     float32
	lineAnchor    int
	lineDrag      *docview.LineRange
	focusGutter   bool
	revealVersion uint64
	// shownLines are the selected lines on screen, for the status bar.
	shownLines *docview.LineRange

	preview ui.ScrollState
	// blocksPath is the document a.blocks computed for.
	blocksPath string

	// The open image document.
	imageKey     string
	imageVersion int
	image        ui.ImageSource
	viewer       docview.ImageViewer
	imageScroll  ui.ScrollState
	focusImage   bool

	diagram *expandedDiagram

	// copied is what a copy button copied last, until copiedUntil.
	copied      string
	copiedUntil time.Time
}

// document shows the open document as the model decided.
func (a *App) document(c *ui.Context, code *editor.Theme, palette theme.Palette) {
	m := a.Model
	view := m.View
	t := c.Theme()
	a.doc.shownLines = nil
	if m.Page.Selected != "" {
		a.documentToolbar(c, view)
		// The toolbar's mode switch can change the view: the frame draws the new
		// one, or source would build an editor for a view without code.
		view = m.View
		for _, notice := range view.Notices {
			noticeBar(c, palette, notice)
		}
	}
	a.errorLine(c)
	switch view.Kind {
	case docview.None:
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, docview.Empty(m.Page.FileCount)).TextColor(t.TextMuted)
		})
	case docview.Failed:
		errorBox(c, palette, view.Message)
	case docview.PreviewFailed:
		errorBox(c, palette, view.Message, "Switch to Source to read the original file.")
	case docview.Binary, docview.TooLarge:
		fileNote(c, view.Note)
	case docview.Code, docview.Formatted:
		a.source(c, code, palette)
	case docview.Markdown:
		a.preview(c, code)
	case docview.Image:
		a.imageView(c, palette)
	}
	a.diagramDialog(c, palette)
}

// place keeps a document's scroll when another takes the pane and asks for
// the new one's, as use-document-navigation.ts does. Rereads of the same
// document leave the scroll alone.
func (a *App) place() {
	m := a.Model
	d := &a.doc
	if m.Page.Selected == "" {
		return
	}
	key := m.OpenPlaceKey()
	switch {
	case d.shown != key:
		// A closed tab's document was forgotten with it.
		if d.shown != "" && tabs.Includes(m.Tabs, tabs.DocumentTab(d.shownPath)) {
			if y, ok := a.scrollY(); ok {
				m.RememberScroll(d.shown, y)
			}
		}
		d.shown, d.shownPath, d.resetVersion = key, m.Page.Selected, m.ScrollResetVersion
		d.restore, d.restoring = m.Place(key).Scroll, true
		d.scrolled = docview.None
		d.diagram = nil
		// A Bitmap keeps its encoded bytes: only the image on screen holds them.
		d.image, d.imageKey = nil, ""
	case d.resetVersion != m.ScrollResetVersion:
		d.resetVersion = m.ScrollResetVersion
		d.restore, d.restoring = 0, true
	}
}

// scrollY is the shown view's scroll; ok is false when none has any.
func (a *App) scrollY() (float64, bool) {
	// A scroll still waiting to apply is where the document stands.
	if a.doc.scrolled != docview.None && a.doc.restoring {
		return a.doc.restore, true
	}
	switch a.doc.scrolled {
	case docview.Code, docview.Formatted:
		if a.code != nil {
			return a.code.ScrollY(), true
		}
	case docview.Markdown:
		return float64(a.doc.preview.Y), true
	case docview.Image:
		return float64(a.doc.imageScroll.Y), true
	}
	return 0, false
}

// documentToolbar is the document's path and actions over its view.
func (a *App) documentToolbar(c *ui.Context, view docview.View) {
	m := a.Model
	t := c.Theme()
	// A fixed height: the views below do not jump with the controls shown.
	ui.Row(c).Height(toolbarHeight).Padding(0, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if iconButton(c, "Back", iconBack).Disabled(!m.CanGoBack()).Clicked() {
			m.GoBack()
		}
		if iconButton(c, "Forward", iconForward).Disabled(!m.CanGoForward()).Clicked() {
			m.GoForward()
		}
		ui.Text(c, m.Page.Selected).SingleLine().Shrink(1)
		if m.Page.HasFile {
			a.copyButton(c, m.Page.Selected, "Copy path")
		}
		ui.Spacer(c)
		if view.FormatPending {
			ui.Spinner(c).Label("Formatting")
		}
		a.modeSwitch(c, view)
		if view.Kind == docview.Image {
			a.imageZoom(c)
		}
	})
}

// modeSwitch shows the document's views when it has more than one; one the
// server disabled explains why instead of switching.
func (a *App) modeSwitch(c *ui.Context, view docview.View) {
	if len(view.Modes) < 2 {
		return
	}
	m := a.Model
	t := c.Theme()
	selected := 0
	for i, mode := range view.Modes {
		if mode == view.Mode {
			selected = i
		}
	}
	parts := ui.SegmentedBase(c, &selected, len(view.Modes))
	parts.Track.AlignItems(ui.Stretch).Padding(t.Space(0.5)).Gap(t.Space(0.5)).Radius(t.Radius).Background(t.Surface).Border(1, t.Border).Label("Document view")
	parts.Track.Children(func() {
		for i, mode := range view.Modes {
			reason := docview.DisabledReason(m.Page, mode)
			segment := parts.Segment(i).Padding(t.Space(1), t.Space(3)).Radius(max(t.Radius-t.Space(0.5), 0)).Disabled(reason != "")
			if reason != "" {
				segment.Tooltip(reason)
			}
			if i == selected {
				segment.Background(t.SurfacePressed)
			}
			segment.Children(func() {
				text := ui.Text(c, docview.ModeLabel(m.Page, mode)).SingleLine()
				if reason != "" {
					text.TextColor(t.TextMuted)
				}
			})
		}
	})
	if parts.Track.Changed() && selected >= 0 && selected < len(view.Modes) {
		m.SetMode(view.Modes[selected])
	}
}

// navigationKeys are the window's back and forward: Alt+Left and Alt+Right,
// and Cmd+[ and Cmd+] as editors and browsers have them.
func (a *App) navigationKeys(c *ui.Context) {
	m := a.Model
	c.OnShortcut(ui.Alt, ui.KeyLeft, m.GoBack)
	c.OnShortcut(ui.Alt, ui.KeyRight, m.GoForward)
	c.OnShortcut(ui.Cmd, ui.KeyBracketLeft, m.GoBack)
	c.OnShortcut(ui.Cmd, ui.KeyBracketRight, m.GoForward)
}

// noticeBar is a Notice in its bar layout (notice.css).
func noticeBar(c *ui.Context, palette theme.Palette, notice docview.Notice) {
	ink, background := ui.Hex(palette.TextSecondary), ui.Hex(palette.SurfaceSecondary)
	switch notice.Tone {
	case docview.Warning:
		ink, background = ui.Hex(palette.Warning), ui.Hex(palette.WarningSubtle)
	case docview.Danger:
		ink, background = ui.Hex(palette.Destructive), ui.Hex(palette.DestructiveSubtle)
	}
	ui.Row(c).Padding(8, 12).Background(background).BorderWidth(0, 0, 1, 0).BorderColor(background.Mix(ink, 0.35)).AlignSelf(ui.Stretch).Role(ui.RoleStatus).Children(func() {
		ui.Text(c, notice.Text).FontSize(12).TextColor(ink).Wrap().Selectable()
	})
}

// errorBox is the web's .error: a failure the pane shows instead of a view.
func errorBox(c *ui.Context, palette theme.Palette, lines ...string) {
	ink := ui.Hex(palette.Destructive)
	ui.Column(c).Fill().Padding(24).Children(func() {
		ui.Column(c).Padding(12, 16).Gap(8).Border(1, ink).Radius(4).Background(ui.Hex(palette.DestructiveSubtle)).MaxWidth(720).Role(ui.RoleStatus).Children(func() {
			for _, line := range lines {
				ui.Text(c, line).FontSize(14).TextColor(ink).Wrap().Selectable()
			}
		})
	})
}

// fileNote is BinaryDocument's FileNote: a file without a view.
func fileNote(c *ui.Context, note docview.Note) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Padding(24).Gap(8).Children(func() {
		ui.Text(c, note.Title).FontSize(18).Bold().Wrap()
		ui.Text(c, note.Meta).FontSize(14).TextColor(t.TextMuted).Wrap()
		ui.Text(c, note.Message).FontSize(14).TextColor(t.TextMuted).Wrap()
	})
}

// preview shows a Markdown preview: links open through the model, images
// load from the server's image endpoint, fences highlight with
// Highlights, and Mermaid draws as diagrams. A reread keeps its scroll and
// what its blocks computed.
func (a *App) preview(c *ui.Context, code *editor.Theme) {
	m := a.Model
	t := c.Theme()
	d := &a.doc
	_, palette := windowTheme(t)
	th := markdownTheme(t, palette, code)
	a.markdown.Theme = th
	a.markdown.Anchor, a.markdown.AnchorVersion = m.Anchor, m.AnchorVersion
	a.markdown.OnLink = func(url string) {
		if external := m.FollowLink(url); external != "" && ui.SafeToOpen(external) {
			c.OpenURL(external)
		}
	}
	read := m.ImageReader()
	a.markdown.LoadImage = func(url string) (ui.ImageSource, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		data, err := read(ctx, url)
		if err != nil {
			return nil, err
		}
		return ui.DecodeImage(data)
	}
	a.blocks.use(a.blocksKey(), m.DocumentVersion)
	d.blocksPath = m.Page.Selected
	a.markdown.CodeBlock = func(c *ui.Context, b *markdown.CodeBlockView) { a.codeBlock(c, b, th) }
	a.markdown.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){
		"mermaid": func(c *ui.Context, b *markdown.CodeBlockView) { a.mermaidBlock(c, b, th, palette) },
	}
	source := mdlink.Preview(m.Page.Source, m.Page.DocumentType)
	if d.restoring {
		d.preview.Y, d.restoring = float32(d.restore), false
	}
	d.scrolled = docview.Markdown
	ui.Scroll(c).Grow(1).FillWidth().TrackScroll(&d.preview).Children(func() {
		ui.Column(c).Padding(16, 24).FillWidth().Children(func() {
			markdown.View(c.Key(m.Page.Selected), source, &a.markdown)
		})
	})
}

// blocksKey names the document the preview's blocks were computed from.
func (a *App) blocksKey() string {
	return fmt.Sprint(a.Model.Page.Project, "\x00", a.Model.Page.Selected)
}

// copyButton copies text, showing a check for a moment as CopyButton does.
func (a *App) copyButton(c *ui.Context, text, label string) ui.Element {
	d := &a.doc
	icon := iconCopy
	if d.copied == text && c.Now().Before(d.copiedUntil) {
		icon = iconCheck
		c.After(d.copiedUntil.Sub(c.Now()))
	}
	b := iconButton(c, label, icon)
	if b.Clicked() {
		c.WriteClipboard(text)
		d.copied, d.copiedUntil = text, c.Now().Add(1600*time.Millisecond)
	}
	return b
}
