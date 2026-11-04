package view

import (
	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/apps/native/internal/highlight"
	"github.com/flexdinesh/servef/apps/native/internal/review"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
	"github.com/flexdinesh/servef/apps/native/internal/workspace"
	"github.com/flexdinesh/servef/packages/mygo/diffs"
	"github.com/flexdinesh/servef/packages/mygo/diffs/diffview"
	"github.com/flexdinesh/servef/pkg/contract"
)

// reviewState is the diff viewer's state for the model's open review.
type reviewState struct {
	viewer  diffview.State
	of      *workspace.Review
	version int
	lines   int
	// spans are the sides' highlighted lines as the viewer colors them,
	// in the appearance spansDark; spansVersion counts their changes.
	spans        [2][][]diffview.Span
	spansDark    bool
	spansVersion int
	layout       int // 0 split, 1 unified
	full         bool
	space        bool
}

// changes lists the working tree's changes since HEAD; choosing one opens
// its diff.
func (a *App) changes(c *ui.Context) {
	m := a.Model
	t := c.Theme()
	files := m.Changes.Files
	ui.Row(c).Padding(6, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		label := "Changes"
		if m.ChangesRead {
			label = review.ChangeCount(len(files))
		}
		ui.Text(c, label).Bold().SingleLine().Grow(1)
		if ui.Button(c, "Refresh").Clicked() {
			m.LoadChanges()
		}
	})
	if m.ChangesError != "" {
		ui.Text(c, m.ChangesError).TextColor(t.Danger).Wrap().Padding(4, 8)
	}
	for _, warning := range m.Changes.Warnings {
		ui.Text(c, warning).TextColor(t.Warning).Wrap().Padding(4, 8)
	}
	if m.Changes.Truncated {
		ui.Text(c, "Showing the first changes only.").TextColor(t.TextMuted).Wrap().Padding(4, 8)
	}
	a.syncChange()
	list := ui.List(c, &a.changeList, len(files), func(i int) {
		file := files[i]
		ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, review.Letter(file.Status)).Bold().TextColor(statusColor(t, file.Status)).Tooltip(review.StatusName(file.Status)).Width(14)
			fileIcon(c, file.Path)
			ui.Text(c, review.Label(file)).SingleLine().Grow(1)
		})
	}).Grow(1).Label("Changes").Children(func() {
		switch {
		case !m.ChangesRead:
		case !m.Changes.Repository:
			ui.Text(c, "Source Control needs a git repository.").TextColor(t.TextMuted).Wrap().Padding(12)
		case len(files) == 0:
			ui.Text(c, "No changes since last commit.").TextColor(t.TextMuted).Padding(12)
		}
	})
	if list.Changed() || list.Submitted() {
		if i := a.changeRow; i >= 0 && i < len(files) {
			m.OpenChange(files[i])
		}
	}
}

// syncChange selects the open review's row.
func (a *App) syncChange() {
	m := a.Model
	a.changeRow = -1
	if m.Review == nil {
		return
	}
	for i, file := range m.Changes.Files {
		if file.Path == m.Review.File.Path {
			a.changeRow = i
			return
		}
	}
}

func statusColor(t *ui.Theme, status contract.ChangeStatus) ui.Color {
	switch status {
	case contract.ChangeAdded, contract.ChangeUntracked:
		return t.Success
	case contract.ChangeDeleted:
		return t.Danger
	}
	return t.Warning
}

// diff shows the open review: the server's hunks in the diffs viewer.
func (a *App) diff(c *ui.Context, palette theme.Palette) {
	m := a.Model
	open := m.Review
	t := c.Theme()
	layout := open.Layout
	shown := open.Diff != nil && open.Error == "" && layout.Kind == review.Text
	ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, review.Label(open.File)).SingleLine().Grow(1)
		if shown {
			ui.Text(c, review.Counts(layout.Additions, layout.Deletions)).TextColor(t.TextMuted).SingleLine()
			a.hunkStep(c, "Previous change", "Previous change (Shift+F7)", -1)
			a.hunkStep(c, "Next change", "Next change (F7)", 1)
			ui.Segmented(c, &a.review.layout, "Split", "Unified").Label("Diff layout")
		}
		a.review.full, a.review.space = m.FullFile, m.IgnoreWhitespace
		if ui.Toggle(c, &a.review.full, "Full file").Tooltip("Show whole files, not only the changes").Changed() {
			m.SetFullFile(a.review.full)
		}
		if ui.Toggle(c, &a.review.space, "Ignore whitespace").Tooltip("Ignore leading and trailing whitespace").Changed() {
			m.SetIgnoreWhitespace(a.review.space)
		}
		if ui.Button(c, "Close").Clicked() {
			m.CloseReview()
		}
	})
	if m.Review == nil {
		return
	}
	switch {
	case open.Error != "":
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, open.Error).TextColor(t.TextMuted).Wrap()
		})
	case open.Diff == nil:
		ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c) })
	case layout.Kind != review.Text:
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, layout.Message).TextColor(t.TextMuted).Wrap()
		})
	default:
		a.diffView(c, palette, open)
	}
}

