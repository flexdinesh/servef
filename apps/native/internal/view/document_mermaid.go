package view

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
	"github.com/flexdinesh/servef/packages/mygo/ondine"
	"github.com/flexdinesh/servef/packages/mygo/svgdraw"
)

// expandedDiagram is OndineMermaidDiagram's dialog: the source rendered at
// its full width, with zoom (buttons, Ctrl or Cmd with the wheel) and drag
// pan; inline diagrams do not zoom (decision 0025). It holds its render
// only while open.
type expandedDiagram struct {
	open bool
	doc  *svgdraw.Doc
	// view is nil until the viewer has a size to fit.
	view     *docview.DiagramView
	viewport ui.Rect
	dragging bool
	lastX    float32
	lastY    float32
}

// expandDiagram renders source for the dialog off the UI thread; a render
// that fails shows no dialog, as on the web.
func (a *App) expandDiagram(source string) {
	expanded := &expandedDiagram{open: true}
	a.doc.diagram = expanded
	a.run.Go(func() func() {
		auto := false
		out, err := ondine.Render(source, ondine.Options{AutoDirection: &auto, IDPrefix: "mermaid-expanded", Padding: 16})
		var doc *svgdraw.Doc
		if err == nil {
			doc, err = svgdraw.Parse(out.SVG)
		}
		return func() {
			if err != nil {
				expanded.open = false
				return
			}
			expanded.doc = doc
		}
	})
}

func percentLabel(percent int) string { return fmt.Sprintf("%d%%", percent) }

// diagramDialog is the expanded diagram's dialog.
func (a *App) diagramDialog(c *ui.Context, palette theme.Palette) {
	e := a.doc.diagram
	if e == nil {
		return
	}
	if !e.open {
		a.doc.diagram = nil
		return
	}
	if e.doc == nil {
		return
	}
	t := c.Theme()
	w, h := c.Size()
	ui.DialogBase(c, &e.open, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.4))
		// The viewer variant of dialog-shell.css: the window less --space-6.
		panel.Size(max(w-32, 0), max(h-32, 0)).Radius(8).Background(ui.Hex(palette.Surface)).Border(1, t.Border).Clip().Label("Mermaid diagram")
		panel.Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.3))
		a.diagramHeader(c, e)
		a.diagramViewer(c, e, palette)
	})
}

func (a *App) diagramHeader(c *ui.Context, e *expandedDiagram) {
	t := c.Theme()
	ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, "Mermaid diagram").Bold().SingleLine()
		ui.Spacer(c)
		if e.view != nil {
			ui.Text(c, percentLabel(docview.ZoomPercent(e.view.Zoom))).FontSize(12).TextColor(t.TextMuted).Role(ui.RoleStatus)
		}
		if iconButton(c, "Zoom out", iconZoomOut).Clicked() {
			e.zoomBy(1 / docview.DiagramZoomStep)
		}
		if iconButton(c, "Zoom in", iconZoomIn).Clicked() {
			e.zoomBy(docview.DiagramZoomStep)
		}
		if iconButton(c, "Reset zoom", iconReset).Clicked() {
			e.fit()
		}
		if iconButton(c, "Close diagram", iconClose).Clicked() {
			e.open = false
		}
	})
}

// fit shows the diagram at natural size, scaled down to the viewer.
func (e *expandedDiagram) fit() {
	if e.viewport.W <= 0 || e.viewport.H <= 0 {
		return
	}
	w, h := e.doc.Size()
	view := docview.FitDiagramView(docview.Size{Width: float64(e.viewport.W), Height: float64(e.viewport.H)}, docview.Size{Width: float64(w), Height: float64(h)})
	e.view = &view
}

// zoomBy zooms around the viewer's middle.
func (e *expandedDiagram) zoomBy(factor float64) {
	e.zoomAt(factor, e.viewport.W/2, e.viewport.H/2)
}

func (e *expandedDiagram) zoomAt(factor float64, x, y float32) {
	if e.view != nil {
		view := e.view.Zoomed(factor, float64(x), float64(y))
		e.view = &view
	}
}

func (e *expandedDiagram) pan(dx, dy float32) {
	if e.view != nil {
		view := e.view.Panned(float64(dx), float64(dy))
		e.view = &view
	}
}

// input is the viewer's wheel and drag, as MermaidDiagramDialog's: the
// wheel pans; with Ctrl or Cmd, or a pinch, it zooms around the pointer.
func (e *expandedDiagram) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputScroll:
		if ev.Mods&(ui.Ctrl|ui.Super) != 0 {
			factor := 1 / docview.DiagramZoomStep
			if ev.DY < 0 {
				factor = docview.DiagramZoomStep
			}
			e.zoomAt(factor, ev.X, ev.Y)
		} else {
			e.pan(-ev.DX, -ev.DY)
		}
		return true
	case ui.InputZoom:
		e.zoomAt(float64(ev.Scale), ev.X, ev.Y)
		return true
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		e.dragging, e.lastX, e.lastY = true, ev.X, ev.Y
		return true
	case ui.InputPointerMove:
		if !e.dragging {
			return false
		}
		e.pan(ev.X-e.lastX, ev.Y-e.lastY)
		e.lastX, e.lastY = ev.X, ev.Y
		return true
	case ui.InputPointerUp:
		e.dragging = false
		return true
	}
	return false
}

// diagramViewer draws the diagram under the view's pan and zoom. The
// canvas keeps the diagram's own surface inside the raised dialog.
func (a *App) diagramViewer(c *ui.Context, e *expandedDiagram, palette theme.Palette) {
	viewer := ui.Box(c).Key("mermaid-viewer").Grow(1).FillWidth().Clip().Background(ui.Hex(palette.Surface)).Label("Mermaid diagram canvas").Cursor(ui.CursorGrab)
	viewer.HandleInput(e.input)
	e.viewport = viewer.LayoutBounds()
	if e.view == nil {
		// Either way another frame follows: the header above shows the zoom.
		e.fit()
		c.Invalidate()
	}
	style := &svgdraw.Style{Vars: palette.Ondine(), Font: "monospace", Color: ui.Hex(palette.TextPrimary)}
	viewer.Draw(func(p *ui.Painter, r ui.Rect) {
		if e.view == nil {
			return
		}
		w, h := e.doc.Size()
		zoom := float32(e.view.Zoom)
		_ = e.doc.Draw(p, ui.Rect{X: r.X + float32(e.view.X), Y: r.Y + float32(e.view.Y), W: w * zoom, H: h * zoom}, style)
	})
}
