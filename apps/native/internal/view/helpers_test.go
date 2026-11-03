package view

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"github.com/flexdinesh/servef/pkg/contract"
)

func pngBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func eventStatus(mode string) contract.Event {
	return contract.Event{Name: contract.EventStatus, Status: &contract.StatusEvent{Mode: mode}}
}
