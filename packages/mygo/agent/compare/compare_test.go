package compare_test

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/flexdinesh/servef/packages/mygo/agent/compare"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return img
}

func TestComposeKeepsScaleAndLabels(t *testing.T) {
	red := solid(100, 60, color.RGBA{255, 0, 0, 255})
	blue := solid(80, 90, color.RGBA{0, 0, 255, 255})
	out := compare.Compose([]compare.Panel{{Label: "web", Image: red}, {Label: "native", Image: blue}})
	if out.Bounds().Dx() != 16+100+16+80+16 || out.Bounds().Dy() != 90+32+32 {
		t.Fatalf("size %v", out.Bounds())
	}
	// Panels sit unscaled under their labels.
	if out.RGBAAt(16+50, 16+32+30) != red.RGBAAt(0, 0) || out.RGBAAt(16+100+16+40, 16+32+80) != blue.RGBAAt(0, 0) {
		t.Fatal("panels not placed 1:1")
	}
	lit := 0
	for y := 16; y < 16+28; y++ {
		for x := 16; x < 16+60; x++ {
			if c := out.RGBAAt(x, y); c.R > 0xe0 && c.G > 0xe0 {
				lit++
			}
		}
	}
	if lit == 0 {
		t.Fatal("no label drawn")
	}
}

func TestDiffCountsChangedPixels(t *testing.T) {
	a := solid(10, 10, color.RGBA{10, 10, 10, 255})
	b := solid(10, 10, color.RGBA{10, 10, 10, 255})
	if _, ratio := compare.Diff(a, b); ratio != 0 {
		t.Fatalf("same images differ by %v", ratio)
	}
	for x := range 5 {
		for y := range 2 {
			b.SetRGBA(x, y, color.RGBA{200, 200, 200, 255})
		}
	}
	heat, ratio := compare.Diff(a, b)
	if ratio != 0.1 {
		t.Fatalf("ratio %v", ratio)
	}
	if c := heat.RGBAAt(0, 0); c.R <= c.G {
		t.Fatalf("changed pixel not hot: %v", c)
	}
	if _, ratio := compare.Diff(a, solid(10, 20, color.RGBA{10, 10, 10, 255})); ratio != 0.5 {
		t.Fatalf("size mismatch ratio %v", ratio)
	}
}
