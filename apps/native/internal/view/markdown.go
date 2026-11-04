package view

import (
	"errors"
	"math"
	"strings"

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

// cacheLimit bounds the fences and diagrams a view keeps; past it the
// cache starts over.
const cacheLimit = 64

// fenceKey is a fence's code in a language.
type fenceKey struct{ language, code string }

// fence is a fence's highlighted lines; nil lines show plain.
type fence struct {
	lines [][]highlight.Span
	ready bool
}

// diagramKey is a Mermaid source laid out for a width.
type diagramKey struct {
	source string
	width  int
}

// diagram is a rendered Mermaid source: the drawing, or the message its
// source shows with.
type diagram struct {
	doc    *svgdraw.Doc
	err    string
	failed bool // Mermaid did not load
	ready  bool
}

// markdownBlocks keeps what Markdown code blocks computed off the UI
// thread.
type markdownBlocks struct {
	fences   map[fenceKey]*fence
	diagrams map[diagramKey]*diagram
	// shown is each source's last ready width, shown while another width
	// renders.
	shown map[string]int
}

func (b *markdownBlocks) fence(key fenceKey) (*fence, bool) {
	if f, ok := b.fences[key]; ok {
		return f, true
	}
	if b.fences == nil || len(b.fences) >= cacheLimit {
		b.fences = map[fenceKey]*fence{}
	}
	f := &fence{}
	b.fences[key] = f
	return f, false
}

func (b *markdownBlocks) diagram(key diagramKey) (*diagram, bool) {
	if d, ok := b.diagrams[key]; ok {
		return d, true
	}
	if b.diagrams == nil || len(b.diagrams) >= cacheLimit {
		b.diagrams, b.shown = map[diagramKey]*diagram{}, map[string]int{}
	}
	d := &diagram{}
	b.diagrams[key] = d
	return d, false
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
// a bordered surface, colored by the theme's --ondine-* variables. Until it
// renders the source shows; a source Ondine rejects shows with Ondine's
// diagnostic, and one that fails otherwise with a generic message.
func (a *App) mermaidBlock(c *ui.Context, b *markdown.CodeBlockView, th *markdown.Theme, palette theme.Palette) {
	source := b.Code()
	frame := ui.Column(c).AlignSelf(ui.Stretch).Gap(em(th, 0.5))
	width := int(math.Round(float64(frame.LayoutBounds().W)))
	frame.Children(func() {
		if b.Open {
			mermaidSource(c, th, source, "")
			return
		}
		if width <= 0 {
			// The frame's width comes with the next frame.
			mermaidSource(c, th, source, "")
			c.Invalidate()
			return
		}
		d, known := a.blocks.diagram(diagramKey{source, width})
		if !known {
			a.renderMermaid(source, width, d)
		}
		if !d.ready {
			if last, ok := a.blocks.shown[source]; ok {
				if shown, ok := a.blocks.diagrams[diagramKey{source, last}]; ok && shown.ready {
					d = shown
				}
			}
		}
		switch {
		case !d.ready:
			mermaidSource(c, th, source, "")
		case d.failed:
			mermaidSource(c, th, source, mermaidLoadError)
			if ui.Button(c, "Reload diagrams").Clicked() {
				a.blocks.diagrams, a.blocks.shown = nil, nil
			}
		case d.err != "":
			mermaidSource(c, th, source, d.err)
		default:
			style := &svgdraw.Style{Vars: palette.Ondine(), Font: "monospace", Color: ui.Hex(palette.TextPrimary)}
			// mermaid.css: margin var(--space-5) 0, radius var(--radius-md).
			ui.Column(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).MarginY(24).Border(1, ui.Hex(palette.Border)).Radius(4).Background(ui.Hex(palette.Surface)).Clip().Children(func() {
				svgdraw.View(c, d.doc, style).Label("Mermaid diagram")
			})
		}
	})
}

// renderMermaid renders source for width off the UI thread into d.
func (a *App) renderMermaid(source string, width int, d *diagram) {
	a.run.Go(func() func() {
		var result diagram
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
		result.ready = true
		return func() {
			*d = result
			if d.doc != nil && a.blocks.shown != nil {
				a.blocks.shown[source] = width
			}
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
