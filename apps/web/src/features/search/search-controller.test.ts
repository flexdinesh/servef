import assert from "node:assert/strict"
import test from "node:test"

import {
  createSearchController,
  type SearchHandlers,
  type SearchStatus,
  type SearchView,
  type SearchWorker,
  type SearchWorkerRequest,
  type SearchDocumentsResponse,
  type SearchResults,
  emptyQuery,
} from "./search-controller.ts"
import type { SearchQuery } from "./search-engine.ts"

class ShortcutEvent extends Event {
  readonly altKey: boolean
  readonly ctrlKey: boolean
  readonly key: string
  readonly metaKey: boolean
  readonly shiftKey: boolean
  prevented = false

  constructor({ altKey = false, ctrlKey = false, key, metaKey = false, shiftKey = false }: {
    altKey?: boolean
    ctrlKey?: boolean
    key: string
    metaKey?: boolean
    shiftKey?: boolean
  }) {
    super("keydown")
    this.altKey = altKey
    this.ctrlKey = ctrlKey
    this.key = key
    this.metaKey = metaKey
    this.shiftKey = shiftKey
  }

  preventDefault(): void {
    this.prevented = true
  }
}

class WorkerFake extends EventTarget implements SearchWorker {
  messages: SearchWorkerRequest[] = []
  terminated = false

  terminate(): void {
    this.terminated = true
  }

  postMessage(message: SearchWorkerRequest): void {
    this.messages.push(message)
  }

  emit(data: unknown): void {
    this.dispatchEvent(new MessageEvent("message", { data }))
  }
}

function nextTimer(timers: Array<() => void>): void {
  const callback = timers.shift()
  assert.ok(callback)
  callback()
}

