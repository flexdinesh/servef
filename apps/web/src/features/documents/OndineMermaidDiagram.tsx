import { Maximize2Icon, RotateCcwIcon, XIcon, ZoomInIcon, ZoomOutIcon } from "lucide-react"
import { use, useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef, useState, type PointerEvent } from "react"
import { createPortal } from "react-dom"
import type { Render } from "ondine/compiler/wasm"

import { DialogShell } from "../../components/ui/dialog-shell.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { ViewHeader } from "../../components/ui/view-header.tsx"
import {
  diagramZoomStep,
  fitDiagramView,
  mermaidErrorMessage,
  panDiagramView,
  zoomDiagramView,
  type DiagramPoint,
  type DiagramSize,
  type DiagramView,
} from "./diagram.ts"
import { MermaidSource } from "./MermaidSource.tsx"
import { loadOndineRenderer, type OndineRenderer } from "./ondine-renderer.ts"

type RenderedDiagram = { error: string } | { diagram: Render.render.ReturnType }

function renderDiagram(renderer: OndineRenderer, source: string, options: Render.render.Options): RenderedDiagram {
  try {
    return { diagram: renderer.render(source, options) }
  } catch (error) {
    return { error: mermaidErrorMessage(error) }
  }
}

// diagramElement parses ondine's SVG in an inert template. Diagram links are not
// navigable, so anchors are unwrapped and their content kept.
function diagramElement(svg: string): SVGSVGElement | null {
  const template = document.createElement("template")
  template.innerHTML = svg
  const root = template.content.firstElementChild
  if (!(root instanceof SVGSVGElement)) return null
  for (const anchor of root.querySelectorAll("a")) anchor.replaceWith(...anchor.childNodes)
  return root
}

function DiagramSVG({ className, svg }: { className: string; svg: string }) {
  const host = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const element = diagramElement(svg)
    host.current?.replaceChildren(...(element ? [element] : []))
  }, [svg])

  return <div ref={host} className={className} role="img" aria-label="Mermaid diagram" />
}

