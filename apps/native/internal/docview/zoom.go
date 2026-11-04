package docview

import "math"

// Zoom bounds and step, as image-zoom.ts (decision 0005).
const (
	MinimumZoom = 0.1
	MaximumZoom = 8.0
	zoomFactor  = 1.25
)

// Size is an image's, a diagram's, or a viewport's size.
type Size struct{ Width, Height float64 }

func roundZoom(scale float64) float64 { return math.Round(scale*10_000) / 10_000 }

func clampZoom(scale float64) float64 {
	return math.Min(MaximumZoom, math.Max(MinimumZoom, roundZoom(scale)))
}

// FitScale fits image inside viewport without enlarging it.
func FitScale(image, viewport Size) float64 {
	if image.Width <= 0 || image.Height <= 0 || viewport.Width <= 0 || viewport.Height <= 0 {
		return 1
	}
	// Very large images may fit below the manual zoom floor.
	return math.Max(0.0001, roundZoom(math.Min(1, math.Min(viewport.Width/image.Width, viewport.Height/image.Height))))
}

// ZoomIn is the next step up from scale.
func ZoomIn(scale float64) float64 { return math.Max(scale, clampZoom(scale*zoomFactor)) }

// ZoomOut is the next step down from scale.
func ZoomOut(scale float64) float64 { return math.Min(scale, clampZoom(scale/zoomFactor)) }

// ZoomPercent is scale as a whole percentage.
func ZoomPercent(scale float64) int { return int(math.Round(scale * 100)) }

// ImageStatus is where an image document stands.
type ImageStatus int

const (
	ImageLoading ImageStatus = iota
	ImageReady
	ImageError
)

// ImageViewer is use-image-viewer.ts: fit and zoom of the open image.
type ImageViewer struct {
	Status   ImageStatus
	Image    Size
	Viewport Size
	// Zoom is the manual scale; 0 fits the viewport.
	Zoom float64
}

// Scale is the scale the image shows at.
func (v ImageViewer) Scale() float64 {
	if v.Zoom != 0 {
		return v.Zoom
	}
	return FitScale(v.Image, v.Viewport)
}

// Fit reports whether the image fits the viewport rather than a manual zoom.
func (v ImageViewer) Fit() bool { return v.Zoom == 0 }

// Percent is Scale as a percentage.
func (v ImageViewer) Percent() int { return ZoomPercent(v.Scale()) }

// CanZoomIn reports whether ZoomIn changes anything.
func (v ImageViewer) CanZoomIn() bool { return v.Status == ImageReady && v.Scale() < MaximumZoom }

// CanZoomOut reports whether ZoomOut changes anything.
func (v ImageViewer) CanZoomOut() bool { return v.Status == ImageReady && v.Scale() > MinimumZoom }

// Size is the image's size on screen, at least a pixel each way; zero
// before it loads.
func (v ImageViewer) Size() Size {
	if v.Status != ImageReady || v.Image.Width <= 0 || v.Image.Height <= 0 {
		return Size{}
	}
	scale := v.Scale()
	return Size{math.Max(1, math.Floor(v.Image.Width*scale)), math.Max(1, math.Floor(v.Image.Height*scale))}
}

func (v *ImageViewer) setZoom(zoom float64) {
	if v.Status == ImageReady {
		v.Zoom = zoom
	}
}

// ZoomIn steps up.
func (v *ImageViewer) ZoomIn() { v.setZoom(ZoomIn(v.Scale())) }

// ZoomOut steps down.
func (v *ImageViewer) ZoomOut() { v.setZoom(ZoomOut(v.Scale())) }

// FitToView fits the viewport.
func (v *ImageViewer) FitToView() { v.setZoom(0) }

// ActualSize shows the image at 100%.
func (v *ImageViewer) ActualSize() { v.setZoom(1) }

// DiagramZoomStep is one step of Mermaid's expanded viewer (diagram.ts).
const DiagramZoomStep = 1.25

// DiagramView is the expanded viewer's pan and zoom.
type DiagramView struct{ X, Y, Zoom float64 }

// ClampDiagramZoom keeps zoom within bounds, 1 when it is not a number.
func ClampDiagramZoom(zoom float64) float64 {
	if math.IsNaN(zoom) || math.IsInf(zoom, 0) {
		return 1
	}
	return math.Min(MaximumZoom, math.Max(MinimumZoom, zoom))
}

// FitDiagramView shows content at natural size, scaled down to fit
// viewport, centered.
func FitDiagramView(viewport, content Size) DiagramView {
	zoom := ClampDiagramZoom(FitScale(content, viewport))
	return DiagramView{(viewport.Width - content.Width*zoom) / 2, (viewport.Height - content.Height*zoom) / 2, zoom}
}

// Zoomed scales by factor and keeps the anchor point fixed on screen.
func (v DiagramView) Zoomed(factor, anchorX, anchorY float64) DiagramView {
	zoom := ClampDiagramZoom(v.Zoom * factor)
	ratio := zoom / v.Zoom
	return DiagramView{anchorX - (anchorX-v.X)*ratio, anchorY - (anchorY-v.Y)*ratio, zoom}
}

// Panned moves the view without zooming.
func (v DiagramView) Panned(dx, dy float64) DiagramView {
	return DiagramView{v.X + dx, v.Y + dy, v.Zoom}
}
