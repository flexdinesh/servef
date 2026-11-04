package view

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/flexdinesh/servef/apps/native/internal/highlight"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
	"github.com/flexdinesh/servef/packages/mygo/ondine"
	"github.com/flexdinesh/servef/packages/mygo/svgdraw"
)

// Messages as apps/web/src/features/documents (diagram.ts, MermaidBlock.tsx).
const (
	mermaidRenderError = "Could not render Mermaid diagram. Check its source."
	mermaidLoadError   = "Could not load Mermaid. Reload to try again."
)

// resizeDelay is how long a diagram's frame keeps a new width before the
// diagram renders for it; until then the last render scales to fit.
const resizeDelay = 250 * time.Millisecond

// fenceKey is a fence's code in a language.
type fenceKey struct{ language, code string }

// fence is a fence's highlighted lines; nil lines show plain.
type fence struct {
	lines [][]highlight.Span
	ready bool
}

// rendered is a finished Mermaid render: the drawing, or the message its
// source shows with.
type rendered struct {
	doc    *svgdraw.Doc
	err    string
	failed bool // Mermaid did not load
}

// diagram is a Mermaid source's renders: the last finished one, for the
// width it was laid out for, and the one in flight.
type diagram struct {
	shown    *rendered
	width    int
	inflight bool
	// want is a new frame width, seen since wantSince.
	want      int
	wantSince time.Time
	// measured asked for the frame that measures the width.
	measured bool
}

// markdownBlocks keeps what the open document's code blocks computed off
// the UI thread, one entry per fence or diagram source: mygo builds every
// block each frame, so an entry must outlive the frame for its result to
// show. It starts over for another document.
type markdownBlocks struct {
	document string
	fences   map[fenceKey]*fence
	diagrams map[string]*diagram
}

// use keeps the blocks of document, dropping another document's.
func (b *markdownBlocks) use(document string) {
	if b.document != document || b.fences == nil {
		b.document = document
		b.fences, b.diagrams = map[fenceKey]*fence{}, map[string]*diagram{}
	}
}

// fence returns key's entry; known is false when it was just made, and
// its highlighting is to start.
func (b *markdownBlocks) fence(key fenceKey) (f *fence, known bool) {
	if f, ok := b.fences[key]; ok {
		return f, true
	}
	f = &fence{}
	b.fences[key] = f
	return f, false
}

func (b *markdownBlocks) diagram(source string) *diagram {
	d, ok := b.diagrams[source]
	if !ok {
		d = &diagram{}
		b.diagrams[source] = d
	}
	return d
}

// codeBlock builds a fence as the web app's preview does: highlighted with
// Highlights once it is, labeled with its language only then, and plain
// for languages Highlights does not know.
func (a *App) codeBlock(c *ui.Context, b *markdown.CodeBlockView, th *markdown.Theme) {
	code := strings.TrimSuffix(b.Code(), "\n")
	var lines [][]highlight.Span
	if highlight.Supported(b.Language) && !b.Open {
		f, known := a.blocks.fence(fenceKey{b.Language, code})
		if !known {
			language := b.Language
			a.run.Go(func() func() {
				lines := highlight.Lines(code, language)
				return func() { f.lines, f.ready = lines, true }
			})
		}
		lines = f.lines
	}
	label := ""
	if lines != nil {
		label = b.Language
	}
	codeBox(c, th, label, func() {
		ui.Spacer(c)
		b.CopyButton(c)
	}, func() {
		codeText(c, th, code, lines)
	}).Label(codeLabel(label))
}

func codeLabel(language string) string {
	if language == "" {
		return "Code"
	}
	return "Code, " + language
}

// codeBox is a code block's box, as markdown.DefaultCodeBlock draws it:
// a head row over the code.
func codeBox(c *ui.Context, th *markdown.Theme, label string, head, body func()) ui.Element {
	box := ui.Column(c).Direction(ui.LeftToRight).Background(th.CodeBlockBackground).Border(1, th.CodeBlockBorder).Radius(th.CodeBlockRadius).AlignSelf(ui.Stretch).Clip().Role(ui.RoleGroup)
	return box.Children(func() {
		if head != nil {
			ui.Row(c).AlignItems(ui.Center).Padding(em(th, 0.35), em(th, 0.4), 0, em(th, th.CodeBlockPadding)).Children(func() {
				if label != "" {
					ui.Text(c, label).FontSize(em(th, 0.8)).TextColor(th.Muted).Unselectable()
				}
				head()
			})
		}
		body()
	})
}

func em(th *markdown.Theme, v float32) float32 { return v * th.FontSize }

// codeText is code in the code font, its lines colored by spans (plain
// when nil), scrolling sideways.
func codeText(c *ui.Context, th *markdown.Theme, code string, lines [][]highlight.Span) {
	pad := em(th, th.CodeBlockPadding)
	ui.ScrollHorizontal(c).Direction(ui.LeftToRight).AlignSelf(ui.Stretch).Font(th.CodeFont).FontSize(em(th, th.CodeSize)).LineHeight(th.CodeLineHeight).Children(func() {
		ui.Column(c).Padding(em(th, 0.25), pad, pad, pad).Shrink(0).Children(func() {
			ui.RichText(c, codeSpans(th.CodeColors, code, lines)...).NoWrap().Selectable()
		})
	})
}

