import { useEffect, useLayoutEffect, useRef, useState } from "react"

import { createSearchController, type SearchHandlers, type SearchStatus, type SearchView } from "./search-controller.ts"
import { isSearchDocumentsResponse, type SearchDocumentsResponse } from "./search-documents.ts"
import type { SearchGroups } from "./search-engine.ts"

async function fetchDocuments(signal: AbortSignal): Promise<SearchDocumentsResponse> {
  const response = await fetch("/api/search-documents", { headers: { Accept: "application/json" }, signal })
  if (!response.ok) throw new Error(`Search documents request failed: ${response.status}`)
  const payload: unknown = await response.json()
  if (!isSearchDocumentsResponse(payload)) throw new Error("Invalid search documents response")
  return payload
}

export function useSearchSession(navigate: (path: string) => void, registerOpen: (open: (() => void) | null) => void) {
  const dialog = useRef<HTMLDialogElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const handlers = useRef<SearchHandlers | null>(null)
  const controllerRef = useRef<ReturnType<typeof createSearchController> | null>(null)
  const navigateRef = useRef(navigate)
  const [activeIndex, setActiveIndex] = useState(-1)
  const [expanded, setExpanded] = useState(false)
  const [groups, setGroups] = useState<SearchGroups | null>(null)
  const [query, setQuery] = useState("")
  const [status, setStatus] = useState<Required<SearchStatus>>({ kind: "idle", message: "", detail: "" })

  useLayoutEffect(() => { navigateRef.current = navigate }, [navigate])

  useEffect(() => {
    if (!dialog.current || !input.current) return
    let scrollFrame: number | null = null
    const view: SearchView = {
      setHandlers(next) { handlers.current = next },
      open() {
        if (!dialog.current?.open) dialog.current?.showModal()
        setExpanded(true)
      },
      close() {
        if (dialog.current?.open) dialog.current.close()
        setExpanded(false)
      },
      focusInput() {
        input.current?.focus({ preventScroll: true })
        input.current?.select()
      },
      clearResults() {
        setGroups(null)
        setActiveIndex(-1)
      },
      setStatus(next = {}) {
        setStatus({ kind: next.kind || "idle", message: next.message || "", detail: next.detail || "" })
      },
      renderResults(next) {
        setGroups({ contentResults: next.contentResults, pathResults: next.pathResults })
        setActiveIndex(next.activeIndex)
      },
      setActive(index, scroll = true) {
        setActiveIndex(index)
        if (scrollFrame !== null) cancelAnimationFrame(scrollFrame)
        if (scroll) scrollFrame = requestAnimationFrame(() => {
          dialog.current?.querySelector(`#search-option-${index}`)?.scrollIntoView({ block: "nearest" })
        })
      },
    }
    const controller = createSearchController({
      fetchDocuments,
      createWorker: () => new Worker(new URL("./search-worker.ts", import.meta.url), { type: "module" }),
      navigate(path) {
        controller.close()
        navigateRef.current(path)
      },
      view,
    })
    controllerRef.current = controller
    controller.start()
    return () => {
      controller.destroy()
      controllerRef.current = null
      if (scrollFrame !== null) cancelAnimationFrame(scrollFrame)
    }
  }, [])

  useEffect(() => {
    registerOpen(controllerRef.current?.open ?? null)
    return () => registerOpen(null)
  }, [registerOpen])

  function changeQuery(next: string) {
    setQuery(next)
    handlers.current?.onInput(next)
  }

  return { dialog, input, handlers, activeIndex, expanded, groups, query, status, changeQuery }
}
