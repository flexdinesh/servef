// Package compare composes labeled screenshots side by side at one scale
// (web | native), with an optional heat map of the pixels that differ.
package compare

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Panel is a labeled image.
type Panel struct {
	Label string
	Image image.Image
}

const (
	gap       = 16
	labelBar  = 32
	textScale = 2
)

var (
	background = color.RGBA{0x2b, 0x2d, 0x31, 0xff}
	labelColor = color.RGBA{0xf2, 0xf3, 0xf5, 0xff}
)

// Compose places panels left to right, each under its label, unscaled, so
// one DIP of a native shot sits beside one CSS pixel of a web shot taken
// at the same viewport.
func Compose(panels []Panel) *image.RGBA {
	width, height := gap, 0
	for _, p := range panels {
		b := p.Image.Bounds()
		width += b.Dx() + gap
		height = max(height, b.Dy())
	}
	out := image.NewRGBA(image.Rect(0, 0, width, height+labelBar+2*gap))
	draw.Draw(out, out.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	x := gap
	for _, p := range panels {
		b := p.Image.Bounds()
		drawLabel(out, p.Label, x, gap)
		draw.Draw(out, image.Rect(x, gap+labelBar, x+b.Dx(), gap+labelBar+b.Dy()), p.Image, b.Min, draw.Src)
		x += b.Dx() + gap
	}
	return out
}

// drawLabel writes s at (x, y) in basicfont, scaled up for legibility.
func drawLabel(dst *image.RGBA, s string, x, y int) {
	face := basicfont.Face7x13
	small := image.NewRGBA(image.Rect(0, 0, font.MeasureString(face, s).Ceil()+2, 14))
	d := font.Drawer{Dst: small, Src: image.NewUniform(labelColor), Face: face, Dot: fixed.P(1, 11)}
	d.DrawString(s)
	b := small.Bounds()
	for sy := 0; sy < b.Dy(); sy++ {
		for sx := 0; sx < b.Dx(); sx++ {
			c := small.RGBAAt(sx, sy)
			if c.A == 0 {
				continue
			}
			for dy := range textScale {
				for dx := range textScale {
					dst.SetRGBA(x+sx*textScale+dx, y+sy*textScale+dy, c)
				}
			}
		}
	}
}

// Threshold is the per-channel difference (0-255) a pixel must pass to
// count as changed.
const Threshold = 24

// Diff returns a heat map of where a and b differ (the overlap of their
// sizes; the rest counts as changed) over b dimmed, and the fraction of
// changed pixels.
func Diff(a, b image.Image) (*image.RGBA, float64) {
	ab, bb := a.Bounds(), b.Bounds()
	w, h := max(ab.Dx(), bb.Dx()), max(ab.Dy(), bb.Dy())
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	changed := 0
	for y := range h {
		for x := range w {
			inA := x < ab.Dx() && y < ab.Dy()
			inB := x < bb.Dx() && y < bb.Dy()
			if !inA || !inB {
				out.SetRGBA(x, y, color.RGBA{0xff, 0x00, 0xff, 0xff})
				changed++
				continue
			}
			ar, ag, abl := rgb(a.At(ab.Min.X+x, ab.Min.Y+y))
			br, bg, bbl := rgb(b.At(bb.Min.X+x, bb.Min.Y+y))
			d := max(absDiff(ar, br), absDiff(ag, bg), absDiff(abl, bbl))
			gray := uint8((299*int(br) + 587*int(bg) + 114*int(bbl)) / 1000 / 3)
			if d < Threshold {
				out.SetRGBA(x, y, color.RGBA{gray, gray, gray, 0xff})
				continue
			}
			changed++
			heat := 0.4 + 0.6*float64(d)/255
			out.SetRGBA(x, y, color.RGBA{uint8(math.Min(255, float64(gray)+heat*255)), gray / 2, gray / 2, 0xff})
		}
	}
	if w*h == 0 {
		return out, 0
	}
	return out, float64(changed) / float64(w*h)
}

func rgb(c color.Color) (uint8, uint8, uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