export default function OndineMermaidDiagram({ source }: { source: string }) {
  const renderer = use(loadOndineRenderer())
  const idPrefix = `mermaid-${useId().replace(/[^a-zA-Z0-9_-]/g, "")}`
  const frame = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState<number | null>(null)
  const [expanded, setExpanded] = useState(false)

  useLayoutEffect(() => {
    const element = frame.current
    if (!element) return undefined
    const measure = () => setWidth(Math.round(element.getBoundingClientRect().width))
    measure()
    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", measure)
      return () => window.removeEventListener("resize", measure)
    }
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  const rendered = useMemo(
    () => width === null ? null : renderDiagram(renderer, source, { idPrefix, maxWidth: width, padding: 16 }),
    [idPrefix, renderer, source, width],
  )
  const expandedDiagram = useMemo(
    () => expanded ? renderDiagram(renderer, source, { autoDirection: false, idPrefix: `${idPrefix}-expanded`, padding: 16 }) : null,
    [expanded, idPrefix, renderer, source],
  )

  return (
    <div ref={frame} className="mermaid-frame">
      {rendered === null && <MermaidSource source={source} />}
      {rendered !== null && "error" in rendered && <MermaidSource source={source} error={rendered.error} />}
      {rendered !== null && "diagram" in rendered && (
        <div className="mermaid-diagram">
          <DiagramSVG className="mermaid-svg" svg={rendered.diagram.svg} />
          <div className="mermaid-diagram-controls">
            <IconButton
              label="Expand diagram"
              icon={<Maximize2Icon aria-hidden="true" />}
              onClick={() => setExpanded(true)}
            />
          </div>
        </div>
      )}
      {/* The dialog renders outside the article so prose styles stay off its chrome. */}
      {expandedDiagram !== null && "diagram" in expandedDiagram && createPortal(
        <MermaidDiagramDialog diagram={expandedDiagram.diagram} onClose={() => setExpanded(false)} />,
        document.body,
      )}
    </div>
  )
}

interface MermaidDiagramDialogProps {
  diagram: Render.render.ReturnType
  onClose(): void
}

function MermaidDiagramDialog({ diagram, onClose }: MermaidDiagramDialogProps) {
  const dialog = useRef<HTMLDialogElement>(null)
  const viewport = useRef<HTMLDivElement>(null)
  const drag = useRef<DiagramPoint | null>(null)
  const [view, setView] = useState<DiagramView | null>(null)
  const content = useMemo<DiagramSize>(() => ({ height: diagram.height, width: diagram.width }), [diagram])

  const fit = useCallback(() => {
    const element = viewport.current
    if (!element) return
    setView(fitDiagramView({ height: element.clientHeight, width: element.clientWidth }, content))
  }, [content])

  const zoomBy = useCallback((factor: number, anchor?: DiagramPoint) => {
    const element = viewport.current
    if (!element) return
    const center = { x: element.clientWidth / 2, y: element.clientHeight / 2 }
    setView((current) => current && zoomDiagramView(current, factor, anchor ?? center))
  }, [])

  useLayoutEffect(() => {
    if (dialog.current && !dialog.current.open) dialog.current.showModal()
    fit()
  }, [fit])

  useEffect(() => {
    const element = viewport.current
    if (!element) return undefined
    // React registers wheel listeners as passive; zoom needs preventDefault.
    const wheel = (event: WheelEvent) => {
      event.preventDefault()
      if (event.ctrlKey || event.metaKey) {
        const bounds = element.getBoundingClientRect()
        zoomBy(event.deltaY < 0 ? diagramZoomStep : 1 / diagramZoomStep, { x: event.clientX - bounds.left, y: event.clientY - bounds.top })
        return
      }
      setView((current) => current && panDiagramView(current, -event.deltaX, -event.deltaY))
    }
    element.addEventListener("wheel", wheel, { passive: false })
    return () => element.removeEventListener("wheel", wheel)
  }, [zoomBy])

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return
    event.currentTarget.setPointerCapture(event.pointerId)
    drag.current = { x: event.clientX, y: event.clientY }
  }
  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const last = drag.current
    if (!last) return
    drag.current = { x: event.clientX, y: event.clientY }
    setView((current) => current && panDiagramView(current, event.clientX - last.x, event.clientY - last.y))
  }
  const endDrag = () => { drag.current = null }

  return (
    <DialogShell
      ref={dialog}
      variant="viewer"
      className="mermaid-dialog"
      aria-label="Mermaid diagram"
      onClick={(event) => { if (event.target === dialog.current) dialog.current.close() }}
      onClose={onClose}
    >
      <div className="mermaid-dialog-shell">
        <ViewHeader
          variant="dialog"
          title="Mermaid diagram"
          actions={<>
            <IconButton label="Zoom out" icon={<ZoomOutIcon aria-hidden="true" />} onClick={() => zoomBy(1 / diagramZoomStep)} />
            <IconButton label="Zoom in" icon={<ZoomInIcon aria-hidden="true" />} onClick={() => zoomBy(diagramZoomStep)} />
            <IconButton label="Reset zoom" icon={<RotateCcwIcon aria-hidden="true" />} onClick={fit} />
            <IconButton label="Close diagram" icon={<XIcon aria-hidden="true" />} onClick={() => dialog.current?.close()} />
          </>}
        >
          <span className="mermaid-dialog-zoom" role="status">{view ? `${Math.round(view.zoom * 100)}%` : ""}</span>
        </ViewHeader>
        <div
          ref={viewport}
          className="mermaid-viewer"
          onLostPointerCapture={endDrag}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={endDrag}
        >
          <div
            className="mermaid-viewer-content"
            style={view ? { transform: `translate(${view.x}px, ${view.y}px) scale(${view.zoom})` } : { visibility: "hidden" }}
          >
            <DiagramSVG className="mermaid-svg" svg={diagram.svg} />
          </div>
        </div>
      </div>
    </DialogShell>
  )
}
