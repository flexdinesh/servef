import assert from "node:assert/strict"
import { test } from "node:test"
import type { DaemonConnection } from "../src/main/connections.ts"
import type { ForwardRequest } from "../src/main/forward.ts"
import { BACKOFF_MAX_MS, BACKOFF_MIN_MS, createSubscriptions, MAX_WATCHED } from "../src/main/subscriptions.ts"
import type { SubscriptionEvent } from "../src/main/subscriptions.ts"
import { createFakeTimers, flush } from "./fake-timers.ts"

const connection: DaemonConnection = {
  id: "local", kind: "daemon", origin: "servef-app://local", socket: "/run/servef.sock",
  status: { protocol: 1, version: "1", pid: 1, socket: "/run/servef.sock", roots: [] }, label: "local",
}

interface FakeStream {
  request: ForwardRequest
  controller: ReadableStreamDefaultController<Uint8Array> | null
  cancelled: boolean
  ended: boolean
  send(name: string, data: unknown): void
  write(text: string): void
  end(): void
}

function setup(options: { connection?: () => DaemonConnection | null } = {}) {
  const clock = createFakeTimers()
  const streams: FakeStream[] = []
  const failures: Error[] = []
  const events: SubscriptionEvent[] = []
  let reensures = 0
  const subscriptions = createSubscriptions({
    connection: async () => (options.connection ?? (() => connection))(),
    reensure: async () => {
      reensures += 1
      return true
    },
    timers: clock.timers,
    onEvent: (event) => events.push(event),
    forward: async (request) => {
      const failure = failures.shift()
      if (failure !== undefined) throw failure
      const stream: FakeStream = {
        request,
        controller: null,
        cancelled: false,
        ended: false,
        write(text) { stream.controller?.enqueue(new TextEncoder().encode(text)) },
        send(name, data) { stream.write(`event: ${name}\ndata: ${typeof data === "string" ? data : JSON.stringify(data)}\n\n`) },
        end() {
          stream.ended = true
          stream.controller?.close()
        },
      }
      const body = new ReadableStream<Uint8Array>({
        start(controller) { stream.controller = controller },
        cancel() { stream.cancelled = true },
      })
      streams.push(stream)
      return new Response(body, { headers: { "content-type": "text/event-stream" } })
    },
  })
  const live = () => streams.filter((stream) => !stream.cancelled && !stream.ended).map((stream) => stream.request.path)
  const latest = (path: string) => {
    const stream = streams.findLast((each) => each.request.path === path)
    assert.ok(stream !== undefined, `no stream for ${path}`)
    return stream
  }
  return { subscriptions, clock, streams, failures, events, live, latest, reensures: () => reensures }
}

const CONTROL = "/_daemon/events"
const openPath = (project: string) => `/api/events?${new URLSearchParams({ project, lease: "open" })}`
const watchedPath = (project: string) => `/api/events?${new URLSearchParams({ project, lease: "watched" })}`

test("the control stream opens at once without browser headers and dispatches valid events", async () => {
  const s = setup()
  await flush()
  const control = s.latest(CONTROL)
  assert.equal(control.request.method, "GET")
  assert.deepEqual(control.request.target, { socket: "/run/servef.sock" })
  assert.equal(control.request.stream, true)
  assert.equal(control.request.body, null)
  assert.deepEqual(control.request.headers, { accept: "text/event-stream", host: "localhost" })
  control.send("status", { protocol: 1, version: "1.0.0" })
  control.write(": heartbeat\n\n")
  control.send("walkthrough", { project: "/a", id: "w", title: "Tour", created: true })
  control.send("walkthrough", { project: "/a" })
  control.send("walkthrough", "not json")
  control.send("other", { protocol: 1, version: "x" })
  await flush()
  assert.deepEqual(s.events, [
    { kind: "control", status: { protocol: 1, version: "1.0.0" } },
    { kind: "walkthrough", announcement: { project: "/a", id: "w", title: "Tour", created: true } },
  ])
  s.subscriptions.close()
})

test("shown projects hold open streams that dispatch status, head, and change", async () => {
  const s = setup()
  s.subscriptions.shown(["/src/a b"])
  await flush()
  const stream = s.latest(openPath("/src/a b"))
  assert.equal(stream.request.path, "/api/events?project=%2Fsrc%2Fa+b&lease=open")
  assert.deepEqual(stream.request.headers, { accept: "text/event-stream", host: "localhost" })
  const head = { branch: "main", commit: "c", subject: "s", previous: null }
  const change = { paths: ["x"], tree: false, git: false, overflow: false }
  stream.send("status", { mode: "watching", reason: "" })
  stream.send("head", head)
  stream.send("change", change)
  stream.send("change", { paths: [1] })
  stream.send("status", { mode: "nope", reason: "" })
  await flush()
  assert.deepEqual(s.events, [
    { kind: "status", project: "/src/a b", lease: "open", status: { mode: "watching", reason: "" } },
    { kind: "head", project: "/src/a b", head },
    { kind: "change", project: "/src/a b", lease: "open", change },
  ])
  s.subscriptions.close()
})

