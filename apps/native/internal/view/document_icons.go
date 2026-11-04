package view

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// drawIcon draws a small line icon in r's middle, in color.
type drawIcon func(p *ui.Painter, r ui.Rect, color ui.Color)

const iconStroke = 1.5

// iconButton is a square icon button named by label, its tooltip too.
func iconButton(c *ui.Context, label string, icon drawIcon) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Key(label).Label(label).Tooltip(label).Size(24, 24).Radius(4).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		color := t.TextMuted
		if b.Hovered() {
			color = t.Text
		}
		icon(p, r, color)
	})
	return b
}

func middle(r ui.Rect) (x, y float32) { return r.X + r.W/2, r.Y + r.H/2 }

func iconBack(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	var path ui.Path
	path.MoveTo(x+2, y-5).LineTo(x-3, y).LineTo(x+2, y+5)
	p.StrokePath(&path, iconStroke, color)
}

func iconForward(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	var path ui.Path
	path.MoveTo(x-2, y-5).LineTo(x+3, y).LineTo(x-2, y+5)
	p.StrokePath(&path, iconStroke, color)
}

func iconCopy(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	var back, front ui.Path
	back.MoveTo(x-2, y-3).LineTo(x-2, y-6).LineTo(x+5, y-6).LineTo(x+5, y+2).LineTo(x+2, y+2)
	front.MoveTo(x-5, y-2).LineTo(x+2, y-2).LineTo(x+2, y+6).LineTo(x-5, y+6).Close()
	p.StrokePath(&back, 1.2, color)
	p.StrokePath(&front, 1.2, color)
}

func iconCheck(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	var path ui.Path
	path.MoveTo(x-5, y).LineTo(x-1.5, y+4).LineTo(x+5, y-4)
	p.StrokePath(&path, iconStroke, color)
}

func iconExpand(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	var path ui.Path
	path.MoveTo(x+1, y-6).LineTo(x+6, y-6).LineTo(x+6, y-1)
	path.MoveTo(x+6, y-6).LineTo(x-6, y+6)
	path.MoveTo(x-1, y+6).LineTo(x-6, y+6).LineTo(x-6, y+1)
	p.StrokePath(&path, iconStroke, color)
}

func iconClose(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	var path ui.Path
	path.MoveTo(x-5, y-5).LineTo(x+5, y+5)
	path.MoveTo(x+5, y-5).LineTo(x-5, y+5)
	p.StrokePath(&path, iconStroke, color)
}

// circle adds a circle of radius around x, y to path.
func circle(path *ui.Path, x, y, radius float32, from, to float64) {
	const steps = 24
	for i := 0; i <= steps; i++ {
		angle := from + (to-from)*float64(i)/steps
		px, py := x+radius*float32(math.Cos(angle)), y+radius*float32(math.Sin(angle))
		if i == 0 {
			path.MoveTo(px, py)
		} else {
			path.LineTo(px, py)
		}
	}
}

func lens(p *ui.Painter, x, y float32, color ui.Color) {
	var path ui.Path
	circle(&path, x-1, y-1, 5, 0, 2*math.Pi)
	path.MoveTo(x+3, y+3).LineTo(x+6, y+6)
	p.StrokePath(&path, iconStroke, color)
}

func iconZoomIn(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	lens(p, x, y, color)
	var path ui.Path
	path.MoveTo(x-3.5, y-1).LineTo(x+1.5, y-1)
	path.MoveTo(x-1, y-3.5).LineTo(x-1, y+1.5)
	p.StrokePath(&path, iconStroke, color)
}

func iconZoomOut(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	lens(p, x, y, color)
	var path ui.Path
	path.MoveTo(x-3.5, y-1).LineTo(x+1.5, y-1)
	p.StrokePath(&path, iconStroke, color)
}

func iconReset(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	var path ui.Path
	circle(&path, x, y, 5.5, -math.Pi*0.9, math.Pi*0.9)
	path.MoveTo(x-5.5, y-6).LineTo(x-5.5, y-1.5).LineTo(x-1, y-1.5)
	p.StrokePath(&path, iconStroke, color)
}

func iconPlus(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	var path ui.Path
	path.MoveTo(x-5, y).LineTo(x+5, y)
	path.MoveTo(x, y-5).LineTo(x, y+5)
	p.StrokePath(&path, iconStroke, color)
}

func iconMinus(p *ui.Painter, r ui.Rect, color ui.Color) {
	x, y := middle(r)
	var path ui.Path
	path.MoveTo(x-5, y).LineTo(x+5, y)
	p.StrokePath(&path, iconStroke, color)
}
