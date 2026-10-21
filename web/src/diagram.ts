import { RenderError } from "ondine/compiler/wasm"

import { fitScale, maximumZoom, minimumZoom } from "./image-zoom.ts"

export const diagramZoomStep = 1.25
export const mermaidRenderError = "Could not render Mermaid diagram. Check its source."

export interface DiagramPoint {
  x: number
  y: number
}

export interface DiagramSize {
  height: number
  width: number
}

export interface DiagramView extends DiagramPoint {
  zoom: number
}

export function clampDiagramZoom(zoom: number): number {
  if (!Number.isFinite(zoom)) return 1
  return Math.min(maximumZoom, Math.max(minimumZoom, zoom))
}

// fitDiagramView shows content at natural size, scaled down to fit the viewport, centered.
export function fitDiagramView(viewport: DiagramSize, content: DiagramSize): DiagramView {
  const zoom = clampDiagramZoom(fitScale(content, viewport))
  return {
    x: (viewport.width - content.width * zoom) / 2,
    y: (viewport.height - content.height * zoom) / 2,
    zoom,
  }
}

// zoomDiagramView scales by factor and keeps the anchor point fixed on screen.
export function zoomDiagramView(view: DiagramView, factor: number, anchor: DiagramPoint): DiagramView {
  const zoom = clampDiagramZoom(view.zoom * factor)
  const ratio = zoom / view.zoom
  return {
    x: anchor.x - (anchor.x - view.x) * ratio,
    y: anchor.y - (anchor.y - view.y) * ratio,
    zoom,
  }
}

export function panDiagramView(view: DiagramView, dx: number, dy: number): DiagramView {
  return { ...view, x: view.x + dx, y: view.y + dy }
}

// mermaidErrorMessage keeps ondine's Mermaid-compatible diagnostic, including
// its line and caret snippet; other failures get a generic message.
export function mermaidErrorMessage(error: unknown): string {
  return error instanceof RenderError && error.message ? error.message : mermaidRenderError
}
