import type { SearchDocument, SearchGroups, SearchResult } from "./search-engine.ts"
import type { SearchDocumentsResponse } from "./search-documents.ts"

export type { SearchDocumentsResponse } from "./search-documents.ts"

export interface SearchStatus {
  detail?: string
  kind?: "empty" | "error" | "idle" | "loading" | "searching" | "warning"
  message?: string
}

export interface SearchHandlers {
  onActivate(index?: number): void
  onClose(): void
  onInput(query: string): void
  onMove(delta: number): void
  onResultClick(index: number): void
  onRetry(): void
}

export interface SearchView {
  clearResults(): void
  close(): void
  focusInput(): void
  open(): void
  renderResults(results: SearchGroups & { activeIndex: number }): void
  setActive(index: number, scroll?: boolean): void
  setHandlers(handlers: SearchHandlers | null): void
  setStatus(status?: SearchStatus): void
}

export interface SearchWorker extends EventTarget {
  postMessage(message: SearchWorkerRequest): void
  terminate?(): void
}

export type SearchWorkerRequest =
  | { documents: SearchDocument[]; type: "init" }
  | { query: string; requestId: number; type: "search" }

type TimerHandle = number | ReturnType<typeof globalThis.setTimeout>

export interface SearchControllerOptions {
  clearTimer?: (timer: TimerHandle) => void
  debounceMs?: number
  fetchDocuments: (signal: AbortSignal) => Promise<SearchDocumentsResponse>
  createWorker: () => SearchWorker
  navigate: (path: string) => void
  setTimer?: (callback: () => void, delay: number) => TimerHandle
  shortcutTarget?: EventTarget
  view: SearchView
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

function isShortcutEvent(event: Event): event is Event & {
  altKey: boolean
  ctrlKey: boolean
  key: string
  metaKey: boolean
} {
  return "key" in event
    && typeof event.key === "string"
    && "metaKey" in event
    && typeof event.metaKey === "boolean"
    && "ctrlKey" in event
    && typeof event.ctrlKey === "boolean"
    && "altKey" in event
    && typeof event.altKey === "boolean"
}

function isSearchResult(value: unknown): value is SearchResult {
  return isRecord(value)
    && typeof value.path === "string"
    && typeof value.name === "string"
    && typeof value.score === "number"
}

export function createSearchController({
  view,
  createWorker,
  fetchDocuments,
  navigate,
  shortcutTarget = globalThis.document,
  debounceMs = 120,
  setTimer = globalThis.setTimeout.bind(globalThis),
  clearTimer = globalThis.clearTimeout.bind(globalThis),
}: SearchControllerOptions) {
  if (!view || !createWorker || !fetchDocuments || !navigate || !shortcutTarget) {
    throw new TypeError("Search controller dependencies are required")
  }

  let started = false
  let loading: Promise<void> | undefined
  let worker: SearchWorker | undefined
  let loadController: AbortController | undefined
  let ready = false
  let unavailable = false
  let warnings: string[] = []
  let query = ""
  let debounceTimer: TimerHandle | undefined
  let nextRequestId = 0
  let activeRequestId = 0
  let flattenedResults: SearchResult[] = []
  let activeIndex = -1

  const renderEmptyQuery = (): void => {
    flattenedResults = []
    activeIndex = -1
    view.clearResults()
    view.setStatus({ kind: "idle", message: "Type to search file paths and content." })
  }

  const fail = (message = "Search is unavailable."): void => {
    if (!started) return
    ready = false
    unavailable = true
    loadController?.abort()
    loading = undefined
    disposeWorker()
    if (debounceTimer !== undefined) clearTimer(debounceTimer)
    debounceTimer = undefined
    activeRequestId = ++nextRequestId
    flattenedResults = []
    activeIndex = -1
    view.clearResults()
    view.setStatus({ kind: "error", message })
  }

  const requestSearch = (requestedQuery: string, requestId: number): void => {
    if (!started || !ready || unavailable || requestId !== activeRequestId || requestedQuery !== query || !requestedQuery.trim()) return
    view.setStatus({ kind: "searching", message: "Searching…" })
    try {
      worker?.postMessage({ type: "search", requestId, query: requestedQuery })
    } catch {
      fail()
    }
  }

  const scheduleSearch = (nextQuery: string): void => {
    if (!started) return
    query = nextQuery
    activeRequestId = ++nextRequestId
    if (debounceTimer !== undefined) clearTimer(debounceTimer)
    debounceTimer = undefined
    flattenedResults = []
    activeIndex = -1
    view.clearResults()

    if (unavailable) {
      view.setStatus({ kind: "error", message: "Search is unavailable." })
      return
    }
    if (!query.trim()) {
      renderEmptyQuery()
      return
    }
    if (!ready) {
      view.clearResults()
      view.setStatus({ kind: "loading", message: "Preparing search…" })
      return
    }

    const requestId = activeRequestId
    view.setStatus({ kind: "searching", message: "Searching…" })
    debounceTimer = setTimer(() => {
      debounceTimer = undefined
      requestSearch(query, requestId)
    }, debounceMs)
  }

  const ensureLoaded = (): Promise<void> => {
    if (loading) return loading

    view.setStatus({ kind: "loading", message: "Preparing search…" })
    const controller = new AbortController()
    loadController = controller
    try {
      worker = createWorker()
      worker.addEventListener("message", onWorkerMessage)
      worker.addEventListener("error", onWorkerError)
    } catch {
      fail()
      return Promise.resolve()
    }
    const loadingWorker = worker
    loading = Promise.resolve()
      .then(() => fetchDocuments(controller.signal))
      .then((payload) => {
        if (!started || controller.signal.aborted) return
        warnings = payload.warnings
        loadingWorker.postMessage({ type: "init", documents: payload.documents })
      })
      .catch(() => { if (!controller.signal.aborted) fail() })
    return loading
  }

  const open = (): void => {
    if (!started) return
    view.open()
    view.focusInput()
    if (unavailable) {
      fail()
      return
    }
    void ensureLoaded()
    if (ready) scheduleSearch(query)
  }

  const retry = (): void => {
    if (!started || !unavailable) return
    unavailable = false
    void ensureLoaded()
    view.focusInput()
  }

  const close = (): void => {
    if (debounceTimer !== undefined) clearTimer(debounceTimer)
    debounceTimer = undefined
    view.close()
  }

  const moveSelection = (delta: number): void => {
    if (!flattenedResults.length) return
    activeIndex = (activeIndex + delta + flattenedResults.length) % flattenedResults.length
    view.setActive(activeIndex)
  }

  const activate = (index = activeIndex): void => {
    const result = flattenedResults[index]
    if (result) navigate(result.path)
  }

  const onShortcut = (event: Event): void => {
    if (isShortcutEvent(event) && (event.metaKey || event.ctrlKey) && !event.altKey && event.key.toLowerCase() === "k") {
      event.preventDefault()
      open()
    }
  }

  const onWorkerMessage = (event: Event): void => {
    if (!started || event.target !== worker || !(event instanceof MessageEvent) || !isRecord(event.data)) return
    const message = event.data
    if (message.type === "ready") {
      ready = true
      unavailable = false
      if (query.trim()) scheduleSearch(query)
      else renderEmptyQuery()
      return
    }
    if (message.type === "error") {
      if (message.requestId !== undefined && message.requestId !== activeRequestId) return
      fail()
      return
    }
    if (message.type !== "results" || message.requestId !== activeRequestId) return

    const groups = {
      pathResults: Array.isArray(message.pathResults) ? message.pathResults.filter(isSearchResult).slice(0, 10) : [],
      contentResults: Array.isArray(message.contentResults) ? message.contentResults.filter(isSearchResult).slice(0, 10) : [],
    }
    flattenedResults = [...groups.pathResults, ...groups.contentResults]
    activeIndex = flattenedResults.length ? 0 : -1
    view.renderResults({ ...groups, activeIndex })

    if (warnings.length) {
      view.setStatus({
        kind: "warning",
        message: "Results may be incomplete because some files could not be read.",
        detail: warnings.join("\n"),
      })
    } else if (!flattenedResults.length) {
      view.setStatus({ kind: "empty", message: "No matches found." })
    } else {
      view.setStatus()
    }
  }

  const onWorkerError = (): void => fail()

  function disposeWorker(): void {
    worker?.removeEventListener("message", onWorkerMessage)
    worker?.removeEventListener("error", onWorkerError)
    worker?.terminate?.()
    worker = undefined
  }

  const start = (): void => {
    if (started) return
    started = true
    view.setHandlers({
      onInput: scheduleSearch,
      onMove: moveSelection,
      onActivate: activate,
      onClose: close,
      onResultClick: activate,
      onRetry: retry,
    })
    shortcutTarget.addEventListener("keydown", onShortcut)
  }

  const destroy = (): void => {
    if (!started) return
    close()
    started = false
    loadController?.abort()
    loading = undefined
    ready = false
    unavailable = false
    shortcutTarget.removeEventListener("keydown", onShortcut)
    disposeWorker()
    view.setHandlers(null)
  }

  return { start, destroy, open, close }
}