// codeSpans colors each line of code by its spans.
func codeSpans(colors *editor.Theme, code string, lines [][]highlight.Span) []ui.Span {
	var spans []ui.Span
	for i, line := range strings.Split(code, "\n") {
		if i > 0 {
			spans = append(spans, ui.Span{Text: "\n"})
		}
		start := 0
		if i < len(lines) {
			for _, span := range lines[i] {
				end := min(span.End, len(line))
				if end <= start {
					continue
				}
				kind := tokenKinds[span.Category]
				s := ui.Span{Text: line[start:end], Color: colors.TokenColor(kind)}
				if style := colors.Tokens[kind]; style.Bold {
					s.Weight = 600
				} else if style.Italic {
					s.Italic = true
				}
				spans = append(spans, s)
				start = end
			}
		}
		if start < len(line) {
			spans = append(spans, ui.Span{Text: line[start:], Color: colors.Text})
		}
	}
	if len(spans) == 0 {
		spans = append(spans, ui.Span{Text: " "})
	}
	return spans
}

// mermaidBlock draws a Mermaid fence as the web app's OndineMermaidDiagram
// does: rendered by Ondine for the frame's width (padding 16), centered on
// a bordered surface, colored by the theme's --ondine-* variables (applied
// when drawing, so a theme switch renders nothing anew). Until it renders
// the source shows; a source Ondine rejects shows with Ondine's diagnostic,
// and one that fails otherwise (or fails to draw) with a generic message.
// A new frame width renders again once it has held for resizeDelay; the
// last render scales down meanwhile.
func (a *App) mermaidBlock(c *ui.Context, b *markdown.CodeBlockView, th *markdown.Theme, palette theme.Palette) {
	source := b.Code()
	frame := ui.Column(c).AlignSelf(ui.Stretch).Gap(em(th, 0.5))
	width := int(math.Round(float64(frame.LayoutBounds().W)))
	frame.Children(func() {
		if b.Open {
			mermaidSource(c, th, source, "")
			return
		}
		d := a.blocks.diagram(source)
		if width <= 0 {
			// The frame's width comes with the next frame.
			if !d.measured {
				d.measured = true
				c.Invalidate()
			}
			if d.shown == nil {
				mermaidSource(c, th, source, "")
				return
			}
		} else {
			a.scheduleMermaid(c, source, width, d)
		}
		shown := d.shown
		switch {
		case shown == nil:
			mermaidSource(c, th, source, "")
		case shown.failed:
			mermaidSource(c, th, source, mermaidLoadError)
			if ui.Button(c, "Reload diagrams").Clicked() {
				a.blocks.diagrams = map[string]*diagram{}
			}
		case shown.err != "":
			mermaidSource(c, th, source, shown.err)
		case shown.doc.Err() != nil:
			mermaidSource(c, th, source, mermaidRenderError)
		default:
			style := &svgdraw.Style{Vars: palette.Ondine(), Font: "monospace", Color: ui.Hex(palette.TextPrimary)}
			// mermaid.css: margin var(--space-5) 0, radius var(--radius-md).
			ui.Column(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).MarginY(24).Border(1, ui.Hex(palette.Border)).Radius(4).Background(ui.Hex(palette.Surface)).Clip().Children(func() {
				svgdraw.View(c, shown.doc, style).Label("Mermaid diagram")
			})
		}
	})
}

// scheduleMermaid starts d's render for width when it has none, or when
// the frame has held a new width for resizeDelay; one render at a time.
func (a *App) scheduleMermaid(c *ui.Context, source string, width int, d *diagram) {
	switch {
	case d.inflight:
	case d.shown == nil:
		a.renderMermaid(source, width, d)
	case d.shown.failed || width == d.width:
		d.want = 0
	case d.want != width:
		d.want, d.wantSince = width, c.Now()
		c.After(resizeDelay)
	case c.Now().Sub(d.wantSince) >= resizeDelay:
		a.renderMermaid(source, width, d)
	default:
		c.After(resizeDelay - c.Now().Sub(d.wantSince))
	}
}

// renderMermaid renders source for width off the UI thread into d.
func (a *App) renderMermaid(source string, width int, d *diagram) {
	d.inflight = true
	a.run.Go(func() func() {
		var result rendered
		if err := ondine.Warm(); err != nil {
			result.failed = true
		} else if out, err := ondine.Render(source, ondine.Options{IDPrefix: "mermaid", MaxWidth: float64(width), Padding: 16}); err != nil {
			result.err = mermaidRenderError
			var rejected *ondine.Error
			if errors.As(err, &rejected) && rejected.Message != "" {
				result.err = rejected.Message
			}
		} else if doc, err := svgdraw.Parse(out.SVG); err != nil {
			result.err = mermaidRenderError
		} else {
			result.doc = doc
		}
		return func() {
			d.shown, d.width, d.inflight, d.want = &result, width, false, 0
		}
	})
}

// mermaidSource is MermaidSource: the source as plain code, and the
// message below it in the destructive color.
func mermaidSource(c *ui.Context, th *markdown.Theme, source, message string) {
	codeBox(c, th, "", nil, func() {
		codeText(c, th, strings.TrimSuffix(source, "\n"), nil)
	}).Label(codeLabel("mermaid"))
	if message != "" {
		ui.Text(c, message).Font(th.CodeFont).FontSize(em(th, 0.875)).TextColor(c.Theme().Danger).Wrap().Selectable().Role(ui.RoleStatus)
	}
}
