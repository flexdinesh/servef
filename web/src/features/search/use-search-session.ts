import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react"

import { createSearchController, emptyQuery, type SearchHandlers, type SearchResults, type SearchStatus, type SearchView } from "./search-controller.ts"
import { failureReason, fetchJSON } from "../../api/http.ts"
import { projectUrl } from "../../api/project-url.ts"
import { isSearchDocumentsResponse, type SearchDocumentsResponse } from "./search-documents.ts"
import type { SearchMatch, SearchQuery } from "./search-engine.ts"
import { adjacentMatch, type MatchPosition } from "./search-results.ts"

function fetchDocuments(project: string, signal: AbortSignal): Promise<SearchDocumentsResponse> {
  return fetchJSON(projectUrl("/api/search-documents", project), { signal }, isSearchDocumentsResponse,
    (failure) => `Couldn't load search documents: ${failureReason(failure)}.`)
}

// selectedSearchText seeds Find in Files from a single-line document selection.
function selectedSearchText(pane: HTMLElement | null): string {
  const selection = window.getSelection()
  const text = selection?.toString() ?? ""
  const anchor = selection?.anchorNode
  if (!text.trim() || /[\r\n]/.test(text) || !anchor || !pane?.contains(anchor)) return ""
  return text
}

export type OpenSearchMatch = (path: string, match: SearchMatch, focusDocument: boolean) => void

interface SearchSessionOptions {
  documentPane: RefObject<HTMLElement | null>
  openMatch: OpenSearchMatch
  project: string
  registerOpen(open: (() => void) | null): void
  reveal(): void
}

// useSearchSession indexes one project's documents; mount one per project.
export function useSearchSession({ documentPane, openMatch, project, registerOpen, reveal }: SearchSessionOptions) {
  const input = useRef<HTMLInputElement>(null)
  const handlers = useRef<SearchHandlers | null>(null)
  const controllerRef = useRef<ReturnType<typeof createSearchController> | null>(null)
  const latest = useRef({ documentPane, openMatch, project, reveal })
  const [query, setQuery] = useState<SearchQuery>(emptyQuery)
  const queryRef = useRef(query)
  const [details, setDetails] = useState(false)
  const [results, setResults] = useState<SearchResults | null>(null)
  const resultsRef = useRef(results)
  const [status, setStatus] = useState<SearchStatus>({ kind: "idle" })
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const [active, setActive] = useState<MatchPosition | null>(null)
  const activeRef = useRef(active)
  const [focusRequest, setFocusRequest] = useState(0)

  useLayoutEffect(() => { latest.current = { documentPane, openMatch, project, reveal } }, [documentPane, openMatch, project, reveal])
  useLayoutEffect(() => { resultsRef.current = results }, [results])
  useLayoutEffect(() => { activeRef.current = active }, [active])

  const changeQuery = useCallback((change: Partial<SearchQuery>) => {
    const next = { ...queryRef.current, ...change }
    queryRef.current = next
    setQuery(next)
    handlers.current?.onInput(next)
  }, [])

  // Focus waits for the view to render visible, since revealing is a state change.
  useLayoutEffect(() => {
    if (!focusRequest) return
    input.current?.focus({ preventScroll: true })
    input.current?.select()
  }, [focusRequest])

  useEffect(() => {
    const view: SearchView = {
      setHandlers(next) { handlers.current = next },
      open() {
        latest.current.reveal()
        const seed = selectedSearchText(latest.current.documentPane.current)
        if (seed && seed !== queryRef.current.pattern) changeQuery({ pattern: seed })
      },
      focusInput() { setFocusRequest((count) => count + 1) },
      clearResults() {
        setResults(null)
        setActive(null)
      },
      setStatus(next = {}) { setStatus(next) },
      renderResults(next) {
        setResults(next)
        setCollapsed(new Set())
        setActive(null)
      },
    }
    const controller = createSearchController({
      fetchDocuments: (signal) => fetchDocuments(latest.current.project, signal),
      createWorker: () => new Worker(new URL("./search-worker.ts", import.meta.url), { type: "module" }),
      view,
    })
    controllerRef.current = controller
    controller.start()
    return () => {
      controller.destroy()
      controllerRef.current = null
    }
  }, [changeQuery])

  useEffect(() => {
    registerOpen(controllerRef.current?.open ?? null)
    return () => registerOpen(null)
  }, [registerOpen])

  const open = useCallback((position: MatchPosition, focusDocument: boolean) => {
    const file = resultsRef.current?.files.find((candidate) => candidate.path === position.path)
    const match = file?.matches[position.matchIndex]
    if (!match) return
    setActive(position)
    latest.current.openMatch(position.path, match, focusDocument)
  }, [])

  // F4 and Shift+F4 open the next and previous match, expanding its file.
  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if (event.key !== "F4" || event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) return
      const files = resultsRef.current?.files ?? []
      const next = adjacentMatch(files, activeRef.current, event.shiftKey ? -1 : 1)
      if (!next) return
      event.preventDefault()
      setCollapsed((current) => {
        if (!current.has(next.path)) return current
        const updated = new Set(current)
        updated.delete(next.path)
        return updated
      })
      open(next, false)
    }
    document.addEventListener("keydown", shortcut)
    return () => document.removeEventListener("keydown", shortcut)
  }, [open])

  const toggleFile = useCallback((path: string, expand?: boolean) => {
    setCollapsed((current) => {
      const collapse = expand === undefined ? !current.has(path) : !expand
      if (collapse === current.has(path)) return current
      const updated = new Set(current)
      if (collapse) updated.add(path)
      else updated.delete(path)
      return updated
    })
  }, [])

  const setAllCollapsed = useCallback((collapse: boolean) => {
    setCollapsed(collapse ? new Set(resultsRef.current?.files.map((file) => file.path)) : new Set())
  }, [])

  const clear = useCallback(() => {
    changeQuery({ pattern: "" })
    setFocusRequest((count) => count + 1)
  }, [changeQuery])

  const focusInput = useCallback(() => setFocusRequest((count) => count + 1), [])

  return {
    active, changeQuery, clear, collapsed, details, focusInput, input, open, query, results, setActive,
    setAllCollapsed, setDetails, status, toggleFile,
    refresh: () => handlers.current?.onRefresh(),
    retry: () => handlers.current?.onRetry(),
  }
}

export type SearchSession = ReturnType<typeof useSearchSession>
