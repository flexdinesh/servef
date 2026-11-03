import { isRecord } from "../../api/http.ts"
import type { SearchDocument, SearchOutcome, SearchQuery } from "./search-engine.ts"
import type { SearchDocumentsResponse } from "./search-documents.ts"
import { isSearchOutcome } from "./search-results.ts"

export type { SearchDocumentsResponse } from "./search-documents.ts"

export type SearchResults = Extract<SearchOutcome, { kind: "results" }>

export interface SearchStatus {
  detail?: string
  field?: "exclude" | "include" | "pattern"
  kind?: "empty" | "error" | "idle" | "invalid" | "loading" | "searching" | "warning"
  message?: string
}

export interface SearchHandlers {
  onInput(query: SearchQuery): void
  onRefresh(): void
  onRetry(): void
  // onStale reports changed files on disk (decision 0050); shown says the
  // search view is visible.
  onStale(shown: boolean): void
}

export interface SearchView {
  clearResults(): void
  focusInput(): void
  // open reveals the search view; the view may seed the query.
  open(): void
  renderResults(results: SearchResults): void
  setHandlers(handlers: SearchHandlers | null): void
  setStatus(status?: SearchStatus): void
}

export interface SearchWorker extends EventTarget {
  postMessage(message: SearchWorkerRequest): void
  terminate?(): void
}

export type SearchWorkerRequest =
  | { documents: SearchDocument[]; type: "init" }
  | { query: SearchQuery; requestId: number; type: "search" }

type TimerHandle = number | ReturnType<typeof globalThis.setTimeout>

export interface SearchControllerOptions {
  clearTimer?: (timer: TimerHandle) => void
  debounceMs?: number
  fetchDocuments: (signal: AbortSignal) => Promise<SearchDocumentsResponse>
  createWorker: () => SearchWorker
  setTimer?: (callback: () => void, delay: number) => TimerHandle
  shortcutTarget?: EventTarget
  view: SearchView
}

export const emptyQuery: SearchQuery = { pattern: "", include: "", exclude: "", matchCase: false, wholeWord: false, regex: false }

function isShortcutEvent(event: Event): event is Event & {
  altKey: boolean
  ctrlKey: boolean
  key: string
  metaKey: boolean
  shiftKey: boolean
} {
  return "key" in event
    && typeof event.key === "string"
    && "metaKey" in event
    && typeof event.metaKey === "boolean"
    && "ctrlKey" in event
    && typeof event.ctrlKey === "boolean"
    && "altKey" in event
    && typeof event.altKey === "boolean"
    && "shiftKey" in event
    && typeof event.shiftKey === "boolean"
}

function scopeDescription(query: SearchQuery): string {
  const include = query.include.trim()
  const exclude = query.exclude.trim()
  if (include && exclude) return ` in '${include}' excluding '${exclude}'`
  if (include) return ` in '${include}'`
  if (exclude) return ` excluding '${exclude}'`
  return ""
}