test("a project leaving every window switches to watched and back to open", async () => {
  const s = setup()
  s.subscriptions.shown(["/a", "/b"])
  await flush()
  assert.deepEqual(s.live().sort(), [CONTROL, openPath("/a"), openPath("/b")].sort())
  s.subscriptions.shown(["/b"])
  await flush()
  assert.deepEqual(s.live().sort(), [CONTROL, openPath("/b"), watchedPath("/a")].sort())
  const watched = s.latest(watchedPath("/a"))
  watched.send("change", { paths: [], tree: false, git: true, overflow: false })
  await flush()
  assert.deepEqual(s.events.at(-1), { kind: "change", project: "/a", lease: "watched", change: { paths: [], tree: false, git: true, overflow: false } })
  s.subscriptions.shown(["/a", "/b"])
  await flush()
  assert.equal(watched.cancelled, true)
  assert.deepEqual(s.live().sort(), [CONTROL, openPath("/a"), openPath("/b")].sort())
  // An unchanged set keeps its streams.
  const before = s.streams.length
  s.subscriptions.shown(["/b", "/a"])
  await flush()
  assert.equal(s.streams.length, before)
  s.subscriptions.close()
})

test("watched leases drop the least recently shown project past MAX_WATCHED", async () => {
  const s = setup()
  const projects = Array.from({ length: MAX_WATCHED + 2 }, (_, index) => `/p${index}`)
  for (const project of projects) s.subscriptions.shown([project])
  s.subscriptions.shown([])
  await flush()
  const watched = s.live().filter((path) => path.endsWith("lease=watched"))
  assert.equal(watched.length, MAX_WATCHED)
  assert.ok(!watched.includes(watchedPath("/p0")))
  assert.ok(!watched.includes(watchedPath("/p1")))
  assert.ok(watched.includes(watchedPath(`/p${MAX_WATCHED + 1}`)))
  // Re-showing a watched project refreshes its recency.
  s.subscriptions.shown(["/p2"])
  s.subscriptions.shown(["/new"])
  s.subscriptions.shown([])
  await flush()
  const after = s.live().filter((path) => path.endsWith("lease=watched"))
  assert.equal(after.length, MAX_WATCHED)
  assert.ok(after.includes(watchedPath("/p2")))
  assert.ok(after.includes(watchedPath("/new")))
  assert.ok(!after.includes(watchedPath("/p3")))
  s.subscriptions.close()
})

test("ended streams reconnect with doubling backoff reset by a delivered event", async () => {
  const s = setup()
  await flush()
  const expected: number[] = []
  let delay = BACKOFF_MIN_MS
  for (let round = 0; round < 7; round++) {
    s.latest(CONTROL).end()
    await flush()
    expected.push(delay)
    assert.deepEqual(s.clock.delays, expected)
    s.clock.advance(delay)
    await flush()
    delay = Math.min(delay * 2, BACKOFF_MAX_MS)
  }
  assert.equal(expected.at(-1), BACKOFF_MAX_MS)
  s.latest(CONTROL).send("status", { protocol: 1, version: "1" })
  s.latest(CONTROL).end()
  await flush()
  assert.equal(s.clock.delays.at(-1), BACKOFF_MIN_MS)
  assert.equal(s.reensures(), 0, "a clean end does not re-ensure")
  s.subscriptions.close()
})

test("a gone daemon is re-ensured before the retry", async () => {
  const s = setup()
  s.failures.push(Object.assign(new Error("connect ECONNREFUSED"), { code: "ECONNREFUSED" }))
  await flush()
  assert.equal(s.reensures(), 1)
  assert.deepEqual(s.clock.delays, [BACKOFF_MIN_MS])
  assert.equal(s.streams.length, 0)
  s.clock.advance(BACKOFF_MIN_MS)
  await flush()
  assert.deepEqual(s.live(), [CONTROL])
  s.subscriptions.close()
})

test("no connection re-ensures and retries", async () => {
  let available = false
  const s = setup({ connection: () => available ? connection : null })
  await flush()
  assert.equal(s.reensures(), 1)
  assert.equal(s.streams.length, 0)
  available = true
  s.clock.advance(BACKOFF_MIN_MS)
  await flush()
  assert.deepEqual(s.live(), [CONTROL])
  s.subscriptions.close()
})

test("close cancels every body and pending retry", async () => {
  const s = setup()
  s.subscriptions.shown(["/a"])
  s.subscriptions.shown(["/b"])
  await flush()
  s.latest(openPath("/b")).end()
  await flush()
  assert.equal(s.clock.pending(), 1)
  s.subscriptions.close()
  await flush()
  assert.deepEqual(s.live(), [])
  assert.equal(s.clock.pending(), 0)
  s.subscriptions.shown(["/c"])
  await flush()
  assert.deepEqual(s.live(), [])
})
