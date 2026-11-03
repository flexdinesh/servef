import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react"

export type SidebarView = "explorer" | "search" | "source-control" | "walkthroughs"

const storageKey = "servef-sidebar"
const mobileQuery = "(max-width: 48rem)"

export interface StoredSidebar {
  hidden: boolean
  view: SidebarView
}

const defaultSidebar: StoredSidebar = { view: "explorer", hidden: false }

function isSidebarView(value: unknown): value is SidebarView {
  return value === "explorer" || value === "search" || value === "source-control" || value === "walkthroughs"
}

// parseSidebar reads stored sidebar state; unknown views open the Explorer.
export function parseSidebar(value: unknown): StoredSidebar {
  if (typeof value !== "object" || value === null || !("view" in value) || !("hidden" in value)) return defaultSidebar
  return {
    view: isSidebarView(value.view) ? value.view : "explorer",
    hidden: value.hidden === true,
  }
}

function readSidebar(): StoredSidebar {
  try {
    return parseSidebar(JSON.parse(window.localStorage.getItem(storageKey) ?? "null"))
  } catch {
    // Fall back to the defaults when storage is unavailable or malformed.
    return defaultSidebar
  }
}

function isModifiedShortcut(event: KeyboardEvent, key: string, shift: boolean): boolean {
  return !event.defaultPrevented && !event.altKey && event.shiftKey === shift
    && (event.metaKey || event.ctrlKey) && event.key.toLowerCase() === key
}

// useSidebar owns the activity bar's view, desktop visibility, and the mobile
// drawer. Clicking the active view hides the sidebar on desktop, as in VS Code.
export function useSidebar() {
  const drawer = useRef<HTMLDialogElement>(null)
  const [stored] = useState(readSidebar)
  const [view, setView] = useState(stored.view)
  const [hidden, setHidden] = useState(stored.hidden)
  const [mobile, setMobile] = useState(() => window.matchMedia(mobileQuery).matches)
  const mobileRef = useRef(mobile)
  useLayoutEffect(() => { mobileRef.current = mobile }, [mobile])

  useEffect(() => {
    try {
      window.localStorage.setItem(storageKey, JSON.stringify({ view, hidden }))
    } catch {
      // The sidebar still works when storage is unavailable.
    }
  }, [hidden, view])

  useEffect(() => {
    const query = window.matchMedia(mobileQuery)
    const update = () => {
      if (!query.matches && drawer.current?.open) drawer.current.close()
      setMobile(query.matches)
    }
    query.addEventListener("change", update)
    return () => query.removeEventListener("change", update)
  }, [])

  const reveal = useCallback((next: SidebarView) => {
    setView(next)
    setHidden(false)
    if (mobileRef.current && !drawer.current?.open) {
      drawer.current?.showModal()
      // showModal focuses the first activity item; move that focus to the
      // revealed view's item unless the view already took it, as Search does.
      requestAnimationFrame(() => {
        if (!document.activeElement?.closest(".activity-bar")) return
        drawer.current?.querySelector<HTMLElement>(`.activity-item[data-view="${next}"]`)?.focus()
      })
    }
  }, [])

  const dismiss = useCallback(() => {
    if (drawer.current?.open) drawer.current.close()
  }, [])

  const select = useCallback((next: SidebarView) => {
    if (!mobileRef.current && next === view && !hidden) setHidden(true)
    else reveal(next)
  }, [hidden, reveal, view])

  const toggle = useCallback(() => {
    if (!mobileRef.current) setHidden((current) => !current)
    else if (drawer.current?.open) drawer.current.close()
    else reveal(view)
  }, [reveal, view])

  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if (isModifiedShortcut(event, "b", false)) toggle()
      else if (isModifiedShortcut(event, "e", true)) reveal("explorer")
      else if (isModifiedShortcut(event, "g", true)) reveal("source-control")
      else return
      event.preventDefault()
    }
    document.addEventListener("keydown", shortcut)
    return () => document.removeEventListener("keydown", shortcut)
  }, [reveal, toggle])

  return { drawer, mobile, view, hidden: hidden && !mobile, reveal, select, dismiss }
}

export type SidebarState = ReturnType<typeof useSidebar>