export function createSearchController({
  view,
  createWorker,
  fetchDocuments,
  shortcutTarget = globalThis.document,
  debounceMs = 150,
  setTimer = globalThis.setTimeout.bind(globalThis),
  clearTimer = globalThis.clearTimeout.bind(globalThis),
}: SearchControllerOptions) {
  if (!view || !createWorker || !fetchDocuments || !shortcutTarget) {
    throw new TypeError("Search controller dependencies are required")
  }

  let started = false
  let loading: Promise<void> | undefined
  let worker: SearchWorker | undefined
  let loadController: AbortController | undefined
  let ready = false
  let unavailable = false
  // stale marks loaded documents that changed on disk since.
  let stale = false
  let warnings: string[] = []
  let query = emptyQuery
  let debounceTimer: TimerHandle | undefined
  let nextRequestId = 0
  let activeRequestId = 0

  const cancelTimer = (): void => {
    if (debounceTimer !== undefined) clearTimer(debounceTimer)
    debounceTimer = undefined
  }

  const renderEmptyQuery = (): void => {
    view.clearResults()
    view.setStatus({ kind: "idle" })
  }

  const fail = (message = "Search is unavailable."): void => {
    if (!started) return
    ready = false
    unavailable = true
    loadController?.abort()
    loading = undefined
    disposeWorker()
    cancelTimer()
    activeRequestId = ++nextRequestId
    view.clearResults()
    view.setStatus({ kind: "error", message })
  }

  const requestSearch = (requested: SearchQuery, requestId: number): void => {
    if (!started || !ready || unavailable || requestId !== activeRequestId || requested !== query || !requested.pattern) return
    try {
      worker?.postMessage({ type: "search", requestId, query: requested })
    } catch {
      fail()
    }
  }

  // scheduleSearch keeps the previous results visible until new ones arrive.
  const scheduleSearch = (next: SearchQuery): void => {
    if (!started) return
    query = next
    activeRequestId = ++nextRequestId
    cancelTimer()

    if (unavailable) {
      view.setStatus({ kind: "error", message: "Search is unavailable." })
      return
    }
    if (!query.pattern) {
      renderEmptyQuery()
      return
    }
    if (!ready) {
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
    if (stale) reload()
    else void ensureLoaded()
  }

  // reload fetches documents again, as files may have changed on disk.
  const reload = (): void => {
    if (!started) return
    stale = false
    unavailable = false
    ready = false
    loadController?.abort()
    loading = undefined
    disposeWorker()
    cancelTimer()
    activeRequestId = ++nextRequestId
    void ensureLoaded()
  }

  // markStale rereads changed documents now while the view shows, else when
  // search next opens. Documents never loaded stay unloaded.
  const markStale = (shown: boolean): void => {
    if (!started || !loading) return
    if (shown) reload()
    else stale = true
  }

  const retry = (): void => {
    if (!started || !unavailable) return
    reload()
    view.focusInput()
  }

  const onShortcut = (event: Event): void => {
    if (isShortcutEvent(event) && (event.metaKey || event.ctrlKey) && event.shiftKey && !event.altKey && event.key.toLowerCase() === "f") {
      event.preventDefault()
      open()
    }
  }

  const presentOutcome = (outcome: SearchOutcome): void => {
    if (outcome.kind === "invalid") {
      view.clearResults()
      view.setStatus({ kind: "invalid", field: outcome.field, message: outcome.message })
      return
    }
    view.renderResults(outcome)
    if (!outcome.files.length) {
      view.setStatus({ kind: "empty", message: `No results found${scopeDescription(query)}.` })
    } else if (warnings.length) {
      view.setStatus({
        kind: "warning",
        message: "Results may be incomplete because some files could not be read.",
        detail: warnings.join("\n"),
      })
    } else if (outcome.truncated) {
      view.setStatus({
        kind: "warning",
        message: "The result set only contains a subset of all matches. Be more specific in your search to narrow down the results.",
      })
    } else {
      view.setStatus({ kind: "idle" })
    }
  }

  const onWorkerMessage = (event: Event): void => {
    if (!started || event.target !== worker || !(event instanceof MessageEvent) || !isRecord(event.data)) return
    const message = event.data
    if (message.type === "ready") {
      ready = true
      unavailable = false
      if (query.pattern) scheduleSearch(query)
      else renderEmptyQuery()
      return
    }
    if (message.type === "error") {
      if (message.requestId !== undefined && message.requestId !== activeRequestId) return
      fail()
      return
    }
    if (message.type !== "results" || message.requestId !== activeRequestId) return
    if (!isSearchOutcome(message.outcome)) {
      fail()
      return
    }
    presentOutcome(message.outcome)
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
      onRefresh: reload,
      onRetry: retry,
      onStale: markStale,
    })
    shortcutTarget.addEventListener("keydown", onShortcut)
  }

  const destroy = (): void => {
    if (!started) return
    cancelTimer()
    started = false
    loadController?.abort()
    loading = undefined
    ready = false
    unavailable = false
    shortcutTarget.removeEventListener("keydown", onShortcut)
    disposeWorker()
    view.setHandlers(null)
  }

  return { start, destroy, open }
}
