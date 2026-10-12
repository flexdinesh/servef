import { MinusIcon, PlusIcon } from "lucide-react"

import type { ImageViewer } from "./use-image-viewer.ts"

export function ImageZoomControls({ viewer }: { viewer: ImageViewer }) {
  const ready = viewer.status === "ready"
  return (
    <div className="document-mode-toggle image-zoom-controls" role="group" aria-label="Zoom">
      <button type="button" aria-pressed={viewer.fit} disabled={!ready} title="Fit to view (0)" onClick={viewer.fitToView}>
        Fit
      </button>
      <button type="button" aria-label="Zoom out" disabled={!viewer.canZoomOut} title="Zoom out (−)" onClick={viewer.zoomOut}>
        <MinusIcon aria-hidden="true" />
      </button>
      <button
        type="button"
        className="image-zoom-level"
        aria-label={`${viewer.percent}%, reset to 100%`}
        disabled={!ready}
        title="Actual size (1)"
        onClick={viewer.actualSize}
      >
        {viewer.percent}%
      </button>
      <button type="button" aria-label="Zoom in" disabled={!viewer.canZoomIn} title="Zoom in (+)" onClick={viewer.zoomIn}>
        <PlusIcon aria-hidden="true" />
      </button>
      <span className="visually-hidden" aria-live="polite">{viewer.announcement}</span>
    </div>
  )
}
