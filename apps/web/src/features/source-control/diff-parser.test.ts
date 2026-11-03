import assert from "node:assert/strict"
import test from "node:test"

import type { FileContents } from "@pierre/diffs"

import { parseTextDiff, type DiffContent } from "./diff-content.ts"
import { createDiffParser, type DiffWorker, type DiffWorkerRequest } from "./diff-parser.ts"
import { diffStats } from "./diff-stats.ts"

class WorkerFake extends EventTarget implements DiffWorker {
  messages: DiffWorkerRequest[] = []
  terminated = false

  postMessage(message: DiffWorkerRequest): void {
    this.messages.push(message)
  }

  terminate(): void {
    this.terminated = true
  }

  emit(data: unknown): void {
    this.dispatchEvent(new MessageEvent("message", { data }))
  }

  // reply answers a request as diff-worker.ts does.
  reply(request: DiffWorkerRequest | undefined): void {
    assert.ok(request)
    const content: DiffContent = { kind: "text", oldFile: request.oldFile, newFile: request.newFile }
    this.emit({ type: "parsed", requestId: request.requestId, fileDiff: parseTextDiff(content, request.ignoreWhitespace) })
  }
}

function file(contents: string): FileContents {
  return { name: "main.go", contents, lang: "go" }
}

function setup() {
  const workers: WorkerFake[] = []
  const parser = createDiffParser(() => {
    const worker = new WorkerFake()
    workers.push(worker)
    return worker
  })
  return { parser, workers }
}

test("one lazily started worker parses every diff, honoring ignore whitespace", async () => {
  const { parser, workers } = setup()
  assert.equal(workers.length, 0)
  const before = file("one\ntwo\nthree\n")
  const after = file("one\n  two\nfour\n")

  const plain = parser.parse(before, after, false, new AbortController().signal)
  const trimmed = parser.parse(before, after, true, new AbortController().signal)
  assert.equal(workers.length, 1)
  const [worker] = workers
  assert.ok(worker)
  assert.deepEqual(worker.messages.map((message) => message.ignoreWhitespace), [false, true])
  worker.reply(worker.messages[1])
  worker.reply(worker.messages[0])
  assert.deepEqual(diffStats(await plain), { additions: 2, deletions: 2 })
  assert.deepEqual(diffStats(await trimmed), { additions: 1, deletions: 1 })
})

test("unknown, settled, and malformed replies never resolve the wrong diff", async () => {
  const { parser, workers } = setup()
  const parse = parser.parse(null, file("a\n"), false, new AbortController().signal)
  const worker = workers[0]
  assert.ok(worker)
  const request = worker.messages[0]
  assert.ok(request)
  worker.emit({ type: "parsed", requestId: request.requestId + 1, fileDiff: null })
  worker.emit({ type: "parsed", requestId: request.requestId, fileDiff: { name: "main.go", hunks: "nope" } })
  await assert.rejects(parse, /Couldn't compute this diff/)
  // A late duplicate for a settled request is ignored.
  worker.reply(request)

  const failed = parser.parse(file("a\n"), file("b\n"), false, new AbortController().signal)
  worker.emit({ type: "error", requestId: worker.messages[1]?.requestId })
  await assert.rejects(failed, /Couldn't compute this diff/)
  assert.equal(workers.length, 1)
})

test("aborting a pending parse replaces the busy worker and keeps queued parses", async () => {
  const { parser, workers } = setup()
  const stale = new AbortController()
  const abandoned = parser.parse(file("old\n"), file("older\n"), false, stale.signal)
  const kept = parser.parse(file("a\n"), file("b\n"), false, new AbortController().signal)
  const [busy] = workers
  assert.ok(busy)

  stale.abort()
  await assert.rejects(abandoned, { name: "AbortError" })
  assert.equal(busy.terminated, true)
  const fresh = workers[1]
  assert.ok(fresh)
  assert.deepEqual(fresh.messages.map((message) => message.newFile?.contents), ["b\n"])

  // The terminated worker's reply is discarded; the fresh one answers.
  busy.reply(busy.messages[1])
  fresh.reply(fresh.messages[0])
  assert.deepEqual(diffStats(await kept), { additions: 1, deletions: 1 })

  const done = new AbortController()
  const parsed = parser.parse(file("x\n"), file("y\n"), false, done.signal)
  fresh.reply(fresh.messages[1])
  await parsed
  done.abort()
  assert.equal(fresh.terminated, false, "a settled parse leaves the worker running")
  await assert.rejects(parser.parse(null, file("z\n"), false, done.signal), { name: "AbortError" })
})

test("a failed worker rejects its parses and the next parse starts another", async () => {
  const { parser, workers } = setup()
  const pending = parser.parse(null, file("a\n"), false, new AbortController().signal)
  const [crashed] = workers
  assert.ok(crashed)
  crashed.dispatchEvent(new Event("error"))
  await assert.rejects(pending, /diff worker failed/)
  assert.equal(crashed.terminated, true)

  const next = parser.parse(null, file("a\n"), false, new AbortController().signal)
  const replacement = workers[1]
  assert.ok(replacement)
  replacement.reply(replacement.messages[0])
  assert.deepEqual(diffStats(await next), { additions: 1, deletions: 0 })
})

test("release ends the worker and its parses; the next parse starts another", async () => {
  const { parser, workers } = setup()
  parser.release()
  assert.equal(workers.length, 0)
  const pending = parser.parse(file("one\n"), file("two\n"), false, new AbortController().signal)
  const [first] = workers
  assert.ok(first)
  parser.release()
  assert.equal(first.terminated, true)
  await assert.rejects(pending, { name: "AbortError" })

  const next = parser.parse(file("one\n"), file("two\n"), false, new AbortController().signal)
  const [, second] = workers
  assert.ok(second)
  second.reply(second.messages[0])
  assert.equal((await next).hunks.length, 1)
})
