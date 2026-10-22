import assert from "node:assert/strict"
import test from "node:test"

import { MAIN_THREAD_MAX_CHARACTERS } from "./highlight-thread.ts"
import { createSourceHighlighter, type HighlightWorker } from "./source-highlighter.ts"

class FakeWorker extends EventTarget implements HighlightWorker {
  posted: { requestId: number; source: string; language: string }[] = []
  terminated = false
  postMessage(message: { type: "highlight"; requestId: number; source: string; language: string }): void {
    this.posted.push(message)
  }
  terminate(): void {
    this.terminated = true
  }
  reply(data: unknown): void {
    this.dispatchEvent(new MessageEvent("message", { data }))
  }
}

const large = "x".repeat(MAIN_THREAD_MAX_CHARACTERS + 1)

test("small text highlights on the main thread without a worker", async () => {
  let created = 0
  const highlighter = createSourceHighlighter(() => {
    created++
    return new FakeWorker()
  })
  const lines = await highlighter.highlight("package main\n", "go", new AbortController().signal)
  assert.equal(created, 0)
  assert.equal(lines?.length, 1)
  assert.match(lines?.[0] ?? "", /hl-keyword/)
})

test("large text goes to one shared worker and replies match requests", async () => {
  const workers: FakeWorker[] = []
  const highlighter = createSourceHighlighter(() => {
    const worker = new FakeWorker()
    workers.push(worker)
    return worker
  })
  const first = highlighter.highlight(large, "go", new AbortController().signal)
  const second = highlighter.highlight(large, "python", new AbortController().signal)
  const [worker] = workers
  assert.ok(worker)
  assert.equal(workers.length, 1)
  assert.deepEqual(worker.posted.map((message) => message.language), ["go", "python"])
  worker.reply({ type: "highlighted", requestId: worker.posted[1]?.requestId, lines: ["b"] })
  worker.reply({ type: "highlighted", requestId: worker.posted[0]?.requestId, lines: ["a"] })
  assert.deepEqual(await first, ["a"])
  assert.deepEqual(await second, ["b"])
})

test("aborted, malformed, failed, and released requests show plain text", async () => {
  const workers: FakeWorker[] = []
  const highlighter = createSourceHighlighter(() => {
    const worker = new FakeWorker()
    workers.push(worker)
    return worker
  })
  const controller = new AbortController()
  const aborted = highlighter.highlight(large, "go", controller.signal)
  controller.abort()
  assert.equal(await aborted, null)

  const malformed = highlighter.highlight(large, "go", new AbortController().signal)
  workers[0]?.reply({ type: "highlighted", requestId: workers[0].posted[1]?.requestId, lines: [1] })
  assert.equal(await malformed, null)

  const failed = highlighter.highlight(large, "go", new AbortController().signal)
  workers[0]?.dispatchEvent(new Event("error"))
  assert.equal(await failed, null)
  assert.equal(workers[0]?.terminated, true)

  const released = highlighter.highlight(large, "go", new AbortController().signal)
  assert.equal(workers.length, 2, "the next request starts a fresh worker")
  highlighter.release()
  assert.equal(await released, null)
  assert.equal(workers[1]?.terminated, true)
})
