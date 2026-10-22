import { useCallback, useState } from "react"

import { fitScale, maximumZoom, minimumZoom, zoomIn, zoomOut, zoomPercent } from "./image-zoom.ts"

export type ImageStatus = "loading" | "ready" | "error"

interface ImageViewerState {
  path: string
  status: ImageStatus
  width: number
  height: number
  viewportWidth: number
  viewportHeight: number
  zoom: "fit" | number
  announcement: string
}

export interface ImageViewer {
  status: ImageStatus
  width: number
  height: number
  scale: number
  percent: number
  fit: boolean
  announcement: string
  canZoomIn: boolean
  canZoomOut: boolean
  loaded(width: number, height: number): void
  failed(): void
  resized(width: number, height: number): void
  zoomIn(): void
  zoomOut(): void
  fitToView(): void
  actualSize(): void
}

function initialState(path: string): ImageViewerState {
  return { path, status: "loading", width: 0, height: 0, viewportWidth: 0, viewportHeight: 0, zoom: "fit", announcement: "" }
}

function currentScale(state: ImageViewerState): number {
  if (state.zoom !== "fit") return state.zoom
  return fitScale(state, { width: state.viewportWidth, height: state.viewportHeight })
}

// useImageViewer owns fit/zoom state for the active image document. The
// toolbar, keyboard shortcuts, viewer, and footer all read this one state.
export function useImageViewer(path: string): ImageViewer {
  const [stored, setStored] = useState(() => initialState(path))
  const state = stored.path === path ? stored : initialState(path)

  const update = useCallback((change: (current: ImageViewerState) => ImageViewerState) => {
    setStored((current) => change(current.path === path ? current : initialState(path)))
  }, [path])

  const setZoom = useCallback((next: (current: ImageViewerState) => "fit" | number) => {
    update((current) => {
      if (current.status !== "ready") return current
      const zoom = next(current)
      const percent = zoomPercent(currentScale({ ...current, zoom }))
      return { ...current, zoom, announcement: zoom === "fit" ? `Fit to view, ${percent}%` : `Zoom ${percent}%` }
    })
  }, [update])

  const loaded = useCallback((width: number, height: number) => {
    update((current) => ({ ...current, status: "ready", width, height }))
  }, [update])
  const failed = useCallback(() => {
    update((current) => ({ ...current, status: "error" }))
  }, [update])
  const resized = useCallback((viewportWidth: number, viewportHeight: number) => {
    update((current) => current.viewportWidth === viewportWidth && current.viewportHeight === viewportHeight
      ? current
      : { ...current, viewportWidth, viewportHeight })
  }, [update])

  const zoomInImage = useCallback(() => setZoom((current) => zoomIn(currentScale(current))), [setZoom])
  const zoomOutImage = useCallback(() => setZoom((current) => zoomOut(currentScale(current))), [setZoom])
  const fitToView = useCallback(() => setZoom(() => "fit"), [setZoom])
  const actualSize = useCallback(() => setZoom(() => 1), [setZoom])

  const scale = currentScale(state)
  const ready = state.status === "ready"
  return {
    status: state.status,
    width: state.width,
    height: state.height,
    scale,
    percent: zoomPercent(scale),
    fit: state.zoom === "fit",
    announcement: state.announcement,
    canZoomIn: ready && scale < maximumZoom,
    canZoomOut: ready && scale > minimumZoom,
    loaded,
    failed,
    resized,
    zoomIn: zoomInImage,
    zoomOut: zoomOutImage,
    fitToView,
    actualSize,
  }
}
