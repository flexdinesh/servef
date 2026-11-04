package docview

import (
	"math"
	"testing"
)

// image-zoom.test.ts: fit never enlarges and shrinks to the tighter axis.
func TestFitScale(t *testing.T) {
	for _, c := range []struct {
		image, viewport Size
		want            float64
	}{
		{Size{200, 100}, Size{800, 600}, 1},
		{Size{1920, 1080}, Size{960, 1080}, 0.5},
		{Size{1000, 2000}, Size{1000, 500}, 0.25},
		{Size{0, 0}, Size{800, 600}, 1},
	} {
		if got := FitScale(c.image, c.viewport); got != c.want {
			t.Errorf("FitScale(%v, %v) = %v", c.image, c.viewport, got)
		}
	}
}

// Zoom steps by 1.25 within 10% to 800%.
func TestZoomSteps(t *testing.T) {
	for _, c := range []struct{ got, want float64 }{
		{ZoomIn(1), 1.25},
		{ZoomOut(1), 0.8},
		{ZoomIn(0.8), 1},
		{ZoomIn(7), MaximumZoom},
		{ZoomIn(MaximumZoom), MaximumZoom},
		{ZoomOut(0.11), MinimumZoom},
		{ZoomOut(MinimumZoom), MinimumZoom},
		{ZoomOut(0.05), 0.05},
		{ZoomIn(0.05), MinimumZoom},
	} {
		if c.got != c.want {
			t.Errorf("got %v, want %v", c.got, c.want)
		}
	}
	if ZoomPercent(0.6409) != 64 {
		t.Error("percent")
	}
}

func TestImageViewer(t *testing.T) {
	var v ImageViewer
	v.ZoomIn()
	if v.Zoom != 0 || v.CanZoomIn() || v.CanZoomOut() || v.Size() != (Size{}) {
		t.Fatalf("loading image takes no zoom: %+v", v)
	}
	v.Status, v.Image, v.Viewport = ImageReady, Size{1000, 500}, Size{500, 500}
	if !v.Fit() || v.Scale() != 0.5 || v.Percent() != 50 || v.Size() != (Size{500, 250}) {
		t.Fatalf("fit: %+v", v)
	}
	v.ZoomIn()
	if v.Fit() || v.Scale() != 0.625 {
		t.Fatalf("zoom in from fit: %+v", v)
	}
	v.ZoomOut()
	if v.Scale() != 0.5 {
		t.Fatalf("zoom out: %+v", v)
	}
	v.ActualSize()
	if v.Scale() != 1 || v.Size() != (Size{1000, 500}) {
		t.Fatalf("actual: %+v", v)
	}
	v.FitToView()
	if !v.Fit() {
		t.Fatalf("fit again: %+v", v)
	}
	v.Zoom = MaximumZoom
	if v.CanZoomIn() || !v.CanZoomOut() {
		t.Fatal("bounds")
	}
	tiny := ImageViewer{Status: ImageReady, Image: Size{1, 1}, Zoom: MinimumZoom}
	if tiny.Size() != (Size{1, 1}) {
		t.Fatalf("at least a pixel: %v", tiny.Size())
	}
}

// diagram.test.ts.
func TestClampDiagramZoom(t *testing.T) {
	if ClampDiagramZoom(100) != MaximumZoom || ClampDiagramZoom(0) != MinimumZoom || ClampDiagramZoom(math.NaN()) != 1 || ClampDiagramZoom(1.5) != 1.5 {
		t.Error("clamp")
	}
}

func TestFitDiagramView(t *testing.T) {
	if got := FitDiagramView(Size{800, 600}, Size{200, 100}); got != (DiagramView{300, 250, 1}) {
		t.Errorf("small diagrams keep natural size, centered: %+v", got)
	}
	if got := FitDiagramView(Size{400, 400}, Size{1600, 400}); got != (DiagramView{0, 150, 0.25}) {
		t.Errorf("large diagrams scale down: %+v", got)
	}
	if got := FitDiagramView(Size{0, 0}, Size{100, 100}); got.Zoom != 1 {
		t.Errorf("empty viewport: %+v", got)
	}
	if got := FitDiagramView(Size{100, 100}, Size{0, 0}); got.Zoom != 1 {
		t.Errorf("empty content: %+v", got)
	}
}

func TestDiagramZoomKeepsAnchorFixed(t *testing.T) {
	view := DiagramView{10, 20, 1}
	anchorX, anchorY := 110.0, 70.0
	zoomed := view.Zoomed(2, anchorX, anchorY)
	if zoomed.Zoom != 2 {
		t.Fatalf("zoom %v", zoomed.Zoom)
	}
	if (anchorX-zoomed.X)/zoomed.Zoom != (anchorX-view.X)/view.Zoom || (anchorY-zoomed.Y)/zoomed.Zoom != (anchorY-view.Y)/view.Zoom {
		t.Fatalf("anchor moved: %+v", zoomed)
	}
	bound := DiagramView{5, 5, MaximumZoom}
	if got := bound.Zoomed(2, 50, 50); got != bound {
		t.Fatalf("zoom at a bound leaves the view: %+v", got)
	}
}

func TestDiagramPan(t *testing.T) {
	if got := (DiagramView{1, 2, 3}).Panned(10, -5); got != (DiagramView{11, -3, 3}) {
		t.Errorf("%+v", got)
	}
}
