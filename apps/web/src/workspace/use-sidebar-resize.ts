import { type KeyboardEvent, type PointerEvent, useEffect, useRef, useState } from "react"

const defaultWidth = 288
const minimumWidth = 208
const maximumWidth = 480
const storageKey = "servef-sidebar-width"

function clampWidth(width: number): number {
  return Math.min(maximumWidth, Math.max(minimumWidth, width))
}

function readWidth(): number {
  try {
    const stored = Number(window.localStorage.getItem(storageKey))
    return Number.isFinite(stored) && stored > 0 ? clampWidth(stored) : defaultWidth
  } catch {
    return defaultWidth
  }
}

export function useSidebarResize() {
  const [width, setWidth] = useState(readWidth)
  const resizeStart = useRef<{ pointerID: number; width: number; x: number } | null>(null)

  useEffect(() => {
    document.documentElement.style.setProperty("--sidebar-width", `${width}px`)
    try {
      window.localStorage.setItem(storageKey, String(width))
    } catch {
      // Resizing still applies when storage is unavailable.
    }
  }, [width])

  function startResize(event: PointerEvent<HTMLDivElement>) {
    resizeStart.current = { pointerID: event.pointerId, width, x: event.clientX }
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  function moveResize(event: PointerEvent<HTMLDivElement>) {
    const start = resizeStart.current
    if (!start || start.pointerID !== event.pointerId) return
    setWidth(clampWidth(start.width + event.clientX - start.x))
  }

  function stopResize(event: PointerEvent<HTMLDivElement>) {
    if (resizeStart.current?.pointerID !== event.pointerId) return
    resizeStart.current = null
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
  }

  function resizeWithKeyboard(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return
    event.preventDefault()
    setWidth((current) => clampWidth(current + (event.key === "ArrowLeft" ? -16 : 16)))
  }

  return { width, minimumWidth, maximumWidth, startResize, moveResize, stopResize,
    resizeWithKeyboard, resetWidth: () => setWidth(defaultWidth) }
}
