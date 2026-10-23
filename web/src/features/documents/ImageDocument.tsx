import { type KeyboardEvent, useLayoutEffect, useRef } from "react"

import type { ImageViewer } from "./use-image-viewer.ts"

interface ImageDocumentProps {
  name: string
  src: string
  viewer: ImageViewer
}

export function ImageDocument({ name, src, viewer }: ImageDocumentProps) {
  const element = useRef<HTMLDivElement>(null)
  const { resized } = viewer

  useLayoutEffect(() => {
    const node = element.current
    const viewport = node?.parentElement
    if (!node || !viewport) return
    const measure = () => {
      const style = getComputedStyle(node)
      const horizontal = Number.parseFloat(style.paddingLeft) + Number.parseFloat(style.paddingRight)
      const vertical = Number.parseFloat(style.paddingTop) + Number.parseFloat(style.paddingBottom)
      resized(Math.max(0, viewport.clientWidth - horizontal), Math.max(0, viewport.clientHeight - vertical))
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(viewport)
    return () => observer.disconnect()
  }, [resized])

  const size = viewer.status === "ready" && viewer.width > 0 && viewer.height > 0
    ? {
        width: Math.max(1, Math.floor(viewer.width * viewer.scale)),
        height: Math.max(1, Math.floor(viewer.height * viewer.scale)),
      }
    : undefined

  return (
    <div
      className="image-document"
      ref={element}
      role="group"
      aria-label={`${name} image`}
      aria-busy={viewer.status === "loading"}
      aria-keyshortcuts="+ - 0 1"
      tabIndex={0}
    >
      {viewer.status === "error" ? (
        <p className="image-document-message" role="alert">Can't display this image.</p>
      ) : (
        <div className={`image-document-frame${viewer.status === "loading" ? " loading" : ""}`} style={size}>
          <img
            src={src}
            alt={name}
            draggable={false}
            style={size}
            onLoad={(event) => viewer.loaded(event.currentTarget.naturalWidth, event.currentTarget.naturalHeight)}
            onError={() => viewer.failed()}
          />
        </div>
      )}
    </div>
  )
}

// handleImageKey applies viewer shortcuts while focus is in the document pane.
export function handleImageKey(event: KeyboardEvent<HTMLElement>, viewer: ImageViewer) {
  if (event.ctrlKey || event.metaKey || event.altKey || viewer.status !== "ready") return
  switch (event.key) {
    case "+":
    case "=":
      viewer.zoomIn()
      break
    case "-":
    case "_":
      viewer.zoomOut()
      break
    case "0":
      viewer.fitToView()
      break
    case "1":
      viewer.actualSize()
      break
    default:
      return
  }
  event.preventDefault()
}
