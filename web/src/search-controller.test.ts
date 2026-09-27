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
} from "./search-controller.ts"
import type { SearchGroups } from "./search-engine.ts"

class ShortcutEvent extends Event {
  readonly altKey: boolean
  readonly ctrlKey: boolean
  readonly key: string
  readonly metaKey: boolean
  prevented = false

  constructor({ altKey = false, ctrlKey = false, key, metaKey = false }: {
    altKey?: boolean
    ctrlKey?: boolean
    key: string
    metaKey?: boolean
  }) {
    super("keydown")
    this.altKey = altKey
    this.ctrlKey = ctrlKey
    this.key = key
    this.metaKey = metaKey
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
  const navigations: string[] = []
  const rendered: Array<SearchGroups & { activeIndex: number }> = []
  const statuses: SearchStatus[] = []
  const activeSelections: number[] = []
  const calls: string[] = []
  let fetchCount = 0
  let currentHandlers: SearchHandlers | null = null
  const view: SearchView = {
    setHandlers(handlers) { currentHandlers = handlers },
    open() { calls.push("open") },
    close() { calls.push("close") },
    focusInput() { calls.push("focus") },
    clearResults() { calls.push("clear") },
    setStatus(status = {}) { statuses.push(status) },
    renderResults(results) { rendered.push(results) },
    setActive(index) { activeSelections.push(index) },
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
    navigate: (path) => { navigations.push(path) },
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
    activeSelections,
    calls,
    controller,
    fetchCount: () => fetchCount,
    handlers() {
      assert.ok(currentHandlers)
      return currentHandlers
    },
    navigations,
    rendered,
    statuses,
    target,
    timers,
    worker,
    workers,
  }
}

test("Cmd/Ctrl+K opens the palette and loads documents only once", async () => {
  const harness = setup()
  const meta = new ShortcutEvent({ key: "k", metaKey: true })
  const control = new ShortcutEvent({ key: "K", ctrlKey: true })
  harness.target.dispatchEvent(meta)
  harness.target.dispatchEvent(control)
  await flushPromises()

  assert.equal(Number(meta.prevented) + Number(control.prevented), 2)
  assert.equal(harness.fetchCount(), 1)
  assert.equal(harness.worker.messages.filter((message) => message.type === "init").length, 1)
  assert.equal(harness.calls.filter((name) => name === "open").length, 2)
})

test("changing query prevents activation until current results arrive", async () => {
  const harness = setup()
  harness.controller.open()
  await flushPromises()
  harness.worker.emit({ type: "ready" })
  harness.handlers().onInput("guide")
  nextTimer(harness.timers)
  const requestId = lastSearchRequest(harness.worker).requestId
  harness.worker.emit({ type: "results", requestId,
    pathResults: [{ path: "guide.md", name: "guide.md", score: 1 }], contentResults: [] })

  harness.handlers().onInput("other")
  harness.handlers().onActivate()
  harness.handlers().onResultClick(0)
  assert.deepEqual(harness.navigations, [])

  nextTimer(harness.timers)
  harness.worker.emit({ type: "results", requestId: lastSearchRequest(harness.worker).requestId,
    pathResults: [{ path: "other.md", name: "other.md", score: 1 }], contentResults: [] })
  harness.handlers().onActivate()
  assert.deepEqual(harness.navigations, ["other.md"])
})

test("retry replaces a failed worker and preserves the query", async () => {
  const harness = setup()
  harness.controller.open()
  await flushPromises()
  harness.worker.emit({ type: "ready" })
  harness.handlers().onInput("guide")
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
  assert.equal(lastSearchRequest(replacement).query, "guide")
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

test("stale worker responses are ignored and each group is capped at ten", async () => {
  const harness = setup()
  harness.controller.open()
  await flushPromises()
  harness.worker.emit({ type: "ready" })

  harness.handlers().onInput("guide")
  nextTimer(harness.timers)
  const firstRequest = lastSearchRequest(harness.worker).requestId
  harness.handlers().onInput("guides")
  nextTimer(harness.timers)
  const secondRequest = lastSearchRequest(harness.worker).requestId

  harness.worker.emit({
    type: "results",
    requestId: firstRequest,
    pathResults: [{ path: "stale.md", name: "stale.md", score: 1 }],
    contentResults: [],
  })
  assert.equal(harness.rendered.length, 0)

  const pathResults = Array.from({ length: 12 }, (_, index) => ({ path: `path-${index}.md`, name: `path-${index}.md`, score: 1 }))
  const contentResults = Array.from({ length: 12 }, (_, index) => ({ path: `content-${index}.md`, name: `content-${index}.md`, score: 1 }))
  harness.worker.emit({ type: "results", requestId: secondRequest, pathResults, contentResults })

  const results = harness.rendered[0]
  assert.ok(results)
  assert.equal(results.pathResults.length, 10)
  assert.equal(results.contentResults.length, 10)
  assert.equal(results.activeIndex, 0)
})

test("keyboard selection traverses both groups and activation navigates", async () => {
  const harness = setup()
  harness.controller.open()
  await flushPromises()
  harness.worker.emit({ type: "ready" })
  harness.handlers().onInput("guide")
  nextTimer(harness.timers)
  const requestId = lastSearchRequest(harness.worker).requestId
  harness.worker.emit({
    type: "results",
    requestId,
    pathResults: [{ path: "guide.md", name: "guide.md", score: 1 }],
    contentResults: [{ path: "guide.md", name: "guide.md", score: 1, snippet: { text: "guide", highlights: [] } }],
  })

  harness.handlers().onMove(1)
  harness.handlers().onActivate()
  assert.deepEqual(harness.navigations, ["guide.md"])
  assert.equal(harness.activeSelections.at(-1), 1)
})

test("partial index warnings are presented after results", async () => {
  const harness = setup({ warnings: ["unreadable.md: permission denied"] })
  harness.controller.open()
  await flushPromises()
  harness.worker.emit({ type: "ready" })
  harness.handlers().onInput("guide")
  nextTimer(harness.timers)
  const requestId = lastSearchRequest(harness.worker).requestId
  harness.worker.emit({
    type: "results",
    requestId,
    pathResults: [{ path: "guide.md", name: "guide.md", score: 1 }],
    contentResults: [],
  })

  const status = harness.statuses.at(-1)
  assert.ok(status)
  assert.equal(status.kind, "warning")
  assert.match(status.detail || "", /unreadable/)
})
