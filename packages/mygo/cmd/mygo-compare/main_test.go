package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePNG(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDirComposesShotsWithBothSides(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "web", "a.png"), 40, 30, color.RGBA{255, 255, 255, 255})
	writePNG(t, filepath.Join(dir, "a.png"), 40, 30, color.RGBA{0, 0, 0, 255})
	writePNG(t, filepath.Join(dir, "web", "lonely.png"), 40, 30, color.RGBA{0, 0, 0, 255})
	var out bytes.Buffer
	if err := run([]string{"dir", "-diff", dir}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "100.0% of pixels differ") || strings.Contains(out.String(), "lonely") {
		t.Fatalf("out %s", out.String())
	}
	f, err := os.Open(filepath.Join(dir, "compare", "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil || img.Bounds().Dx() != 16*4+40*3 {
		t.Fatalf("composite %v %v", img.Bounds(), err)
	}
}

func TestComposeNeedsTwoImages(t *testing.T) {
	if err := run([]string{"compose", "-out", "x.png", "only.png"}, &bytes.Buffer{}); err == nil {
		t.Fatal("composed one image")
	}
}
