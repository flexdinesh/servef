import { MinusIcon, PlusIcon } from "lucide-react"

import { SegmentedControl, SegmentedItem } from "../../components/ui/segmented-control.tsx"
import type { ImageViewer } from "./use-image-viewer.ts"

export function ImageZoomControls({ viewer }: { viewer: ImageViewer }) {
  const ready = viewer.status === "ready"
  return (
    <SegmentedControl className="document-mode-toggle image-zoom-controls" aria-label="Zoom">
      <SegmentedItem pressed={viewer.fit} disabled={!ready} title="Fit to view (0)" onClick={viewer.fitToView}>
        Fit
      </SegmentedItem>
      <SegmentedItem aria-label="Zoom out" disabled={!viewer.canZoomOut} title="Zoom out (−)" onClick={viewer.zoomOut}>
        <MinusIcon aria-hidden="true" />
      </SegmentedItem>
      <SegmentedItem
        className="image-zoom-level"
        aria-label={`${viewer.percent}%, reset to 100%`}
        disabled={!ready}
        title="Actual size (1)"
        onClick={viewer.actualSize}
      >
        {viewer.percent}%
      </SegmentedItem>
      <SegmentedItem aria-label="Zoom in" disabled={!viewer.canZoomIn} title="Zoom in (+)" onClick={viewer.zoomIn}>
        <PlusIcon aria-hidden="true" />
      </SegmentedItem>
      <span className="visually-hidden" aria-live="polite">{viewer.announcement}</span>
    </SegmentedControl>
  )
}
