import { useNavigate, useRouter, useRouterState } from "@tanstack/react-router"
import { useCallback, useEffect, useLayoutEffect, useRef } from "react"

import { parseLineAnchor } from "./line-selection.ts"
import type { PageData } from "../../api/page-data.ts"

// revealLine scrolls a line anchor into view near the top third, unless visible.
function revealLine(pane: HTMLElement, line: Element) {
  const paneBox = pane.getBoundingClientRect()
  const lineBox = line.getBoundingClientRect()
  if (lineBox.top >= paneBox.top && lineBox.bottom <= paneBox.bottom) return
  pane.scrollTop += lineBox.top - paneBox.top - pane.clientHeight / 3
}

export function useDocumentNavigation(project: string, page: PageData, isLoading: boolean, loadError: string) {
  const navigate = useNavigate()
  const router = useRouter()
  const location = useRouterState({ select: (state) => state.location })
  const main = useRef<HTMLElement>(null)
  const focusDocument = useRef(false)
  const displayedPath = useRef("")
  const documentPositions = useRef(new Map<string, number>())
  const historyPositions = useRef(new Map<string, { path: string; top: number }>())
  const selected = page.hasFile && !page.error && !loadError ? page.selected : ""

  useLayoutEffect(() => { displayedPath.current = selected }, [selected])

  useEffect(() => router.subscribe("onBeforeLoad", ({ fromLocation }) => {
    const path = displayedPath.current
    if (!path || !main.current) return
    const top = main.current.scrollTop
    documentPositions.current.set(path, top)
    if (fromLocation) {
      historyPositions.current.set(fromLocation.state.__TSR_key ?? fromLocation.href, { path, top })
    }
  }), [router])

  useEffect(() => {
    if (isLoading || loadError) return
    if (focusDocument.current) {
      focusDocument.current = false
      main.current?.focus({ preventScroll: true })
    }
    const frame = window.requestAnimationFrame(() => {
      const pane = main.current
      if (!pane) return
      let hash = location.hash
      try { hash = decodeURIComponent(hash) } catch { /* Keep malformed fragments literal. */ }
      const lines = parseLineAnchor(hash)
      const line = lines ? pane.querySelector(`.source-document [data-line="${lines.start}"]`) : null
      const anchor = hash && !lines ? document.getElementById(hash) : null
      if (line) {
        revealLine(pane, line)
      } else if (anchor && pane.contains(anchor)) {
        anchor.scrollIntoView({ block: "start" })
      } else {
        const entry = historyPositions.current.get(location.state.__TSR_key ?? location.href)
        pane.scrollTop = entry?.path === selected ? entry.top : documentPositions.current.get(selected) ?? 0
      }
    })
    return () => window.cancelAnimationFrame(frame)
  }, [isLoading, loadError, location.hash, location.href, location.state.__TSR_key, page.content, selected])

  const prepareDocumentNavigation = useCallback(() => { focusDocument.current = true }, [])

  const navigateToDocument = useCallback((path: string) => {
    prepareDocumentNavigation()
    void navigate({ to: "/view", search: { project, path }, hash: "" })
  }, [navigate, prepareDocumentNavigation, project])

  const navigateToHref = useCallback((href: string) => {
    prepareDocumentNavigation()
    void navigate({ href })
  }, [navigate, prepareDocumentNavigation])

  const forgetDocumentScroll = useCallback((path: string) => {
    documentPositions.current.delete(path)
    for (const [key, entry] of historyPositions.current) {
      if (entry.path === path) historyPositions.current.delete(key)
    }
    // Closing the active tab must not save its scroll again during navigation.
    if (displayedPath.current === path) displayedPath.current = ""
  }, [])

  const resetDocumentScroll = useCallback((path: string) => {
    documentPositions.current.set(path, 0)
    for (const entry of historyPositions.current.values()) {
      if (entry.path === path) entry.top = 0
    }
    if (main.current) main.current.scrollTop = 0
  }, [])

  return { main, prepareDocumentNavigation, navigateToDocument, navigateToHref,
    forgetDocumentScroll, resetDocumentScroll }
}
