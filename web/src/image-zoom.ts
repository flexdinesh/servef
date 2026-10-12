export const minimumZoom = 0.1
export const maximumZoom = 8
const zoomFactor = 1.25

interface Size {
  width: number
  height: number
}

function roundZoom(scale: number): number {
  return Math.round(scale * 10_000) / 10_000
}

function clampZoom(scale: number): number {
  return Math.min(maximumZoom, Math.max(minimumZoom, roundZoom(scale)))
}

// fitScale fits an image inside the viewport without enlarging it.
export function fitScale(image: Size, viewport: Size): number {
  if (image.width <= 0 || image.height <= 0 || viewport.width <= 0 || viewport.height <= 0) return 1
  // Very large images may fit below the manual zoom floor.
  return Math.max(0.0001, roundZoom(Math.min(1, viewport.width / image.width, viewport.height / image.height)))
}

export function zoomIn(scale: number): number {
  return Math.max(scale, clampZoom(scale * zoomFactor))
}

export function zoomOut(scale: number): number {
  return Math.min(scale, clampZoom(scale / zoomFactor))
}

export function zoomPercent(scale: number): number {
  return Math.round(scale * 100)
}
