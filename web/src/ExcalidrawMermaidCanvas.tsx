import {
  convertToExcalidrawElements,
  Excalidraw,
  getCommonBounds,
} from "@excalidraw/excalidraw"
import type {
  ExcalidrawImperativeAPI,
  ExcalidrawInitialDataState,
} from "@excalidraw/excalidraw/types"
import { parseMermaidToExcalidraw } from "@excalidraw/mermaid-to-excalidraw"
import { useCallback, useEffect, useRef, useState, type WheelEvent } from "react"
import "@excalidraw/excalidraw/index.css"

import {
  addTextRasterGutters,
  calculateDiagramHeight,
  minimumDiagramHeight,
  padTextBounds,
  type DiagramBounds,
} from "./diagram.ts"
import type { MermaidTheme } from "./mermaid-controller.ts"
import { MermaidSource } from "./MermaidSource.tsx"

const renderError = "Could not render Mermaid diagram. Check its source."

interface PreparedDiagram {
  bounds: DiagramBounds
  data: ExcalidrawInitialDataState
}

export default function ExcalidrawMermaidCanvas({ source, theme }: { source: string; theme: MermaidTheme }) {
  const host = useRef<HTMLDivElement>(null)
  const apiRef = useRef<ExcalidrawImperativeAPI | null>(null)
  const mountFrame = useRef<number | null>(null)
  const [diagram, setDiagram] = useState<PreparedDiagram | null>(null)
  const [error, setError] = useState("")
  const [height, setHeight] = useState(minimumDiagramHeight)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    let active = true
    setDiagram(null)
    setError("")
    setReady(false)

    void parseMermaidToExcalidraw(source).then(({ elements: skeletons, files }) => {
      if (!active) return
      // Excalidraw clips its text raster and element bounds separately at high pixel ratios.
      const paddedSkeletons = skeletons.map((element) => {
        if (element.type === "text") return { ...element, text: addTextRasterGutters(element.text) }
        if ("label" in element && element.label) {
          return { ...element, label: { ...element.label, text: addTextRasterGutters(element.label.text) } }
        }
        return element
      })
      const elements = convertToExcalidrawElements(paddedSkeletons, { regenerateIds: true }).map((element) =>
        element.type === "text" ? { ...element, ...padTextBounds(element) } : element,
      )
      if (elements.length === 0) throw new Error("Mermaid diagram created no shapes")
      const [minX, minY, maxX, maxY] = getCommonBounds(elements)
      const bounds = { h: maxY - minY, w: maxX - minX }
      setDiagram({
        bounds,
        data: {
          elements,
          files,
          scrollToContent: true,
        },
      })
    }).catch(() => {
      if (active) setError(renderError)
    })

    return () => { active = false }
  }, [source])

  useEffect(() => {
    if (!diagram || !host.current) return
    const element = host.current
    const resize = () => setHeight(calculateDiagramHeight(element.getBoundingClientRect().width, diagram.bounds))
    resize()
    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", resize)
      return () => window.removeEventListener("resize", resize)
    }
    const observer = new ResizeObserver(resize)
    observer.observe(element)
    return () => observer.disconnect()
  }, [diagram])

  useEffect(() => {
    const refresh = () => apiRef.current?.refresh()
    window.addEventListener("scroll", refresh, true)
    return () => window.removeEventListener("scroll", refresh, true)
  }, [])

  useEffect(() => () => {
    if (mountFrame.current !== null) cancelAnimationFrame(mountFrame.current)
    apiRef.current = null
  }, [])

  const onMount = useCallback((api: ExcalidrawImperativeAPI) => {
    apiRef.current = api
    if (mountFrame.current !== null) cancelAnimationFrame(mountFrame.current)
    mountFrame.current = requestAnimationFrame(() => {
      mountFrame.current = null
      if (apiRef.current !== api) return
      api.scrollToContent(api.getSceneElements(), {
        fitToViewport: true,
        maxZoom: 1,
        viewportZoomFactor: 0.75,
      })
      setReady(true)
    })
  }, [])

  const onWheelCapture = useCallback((event: WheelEvent<HTMLDivElement>) => {
    if (!event.ctrlKey && !event.metaKey) return
    apiRef.current?.refresh()
    document.dispatchEvent(new PointerEvent("pointermove", {
      clientX: event.clientX,
      clientY: event.clientY,
    }))
  }, [])

  if (error) return <MermaidSource source={source} error={error} />
  if (!diagram) return <MermaidSource source={source} />

  return (
    <div
      ref={host}
      className={`mermaid-diagram mermaid-excalidraw${ready ? " ready" : " loading"}`}
      onWheelCapture={onWheelCapture}
      style={ready ? { height } : { minHeight: height }}
    >
      {!ready && <MermaidSource source={source} />}
      <div className="mermaid-canvas" aria-label="Mermaid diagram">
        <Excalidraw
          autoFocus={false}
          detectScroll
          excalidrawAPI={onMount}
          initialData={diagram.data}
          theme={theme}
          viewModeEnabled
          zenModeEnabled
        />
      </div>
    </div>
  )
}
