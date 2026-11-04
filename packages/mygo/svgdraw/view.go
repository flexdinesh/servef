package svgdraw

import "github.com/egoist/mygo/ui"

// View shows d inline, as an img with max-width 100% and height auto: at
// its own size, scaled down to the width it gets, keeping its aspect
// ratio. Label it for assistive technology.
func View(c *ui.Context, d *Doc, s *Style) ui.Element {
	w, h := d.Size()
	box := ui.Box(c)
	if w <= 0 || h <= 0 {
		return box
	}
	return box.Width(w).MaxWidthPercent(100).AspectRatio(w / h).Role(ui.RoleImage).Draw(func(p *ui.Painter, r ui.Rect) {
		d.Draw(p, r, s)
	})
}