async function flushPromises(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

function lastSearchRequest(worker: WorkerFake): Extract<SearchWorkerRequest, { type: "search" }> {
  const message = worker.messages.at(-1)
  assert.ok(message)
  assert.equal(message.type, "search")
  if (message.type !== "search") throw new Error("expected search request")
  return message
}

function setup({ warnings = [], fetchDocuments }: {
  warnings?: string[]
  fetchDocuments?(signal: AbortSignal): Promise<SearchDocumentsResponse>
} = {}) {
  const target = new EventTarget()
  const worker = new WorkerFake()
  const workers: WorkerFake[] = []
  const timers: Array<() => void> = []
  const scheduledTimers = new Map<number, () => void>()
  let timerID = 0
  const rendered: SearchResults[] = []
  const statuses: SearchStatus[] = []
  const calls: string[] = []
  let fetchCount = 0
  let currentHandlers: SearchHandlers | null = null
  const view: SearchView = {
    setHandlers(handlers) { currentHandlers = handlers },
    open() { calls.push("open") },
    focusInput() { calls.push("focus") },
    clearResults() { calls.push("clear") },
    setStatus(status = {}) { statuses.push(status) },
    renderResults(results) { rendered.push(results) },
  }
  const controller = createSearchController({
    view,
    createWorker: () => {
      const next = workers.length === 0 ? worker : new WorkerFake()
      workers.push(next)
      return next
    },
    shortcutTarget: target,
    fetchDocuments: async (signal) => {
      fetchCount += 1
      if (fetchDocuments) return fetchDocuments(signal)
      return { documents: [{ path: "guide.md", name: "guide.md", content: "Guide body" }], warnings }
    },
    setTimer: (callback) => {
      timers.push(callback)
      scheduledTimers.set(++timerID, callback)
      return timerID
    },
    clearTimer: (timer) => {
      if (typeof timer !== "number") return
      const callback = scheduledTimers.get(timer)
      if (!callback) return
      const index = timers.indexOf(callback)
      if (index >= 0) timers.splice(index, 1)
      scheduledTimers.delete(timer)
    },
  })
  controller.start()
  return {
    calls,
    controller,
    fetchCount: () => fetchCount,
    handlers() {
      assert.ok(currentHandlers)
      return currentHandlers
    },
    rendered,
    statuses,
    target,
    timers,
    worker,
    workers,
  }
}

function query(pattern: string, options: Partial<SearchQuery> = {}): SearchQuery {
  return { ...emptyQuery, pattern, ...options }
}

function outcome(paths: string[], truncated = false): SearchResults {
  return {
    kind: "results",
    files: paths.map((path) => ({ path, name: path, matches: [{ line: 1, column: 0, endColumn: 1, preview: "x", highlight: { start: 0, end: 1 } }] })),
    matchCount: paths.length,
    truncated,
  }
}

async function readyHarness(options: Parameters<typeof setup>[0] = {}) {
  const harness = setup(options)
  harness.controller.open()
  await flushPromises()
  harness.worker.emit({ type: "ready" })
  return harness
}

test("Cmd/Ctrl+Shift+F reveals content search and loads documents only once", async () => {
  const harness = setup()
  const meta = new ShortcutEvent({ key: "f", metaKey: true, shiftKey: true })
  const control = new ShortcutEvent({ key: "F", ctrlKey: true, shiftKey: true })
  harness.target.dispatchEvent(meta)
  harness.target.dispatchEvent(control)
  await flushPromises()

  assert.equal(Number(meta.prevented) + Number(control.prevented), 2)
  assert.equal(harness.fetchCount(), 1)
  assert.equal(harness.worker.messages.filter((message) => message.type === "init").length, 1)
  assert.deepEqual(harness.calls.filter((name) => name === "open" || name === "focus"), ["open", "focus", "open", "focus"])
})

test("file shortcuts and unmodified Find do not open content search", () => {
  const harness = setup()
  for (const event of [
    new ShortcutEvent({ key: "k", ctrlKey: true }),
    new ShortcutEvent({ key: "p", metaKey: true }),
    new ShortcutEvent({ key: "f", ctrlKey: true }),
    new ShortcutEvent({ key: "f", ctrlKey: true, shiftKey: true, altKey: true }),
  ]) {
    harness.target.dispatchEvent(event)
    assert.equal(event.prevented, false)
  }
  assert.equal(harness.calls.includes("open"), false)
})

test("queries carry their options and stale responses are ignored", async () => {
  const harness = await readyHarness()
  harness.handlers().onInput(query("guide"))
  nextTimer(harness.timers)
  const first = lastSearchRequest(harness.worker).requestId
  harness.handlers().onInput(query("guide", { matchCase: true, include: "*.md" }))
  nextTimer(harness.timers)
  const second = lastSearchRequest(harness.worker)
  assert.deepEqual(second.query, query("guide", { matchCase: true, include: "*.md" }))

  harness.worker.emit({ type: "results", requestId: first, outcome: outcome(["stale.md"]) })
  assert.equal(harness.rendered.length, 0)
  harness.worker.emit({ type: "results", requestId: second.requestId, outcome: outcome(["guide.md"]) })
  assert.deepEqual(harness.rendered.map((results) => results.files.map((file) => file.path)), [["guide.md"]])
  assert.equal(harness.statuses.at(-1)?.kind, "idle")
})

test("typing keeps previous results until new ones arrive; an empty pattern clears them", async () => {
  const harness = await readyHarness()
  harness.handlers().onInput(query("guide"))
  nextTimer(harness.timers)
  harness.worker.emit({ type: "results", requestId: lastSearchRequest(harness.worker).requestId, outcome: outcome(["guide.md"]) })
  const clears = harness.calls.filter((name) => name === "clear").length
  harness.handlers().onInput(query("guides"))
  assert.equal(harness.calls.filter((name) => name === "clear").length, clears)
  assert.equal(harness.statuses.at(-1)?.kind, "searching")
  harness.handlers().onInput(query(""))
  assert.equal(harness.calls.filter((name) => name === "clear").length, clears + 1)
  assert.equal(harness.statuses.at(-1)?.kind, "idle")
})

test("invalid patterns, empty results, truncation, and malformed outcomes report status", async () => {
  const harness = await readyHarness()
  const respond = (value: unknown) => {
    nextTimer(harness.timers)
    harness.worker.emit({ type: "results", requestId: lastSearchRequest(harness.worker).requestId, outcome: value })
  }
  harness.handlers().onInput(query("(", { regex: true }))
  respond({ kind: "invalid", field: "pattern", message: "Invalid regular expression." })
  assert.deepEqual(harness.statuses.at(-1), { kind: "invalid", field: "pattern", message: "Invalid regular expression." })

  harness.handlers().onInput(query("nothing", { include: "src", exclude: "*.go" }))
  respond(outcome([]))
  assert.deepEqual(harness.statuses.at(-1), { kind: "empty", message: "No results found in 'src' excluding '*.go'." })

  harness.handlers().onInput(query("many"))
  respond(outcome(["a.md"], true))
  assert.equal(harness.statuses.at(-1)?.kind, "warning")
  assert.match(harness.statuses.at(-1)?.message ?? "", /subset of all matches/)

  harness.handlers().onInput(query("broken"))
  respond({ kind: "results" })
  assert.equal(harness.statuses.at(-1)?.kind, "error")
})

test("retry replaces a failed worker and preserves the query", async () => {
  const harness = await readyHarness()
  harness.handlers().onInput(query("guide"))
  harness.worker.dispatchEvent(new Event("error"))
  assert.equal(harness.statuses.at(-1)?.kind, "error")

  harness.handlers().onRetry()
  await flushPromises()
  assert.equal(harness.worker.terminated, true)
  const replacement = harness.workers.at(-1)
  assert.ok(replacement)
  assert.notEqual(replacement, harness.worker)
  replacement.emit({ type: "ready" })
  nextTimer(harness.timers)
  assert.equal(lastSearchRequest(replacement).query.pattern, "guide")
})

test("retry fetches again after a transient document failure", async () => {
  let attempts = 0
  const harness = setup({ fetchDocuments: async () => {
    if (++attempts === 1) throw new Error("offline")
    return { documents: [], warnings: [] }
  } })
  harness.controller.open()
  await flushPromises()
  await flushPromises()
  assert.equal(harness.statuses.at(-1)?.kind, "error")
  harness.handlers().onRetry()
  await flushPromises()
  assert.equal(attempts, 2)
  harness.workers.at(-1)?.emit({ type: "ready" })
  assert.equal(harness.statuses.at(-1)?.kind, "idle")
})

test("changed files reread a shown search now and rerun its query", async () => {
  const harness = await readyHarness()
  harness.handlers().onInput(query("guide"))
  harness.handlers().onStale(true)
  await flushPromises()
  assert.equal(harness.fetchCount(), 2)
  const replacement = harness.workers.at(-1)
  assert.ok(replacement && replacement !== harness.worker)
  replacement.emit({ type: "ready" })
  nextTimer(harness.timers)
  assert.equal(lastSearchRequest(replacement).query.pattern, "guide")
})

test("changed files reread a hidden search only when it opens again", async () => {
  const harness = await readyHarness()
  harness.handlers().onStale(false)
  harness.handlers().onStale(false)
  await flushPromises()
  assert.equal(harness.fetchCount(), 1)
  harness.controller.open()
  await flushPromises()
  assert.equal(harness.fetchCount(), 2)
  harness.controller.open()
  await flushPromises()
  assert.equal(harness.fetchCount(), 2, "one reread clears the staleness")
})

test("changed files do not load a search never opened", async () => {
  const harness = setup()
  harness.handlers().onStale(true)
  await flushPromises()
  assert.equal(harness.fetchCount(), 0)
})

test("refresh fetches documents again and reruns the current query", async () => {
  const harness = await readyHarness()
  harness.handlers().onInput(query("guide"))
  harness.handlers().onRefresh()
  await flushPromises()
  assert.equal(harness.fetchCount(), 2)
  assert.equal(harness.worker.terminated, true)
  const replacement = harness.workers.at(-1)
  assert.ok(replacement)
  replacement.emit({ type: "ready" })
  nextTimer(harness.timers)
  assert.equal(lastSearchRequest(replacement).query.pattern, "guide")
})

test("destroy aborts loading and ignores late settlements", async () => {
  let finish: ((payload: SearchDocumentsResponse) => void) | undefined
  let requestSignal: AbortSignal | undefined
  const harness = setup({ fetchDocuments: (signal) => {
    requestSignal = signal
    return new Promise((resolve) => { finish = resolve })
  } })
  harness.controller.open()
  await flushPromises()
  harness.controller.destroy()
  assert.equal(requestSignal?.aborted, true)
  assert.equal(harness.worker.terminated, true)
  const statusCount = harness.statuses.length
  assert.ok(finish)
  finish({ documents: [], warnings: [] })
  await flushPromises()
  assert.equal(harness.worker.messages.length, 0)
  assert.equal(harness.statuses.length, statusCount)
})

test("partial index warnings are presented after results", async () => {
  const harness = await readyHarness({ warnings: ["unreadable.md: permission denied"] })
  harness.handlers().onInput(query("guide"))
  nextTimer(harness.timers)
  harness.worker.emit({ type: "results", requestId: lastSearchRequest(harness.worker).requestId, outcome: outcome(["guide.md"]) })

  const status = harness.statuses.at(-1)
  assert.ok(status)
  assert.equal(status.kind, "warning")
  assert.match(status.detail || "", /unreadable/)
})