// hunkStep is a toolbar button stepping to the next or previous change,
// as F7 and Shift+F7 do in the viewer.
func (a *App) hunkStep(c *ui.Context, label, tip string, direction int) {
	t := c.Theme()
	b := ui.ButtonBase(c).Key(label).Label(label).Tooltip(tip).Size(28, 28).Radius(6).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy, d := r.X+r.W/2, r.Y+r.H/2, float32(4)
		var path ui.Path
		if direction < 0 {
			path.MoveTo(cx-d, cy+d/2).LineTo(cx, cy-d/2).LineTo(cx+d, cy+d/2)
		} else {
			path.MoveTo(cx-d, cy-d/2).LineTo(cx, cy+d/2).LineTo(cx+d, cy-d/2)
		}
		p.StrokePath(&path, 1.5, t.Text)
	})
	if b.Clicked() {
		a.review.viewer.StepHunk(direction)
	}
}

// diffView keeps the viewer on the review's layout: the server's hunks,
// blocks, and gaps over both whole files, highlighted in the server's
// language. A reread of the same file keeps what was expanded and
// selected; selecting lines offers to copy their reference.
func (a *App) diffView(c *ui.Context, palette theme.Palette, open *workspace.Review) {
	r := &a.review
	v := &r.viewer
	switch {
	case r.of != open:
		v.SetDiff(open.Layout.Diff)
		r.of, r.version, r.lines, r.spans = open, open.Version, -1, [2][][]diffview.Span{}
		r.spansVersion++
	case r.version != open.Version:
		v.Update(open.Layout.Diff)
		r.version, r.lines, r.spans = open.Version, -1, [2][][]diffview.Span{}
		r.spansVersion++
	}
	dark := c.Theme().Dark
	if open.Old != nil && open.LinesVersion == r.version && (r.lines != r.version || r.spansDark != dark) {
		r.spans = [2][][]diffview.Span{syntaxSpans(open.Old, palette), syntaxSpans(open.New, palette)}
		r.lines, r.spansDark = r.version, dark
		r.spansVersion++
	}
	v.SpansVersion = r.spansVersion
	v.Run = a.run.Go
	v.Style = diffs.Split
	if r.layout == 1 {
		v.Style = diffs.Unified
	}
	v.ExpandUnchanged = a.Model.FullFile
	v.Theme = diffTheme(c.Theme(), palette)
	v.Spans = func(side diffs.Side, line int) []diffview.Span {
		lines := r.spans[side]
		if line < 1 || line > len(lines) {
			return nil
		}
		return lines[line-1]
	}
	file := open.File
	v.Utility = func(c *ui.Context, s diffs.Selection) {
		target, ok := v.Diff().Target(s, v.Style)
		if !ok {
			return
		}
		path := file.Path
		if target.Side == diffs.Old && file.OldPath != "" {
			path = file.OldPath
		}
		copyButton(c, target.Reference(path))
	}
	diffview.View(c.Key(open), v).Grow(1).FillWidth()
}

// copyButton copies a line reference, as the web UI's gutter copy button.
func copyButton(c *ui.Context, reference string) {
	t := c.Theme()
	b := ui.ButtonBase(c).Key("copy").Label("Copy "+reference).Tooltip("Copy "+reference).Size(18, 18).Radius(4).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) { iconCopy(p, r, t.TextMuted) })
	if b.Clicked() {
		c.WriteClipboard(reference)
	}
}

// syntaxSpans colors highlighted lines from servef's syntax tokens.
func syntaxSpans(lines [][]highlight.Span, palette theme.Palette) [][]diffview.Span {
	colors := syntaxColors(palette)
	out := make([][]diffview.Span, len(lines))
	for i, line := range lines {
		spans := make([]diffview.Span, len(line))
		for j, span := range line {
			spans[j] = diffview.Span{End: span.End, Italic: span.Category.Italic(), Bold: span.Category.Bold()}
			if color, ok := colors[span.Category]; ok {
				spans[j].Color = ui.Hex(color)
			}
		}
		out[i] = spans
	}
	return out
}

// additionTint is Servef Light's --color-diff-addition-tint (themes.css).
const additionTint = "#00c853"

// diffTheme is the viewer's look on servef's document surface, tinted as
// the web UI's diff.css tints Pierre.
func diffTheme(t *ui.Theme, palette theme.Palette) *diffview.Theme {
	surface := *t
	surface.Background = ui.Hex(palette.Surface)
	th := diffview.DefaultTheme(&surface)
	th.LineNumber = ui.Hex(palette.TextMuted)
	th.Selected = ui.Hex(palette.Accent)
	th.Separator = ui.Hex(palette.SurfaceSecondary)
	if !t.Dark {
		// diff.css: Servef Light mixes --color-diff-addition-tint into
		// added lines.
		tint := ui.Hex(palette.Surface).Mix(ui.Hex(additionTint), 0.12)
		th.AddedLine, th.AddedNumber = tint, tint
	}
	return th
}
