package view

import (
	"context"
	"path"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
)

// checkerSize is --checker-size: one square of the image frame's tiles.
const checkerSize = 16

// imageView shows an image file, read from the server's image endpoint,
// as ImageDocument: fit, or the zoom chosen, on a checkerboard.
func (a *App) imageView(c *ui.Context, palette theme.Palette) {
	m := a.Model
	d := &a.doc
	key := m.OpenPlaceKey()
	if d.imageKey != key {
		d.imageKey = key
		d.viewer, d.focusImage = docview.ImageViewer{}, true
		d.imageVersion = -1
	}
	if d.imageVersion != m.DocumentVersion {
		d.imageVersion, d.image = m.DocumentVersion, nil
		d.viewer.Status = docview.ImageLoading
		version, read := m.DocumentVersion, m.ImageReader()
		a.run.Go(func() func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			data, err := read(ctx, "")
			var source ui.ImageSource
			if err == nil {
				source, err = ui.DecodeImage(data)
			}
			return func() {
				if d.imageVersion != version {
					return
				}
				d.image = source
				if err != nil {
					d.viewer.Status = docview.ImageError
					return
				}
				w, h := ui.ImageSize(source)
				d.viewer.Status, d.viewer.Image = docview.ImageReady, docview.Size{Width: float64(w), Height: float64(h)}
			}
		})
	}
	if d.restoring {
		d.imageScroll.Y, d.restoring = float32(d.restore), false
	}
	d.scrolled = docview.Image
	const padding = 16
	scroll := ui.ScrollBoth(c).Grow(1).FillWidth().TrackScroll(&d.imageScroll).Focusable().FocusRing(false).Label(path.Base(m.Page.Selected) + " image")
	if d.focusImage {
		d.focusImage = false
		scroll.Focus()
	}
	a.imageKeys(scroll)
	bounds := scroll.LayoutBounds()
	viewport := docview.Size{Width: max(0, float64(bounds.W)-2*padding), Height: max(0, float64(bounds.H)-2*padding)}
	if d.viewer.Viewport != viewport {
		d.viewer.Viewport = viewport
		c.Invalidate()
	}
	scroll.Children(func() {
		ui.Column(c).MinWidth(bounds.W).MinHeight(bounds.H).Padding(padding).Center().Children(func() {
			switch d.viewer.Status {
			case docview.ImageError:
				ui.Text(c, "Can't display this image.").FontSize(14).TextColor(c.Theme().TextMuted).Role(ui.RoleStatus)
			case docview.ImageLoading:
				ui.Box(c).Size(256, 160).Background(ui.Hex(palette.SurfaceRecessed))
			default:
				size := d.viewer.Size()
				w, h := float32(size.Width), float32(size.Height)
				// The frame's outline stays outside the image's size.
				ui.Box(c).Size(w+2, h+2).Shrink(0).Border(1, ui.Hex(palette.Border)).Children(func() {
					ui.Box(c).Size(w, h).Draw(checkerboard(palette, bounds)).Children(func() {
						ui.Image(c, d.image).Size(w, h).Label(path.Base(m.Page.Selected))
					})
				})
			}
		})
	})
}

// imageKeys are ImageDocument's shortcuts while the image has the focus.
func (a *App) imageKeys(scroll ui.Element) {
	v := &a.doc.viewer
	if v.Status != docview.ImageReady {
		return
	}
	switch {
	case scroll.Shortcut(ui.Shift, ui.KeyEqual), scroll.Shortcut(0, ui.KeyEqual):
		v.ZoomIn()
	case scroll.Shortcut(0, ui.KeyMinus), scroll.Shortcut(ui.Shift, ui.KeyMinus):
		v.ZoomOut()
	case scroll.Shortcut(0, ui.Key0):
		v.FitToView()
	case scroll.Shortcut(0, ui.Key1):
		v.ActualSize()
	}
}

// checkerboard paints the frame's background as documents.css: the
// document's surface with recessed squares, only where the pane shows.
func checkerboard(palette theme.Palette, pane ui.Rect) func(p *ui.Painter, r ui.Rect) {
	surface, recessed := ui.Hex(palette.Surface), ui.Hex(palette.SurfaceRecessed)
	return func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, surface, 0)
		half := float32(checkerSize) / 2
		left, top := max(r.X, pane.X), max(r.Y, pane.Y)
		right, bottom := min(r.X+r.W, pane.X+pane.W), min(r.Y+r.H, pane.Y+pane.H)
		startX, startY := int((left-r.X)/half), int((top-r.Y)/half)
		for row := startY; r.Y+float32(row)*half < bottom; row++ {
			y := r.Y + float32(row)*half
			height := min(half, r.Y+r.H-y)
			// A tile's top right and bottom left squares are recessed.
			for col := startX; r.X+float32(col)*half < right; col++ {
				if (row+col)%2 == 0 {
					continue
				}
				x := r.X + float32(col)*half
				p.Fill(ui.Rect{X: x, Y: y, W: min(half, r.X+r.W-x), H: height}, recessed, 0)
			}
		}
	}
}

// imageZoom is ImageZoomControls: fit, out, the level (which resets to
// 100%), and in.
func (a *App) imageZoom(c *ui.Context) {
	t := c.Theme()
	v := &a.doc.viewer
	ready := v.Status == docview.ImageReady
	ui.Row(c).AlignItems(ui.Stretch).Padding(t.Space(0.5)).Gap(t.Space(0.5)).Radius(t.Radius).Background(t.Surface).Border(1, t.Border).Role(ui.RoleGroup).Label("Zoom").Children(func() {
		fit := ui.ButtonBase(c).Key("fit").Padding(t.Space(1), t.Space(2)).Radius(max(t.Radius-t.Space(0.5), 0)).Disabled(!ready).Tooltip("Fit to view (0)").Label("Fit to view").Cursor(ui.CursorPointer)
		if v.Fit() {
			fit.Background(t.SurfacePressed)
		}
		fit.Children(func() { ui.Text(c, "Fit").SingleLine() })
		if fit.Clicked() {
			v.FitToView()
		}
		if iconButton(c, "Zoom out", iconMinus).Disabled(!v.CanZoomOut()).Clicked() {
			v.ZoomOut()
		}
		level := ui.ButtonBase(c).Key("level").Padding(t.Space(1), t.Space(1)).MinWidth(52).Center().Radius(max(t.Radius-t.Space(0.5), 0)).Disabled(!ready).Tooltip("Actual size (1)").Label(percentLabel(v.Percent()) + ", reset to 100%").Cursor(ui.CursorPointer)
		level.Children(func() { ui.Text(c, percentLabel(v.Percent())).SingleLine() })
		if level.Clicked() {
			v.ActualSize()
		}
		if iconButton(c, "Zoom in", iconPlus).Disabled(!v.CanZoomIn()).Clicked() {
			v.ZoomIn()
		}
	})
}
