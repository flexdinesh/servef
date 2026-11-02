import assert from "node:assert/strict"
import { test } from "node:test"
import type { ConnectionStore, KnownConnection } from "../src/main/connection-store.ts"
import { assignId } from "../src/main/connection-store.ts"
import { APP_HOST, createConnections, LOCAL_CONNECTION, RELEASE_GRACE_MS } from "../src/main/connections.ts"
import type { ConnectionLookup, Connections, ConnectionsDeps } from "../src/main/connections.ts"
import type { EnsureResult } from "../src/main/daemon.ts"
import type { SshReady, SshSession, SshState, SshTarget } from "../src/main/ssh-connection.ts"
import type { SshFailure } from "../src/main/ssh-errors.ts"
import { createFakeTimers, flush } from "./fake-timers.ts"

const status = (pid: number, socket = "/run/servef.sock") => ({ protocol: 1, version: "1", pid, socket, roots: [] })

const devbox: KnownConnection = { id: "ssh-devbox", destination: "devbox", port: null, hints: [{ hostname: "devbox", user: "dee" }], addedAt: "2026-11-01T00:00:00.000Z" }
const work: KnownConnection = { id: "ssh-work", destination: "dee@work", port: 2222, hints: [], addedAt: "2026-11-01T00:00:00.000Z" }

const unreachable: SshFailure = { kind: "unreachable", destination: "devbox", detail: "Connection timed out" }
const hostKey: SshFailure = { kind: "hostKey", destination: "devbox", detail: "Host key verification failed." }

// FakeSession is an SshSession whose state the test drives with set().
interface FakeSession extends SshSession {
  target: SshTarget
  set(state: SshState): void
  calls: string[]
  reensureResult: boolean
  // closeGate, when set, holds close() until resolved.
  closeGate: Promise<void> | null
  closed: boolean
}

function fakeSession(target: SshTarget): FakeSession {
  let state: SshState = { kind: "connecting" }
  const listeners = new Set<(state: SshState) => void>()
  let waiters: ((result: SshReady) => void)[] = []
  const settle = (current: SshState): SshReady | null => {
    if (current.kind === "ready") return { ok: true, socket: current.socket, status: current.status }
    if (current.kind === "connecting") return null
    return { ok: false, failure: current.failure }
  }
  const session: FakeSession = {
    target,
    calls: [],
    reensureResult: true,
    closeGate: null,
    closed: false,
    state: () => state,
    ready() {
      const result = settle(state)
      if (result !== null) return Promise.resolve(result)
      return new Promise((resolve) => waiters.push(resolve))
    },
    async reensure() {
      session.calls.push("reensure")
      return session.reensureResult
    },
    async stopRemoteDaemon() {
      session.calls.push("stop")
      return true
    },
    onState(listener) {
      listeners.add(listener)
      return () => { listeners.delete(listener) }
    },
    async close() {
      session.calls.push("close")
      if (session.closeGate !== null) await session.closeGate
      session.closed = true
    },
    closeSync() {
      session.calls.push("closeSync")
      session.closed = true
    },
    set(next) {
      state = next
      for (const listener of listeners) listener(next)
      const result = settle(next)
      if (result === null) return
      const pending = waiters
      waiters = []
      for (const resolve of pending) resolve(result)
    },
  }
  return session
}

interface Harness {
  connections: Connections
  sessions: FakeSession[]
  saves: KnownConnection[][]
  stops: string[]
  timers: ReturnType<typeof createFakeTimers>
  heard: [string, string][]
}

function harness(options: { known?: KnownConnection[]; ensure?: () => Promise<EnsureResult> } = {}): Harness {
  const sessions: FakeSession[] = []
  const saves: KnownConnection[][] = []
  const stops: string[] = []
  const timers = createFakeTimers()
  const store: ConnectionStore = {
    load: () => (options.known ?? [devbox, work]).map((entry) => ({ ...entry, hints: [...entry.hints] })),
    save: (known) => { saves.push(known.map((entry) => ({ ...entry, hints: [...entry.hints] }))) },
  }
  const deps: ConnectionsDeps = {
    ensureLocal: options.ensure ?? (async () => ({ ok: true, status: status(1) })),
    stopLocal: async (socket) => {
      stops.push(socket)
      return true
    },
    ssh: {
      connect(target) {
        const session = fakeSession(target)
        sessions.push(session)
        return session
      },
    },
    store,
    timers: timers.timers,
  }
  const connections = createConnections(deps)
  const heard: [string, string][] = []
  connections.subscribe((id, lookup) => heard.push([id, lookup === null ? "null" : lookup.kind]))
  return { connections, sessions, saves, stops, timers, heard }
}

function only(h: Harness, index = 0): FakeSession {
  const session = h.sessions[index]
  assert.ok(session !== undefined, `session ${index} exists`)
  return session
}

function controlledEnsure() {
  const pending: ((result: EnsureResult) => void)[] = []
  let calls = 0
  const ensure = () => {
    calls += 1
    return new Promise<EnsureResult>((resolve) => pending.push(resolve))
  }
  return { ensure, pending, calls: () => calls }
}

// Local daemon.

test("first local lookup starts ensure and concurrent callers share it", async () => {
  const control = controlledEnsure()
  const { connections } = harness({ ensure: control.ensure })
  const first = connections.lookup(LOCAL_CONNECTION)
  const second = connections.lookup(LOCAL_CONNECTION)
  await Promise.resolve()
  assert.equal(control.calls(), 1)
  control.pending[0]?.({ ok: true, status: status(1) })
  const [a, b] = await Promise.all([first, second])
  assert.deepEqual(a, b)
  assert.ok(a.kind === "ready")
  assert.deepEqual(a.connection, { id: "local", kind: "daemon", origin: "servef-app://local", socket: "/run/servef.sock", status: status(1), label: "local" })
  assert.equal((await connections.lookup(LOCAL_CONNECTION)).kind, "ready")
  assert.equal(control.calls(), 1)
})

test("unknown ids and the app host are unknown and never connect", async () => {
  const control = controlledEnsure()
  const h = harness({ ensure: control.ensure })
  for (const id of ["ssh-other", APP_HOST, "cli-1", ""]) {
    assert.deepEqual(await h.connections.lookup(id), { kind: "unknown" }, id)
    assert.deepEqual(h.connections.peek(id), { kind: "unknown" }, id)
    assert.equal(await h.connections.reensure(id), false, id)
    assert.equal(await h.connections.stopAndRetry(id), false, id)
    assert.equal(h.connections.describe(id), null, id)
  }
  assert.equal(control.calls(), 0)
  assert.equal(h.sessions.length, 0)
})

test("local reensure is single-flight and lookups wait for it", async () => {
  let calls = 0
  let next: EnsureResult = { ok: false, kind: "failed", detail: "down" }
  const { connections } = harness({
    ensure: async () => {
      calls += 1
      await new Promise((resolve) => setImmediate(resolve))
      return next
    },
  })
  const failed = await connections.lookup(LOCAL_CONNECTION)
  assert.deepEqual(failed, { kind: "failed", connection: "daemon", failure: { ok: false, kind: "failed", detail: "down" }, label: "local" })
  next = { ok: true, status: status(2) }
  const results = await Promise.all([connections.reensure(LOCAL_CONNECTION), connections.reensure(LOCAL_CONNECTION), connections.lookup(LOCAL_CONNECTION)])
  assert.equal(calls, 2)
  assert.equal(results[0], true)
  assert.equal(results[1], true)
  assert.equal(results[2].kind, "ready")
})

test("a rejected local ensure is a failed lookup", async () => {
  const { connections } = harness({ ensure: async () => { throw new Error("spawn failed") } })
  const lookup = await connections.lookup(LOCAL_CONNECTION)
  assert.ok(lookup.kind === "failed")
  assert.equal(lookup.failure.detail, "spawn failed")
})

test("local listeners hear changed outcomes only", async () => {
  let next: EnsureResult = { ok: true, status: status(1) }
  const h = harness({ ensure: async () => next })
  await h.connections.lookup(LOCAL_CONNECTION)
  await h.connections.reensure(LOCAL_CONNECTION)
  next = { ok: true, status: status(9) }
  await h.connections.reensure(LOCAL_CONNECTION)
  next = { ok: false, kind: "mismatch", detail: "protocol 2" }
  await h.connections.reensure(LOCAL_CONNECTION)
  assert.deepEqual(h.heard, [["local", "ready"], ["local", "ready"], ["local", "failed"]])
})

test("local stopAndRetry stops the failed daemon's socket, then ensures again", async () => {
  let next: EnsureResult = { ok: false, kind: "mismatch", detail: "protocol 2", status: { ...status(1, "/run/old.sock"), protocol: 2 } }
  let calls = 0
  const h = harness({ ensure: async () => { calls += 1; return next } })
  await h.connections.lookup(LOCAL_CONNECTION)
  next = { ok: true, status: status(2) }
  assert.equal(await h.connections.stopAndRetry(LOCAL_CONNECTION), true)
  assert.deepEqual(h.stops, ["/run/old.sock"])
  assert.equal(calls, 2)
  // Ready (or failed without a status): nothing to stop.
  assert.equal(await h.connections.stopAndRetry(LOCAL_CONNECTION), false)
  assert.deepEqual(h.stops, ["/run/old.sock"])
})

// ssh connections.

test("a known ssh id connects once and concurrent lookups share the attempt", async () => {
  const h = harness()
  const first = h.connections.lookup("ssh-devbox")
  const second = h.connections.lookup("ssh-devbox")
  await flush()
  assert.equal(h.sessions.length, 1)
  assert.deepEqual(only(h).target, { id: "ssh-devbox", destination: "devbox", port: null })
  only(h).set({ kind: "ready", socket: "/run/f-1.sock", status: status(7, "/remote/servef.sock") })
  const [a, b] = await Promise.all([first, second])
  assert.deepEqual(a, b)
  assert.deepEqual(a, {
    kind: "ready",
    connection: { id: "ssh-devbox", kind: "ssh", origin: "servef-app://ssh-devbox", socket: "/run/f-1.sock", status: status(7, "/remote/servef.sock"), label: "devbox" },
  })
  assert.equal((await h.connections.lookup("ssh-devbox")).kind, "ready")
  void h.connections.lookup("ssh-work")
  await flush()
  assert.equal(h.sessions.length, 2, "each known id has its own session")
  assert.deepEqual(only(h, 1).target, { id: "ssh-work", destination: "dee@work", port: 2222 })
})

test("ssh lookups map reconnecting to connecting and failed to failed without waiting", async () => {
  const h = harness()
  const started = h.connections.lookup("ssh-devbox")
  await flush()
  const session = only(h)
  session.set({ kind: "reconnecting", failure: unreachable, delayMs: 1000 })
  assert.deepEqual(await started, { kind: "connecting", label: "devbox", failure: unreachable })
  assert.deepEqual(await h.connections.lookup("ssh-devbox"), { kind: "connecting", label: "devbox", failure: unreachable })
  session.set({ kind: "failed", failure: hostKey })
  assert.deepEqual(await h.connections.lookup("ssh-devbox"), { kind: "failed", connection: "ssh", failure: hostKey, label: "devbox" })
  assert.equal(h.sessions.length, 1)
})

test("an ssh lookup during a retry waits for that attempt", async () => {
  const h = harness()
  void h.connections.lookup("ssh-devbox")
  await flush()
  const session = only(h)
  session.set({ kind: "failed", failure: hostKey })
  session.set({ kind: "connecting" })
  const box: { lookup: ConnectionLookup | null } = { lookup: null }
  void h.connections.lookup("ssh-devbox").then((lookup) => { box.lookup = lookup })
  const current = (): ConnectionLookup | null => box.lookup
  await flush()
  assert.equal(current(), null)
  assert.deepEqual(h.connections.peek("ssh-devbox"), { kind: "connecting", label: "devbox", failure: hostKey })
  session.set({ kind: "ready", socket: "/run/f.sock", status: status(1) })
  await flush()
  assert.equal(current()?.kind, "ready")
})

test("a failed attempt resolves the lookup with the session's next state", async () => {
  const h = harness()
  const started = h.connections.lookup("ssh-devbox")
  await flush()
  only(h).set({ kind: "failed", failure: hostKey })
  assert.deepEqual(await started, { kind: "failed", connection: "ssh", failure: hostKey, label: "devbox" })
})

test("peek never starts a session", async () => {
  const h = harness()
  assert.equal(h.connections.peek("ssh-devbox"), null)
  assert.equal(h.connections.peek(LOCAL_CONNECTION), null)
  assert.equal(h.sessions.length, 0)
  void h.connections.lookup("ssh-devbox")
  await flush()
  assert.equal(h.connections.peek("ssh-devbox"), null, "a first connect has no lookup yet")
  only(h).set({ kind: "ready", socket: "/run/f.sock", status: status(1) })
  assert.equal(h.connections.peek("ssh-devbox")?.kind, "ready")
})

test("ssh listeners hear each mapped state change and null on close", async () => {
  const h = harness()
  void h.connections.lookup("ssh-devbox")
  await flush()
  const session = only(h)
  session.set({ kind: "ready", socket: "/run/f.sock", status: status(1) })
  session.set({ kind: "reconnecting", failure: unreachable, delayMs: 1000 })
  session.set({ kind: "connecting" })
  session.set({ kind: "failed", failure: hostKey })
  h.timers.advance(RELEASE_GRACE_MS)
  await flush()
  assert.deepEqual(h.heard, [["ssh-devbox", "ready"], ["ssh-devbox", "connecting"], ["ssh-devbox", "connecting"], ["ssh-devbox", "failed"], ["ssh-devbox", "null"]])
  session.set({ kind: "ready", socket: "/run/f.sock", status: status(1) })
  assert.equal(h.heard.length, 5, "a closed session is no longer heard")
})

test("a never-held session closes after the grace period and a later lookup reconnects", async () => {
  const h = harness()
  void h.connections.lookup("ssh-devbox")
  await flush()
  only(h).set({ kind: "ready", socket: "/run/f.sock", status: status(1) })
  h.timers.advance(RELEASE_GRACE_MS - 1)
  await flush()
  assert.equal(only(h).closed, false)
  h.timers.advance(1)
  await flush()
  assert.equal(only(h).closed, true)
  assert.equal(h.connections.peek("ssh-devbox"), null)
  assert.equal(h.connections.describe("ssh-devbox")?.state, "idle")
  void h.connections.lookup("ssh-devbox")
  await flush()
  assert.equal(h.sessions.length, 2)
})

test("held sessions stay; the last release closes after 30 s; a hold within the grace cancels", async () => {
  const h = harness()
  h.connections.hold("ssh-devbox")
  h.connections.hold("ssh-devbox")
  assert.equal(h.sessions.length, 0, "holding does not connect")
  void h.connections.lookup("ssh-devbox")
  await flush()
  const session = only(h)
  h.timers.advance(5 * RELEASE_GRACE_MS)
  h.connections.release("ssh-devbox")
  h.timers.advance(5 * RELEASE_GRACE_MS)
  await flush()
  assert.equal(session.closed, false, "one window still holds it")

  // A reload: release then hold within the grace does not churn.
  h.connections.release("ssh-devbox")
  h.timers.advance(29_000)
  h.connections.hold("ssh-devbox")
  h.timers.advance(5 * RELEASE_GRACE_MS)
  await flush()
  assert.equal(session.closed, false)
  assert.equal(h.sessions.length, 1)

  h.connections.release("ssh-devbox")
  h.timers.advance(29_000)
  await flush()
  assert.equal(session.closed, false)
  h.timers.advance(1_000)
  await flush()
  assert.equal(session.closed, true)
  assert.deepEqual(h.heard.at(-1), ["ssh-devbox", "null"])
})

test("a new session waits for the previous one to finish closing", async () => {
  const h = harness()
  void h.connections.lookup("ssh-devbox")
  await flush()
  let open = () => {}
  only(h).closeGate = new Promise((resolve) => { open = resolve })
  h.timers.advance(RELEASE_GRACE_MS)
  void h.connections.lookup("ssh-devbox")
  await flush()
  assert.equal(h.sessions.length, 1, "no second master while the first closes")
  open()
  await flush()
  assert.equal(h.sessions.length, 2)
})

test("ssh reensure and stopAndRetry use the session; false without one", async () => {
  const h = harness()
  assert.equal(await h.connections.reensure("ssh-devbox"), false)
  assert.equal(await h.connections.stopAndRetry("ssh-devbox"), false)
  assert.equal(h.sessions.length, 0)
  void h.connections.lookup("ssh-devbox")
  await flush()
  const session = only(h)
  session.reensureResult = false
  assert.equal(await h.connections.reensure("ssh-devbox"), false)
  session.reensureResult = true
  assert.equal(await h.connections.stopAndRetry("ssh-devbox"), true)
  assert.deepEqual(session.calls, ["reensure", "stop", "reensure"])
  assert.deepEqual(h.stops, [], "ssh never stops the local daemon")
})

test("summaries list local first, then known hosts with their states and details", async () => {
  const control = controlledEnsure()
  const h = harness({ ensure: control.ensure })
  assert.deepEqual(h.connections.summaries(), [
    { id: "local", kind: "daemon", label: "local", state: "idle", detail: "" },
    { id: "ssh-devbox", kind: "ssh", label: "devbox", state: "idle", detail: "" },
    { id: "ssh-work", kind: "ssh", label: "dee@work", state: "idle", detail: "" },
  ])
  const local = h.connections.lookup(LOCAL_CONNECTION)
  assert.equal(h.connections.describe(LOCAL_CONNECTION)?.state, "connecting")
  control.pending[0]?.({ ok: false, kind: "failed", detail: "boom" })
  await local
  assert.deepEqual(h.connections.describe(LOCAL_CONNECTION), { id: "local", kind: "daemon", label: "local", state: "failed", detail: "boom" })
  const again = h.connections.reensure(LOCAL_CONNECTION)
  control.pending[1]?.({ ok: true, status: status(1) })
  await again
  assert.equal(h.connections.describe(LOCAL_CONNECTION)?.state, "ready")

  void h.connections.lookup("ssh-devbox")
  await flush()
  const session = only(h)
  const devboxState = () => h.connections.describe("ssh-devbox")
  assert.deepEqual(devboxState(), { id: "ssh-devbox", kind: "ssh", label: "devbox", state: "connecting", detail: "" })
  session.set({ kind: "ready", socket: "/run/f.sock", status: status(1) })
  assert.equal(devboxState()?.state, "ready")
  session.set({ kind: "reconnecting", failure: unreachable, delayMs: 2000 })
  assert.deepEqual(devboxState(), { id: "ssh-devbox", kind: "ssh", label: "devbox", state: "reconnecting", detail: "Connection timed out" })
  session.set({ kind: "failed", failure: hostKey })
  assert.deepEqual(devboxState(), { id: "ssh-devbox", kind: "ssh", label: "devbox", state: "failed", detail: "Host key verification failed." })
  assert.deepEqual(h.connections.summaries().map((summary) => summary.state), ["ready", "failed", "idle"])
})

test("remember merges hints into an existing destination and port", () => {
  const h = harness()
  const same = h.connections.remember({ destination: "devbox", port: null }, { user: "dee", hostname: "devbox" })
  assert.equal(same.id, "ssh-devbox")
  assert.deepEqual(same.hints, [{ hostname: "devbox", user: "dee" }], "equal hints are not duplicated")
  const merged = h.connections.remember({ destination: "devbox", port: null }, { addr: "10.0.0.5", user: "dee" })
  assert.deepEqual(merged.hints, [{ hostname: "devbox", user: "dee" }, { addr: "10.0.0.5", user: "dee" }])
  assert.equal(h.saves.length, 2)
  assert.deepEqual(h.saves.at(-1)?.find((entry) => entry.id === "ssh-devbox")?.hints, merged.hints)
  assert.deepEqual(h.connections.known().map((entry) => entry.id), ["ssh-devbox", "ssh-work"])
  // known() returns copies.
  h.connections.known()[0]?.hints.push({ user: "x" })
  assert.equal(h.connections.known()[0]?.hints.length, 2)
})

test("remember adds new destinations with assigned ids, timestamps, and saves", () => {
  const h = harness({ known: [devbox] })
  h.timers.advance(Date.UTC(2026, 10, 2))
  const added = h.connections.remember({ destination: "dee@box", port: 2222 }, { hostname: "box" })
  assert.deepEqual(added, { id: assignId("dee@box", new Set(["ssh-devbox"])), destination: "dee@box", port: 2222, hints: [{ hostname: "box" }], addedAt: "2026-11-02T00:00:00.000Z" })
  // Same destination on another port is another host; empty hints store none.
  const other = h.connections.remember({ destination: "dee@box", port: null }, {})
  assert.equal(other.id, assignId("dee@box", new Set(["ssh-devbox", added.id])))
  assert.notEqual(other.id, added.id)
  assert.deepEqual(other.hints, [])
  assert.equal(h.saves.length, 2)
  assert.deepEqual(h.saves.at(-1)?.map((entry) => entry.id), ["ssh-devbox", added.id, other.id])
  assert.equal(h.connections.describe(added.id)?.state, "idle")
})

test("forget closes the session now, removes and saves the entry, and notifies once", async () => {
  const h = harness()
  h.connections.hold("ssh-devbox")
  void h.connections.lookup("ssh-devbox")
  await flush()
  await h.connections.forget("ssh-devbox")
  assert.equal(only(h).closed, true)
  assert.deepEqual(h.saves, [[work]])
  assert.deepEqual(h.heard, [["ssh-devbox", "null"]])
  assert.deepEqual(await h.connections.lookup("ssh-devbox"), { kind: "unknown" })
  assert.equal(h.connections.describe("ssh-devbox"), null)
  assert.equal(h.timers.pending(), 0)
  // Without a session it still removes the entry.
  await h.connections.forget("ssh-work")
  assert.deepEqual(h.saves.at(-1), [])
  assert.deepEqual(h.heard.at(-1), ["ssh-work", "null"])
})

test("close closes every session and clears grace timers; closeSync closes synchronously", async () => {
  const h = harness()
  void h.connections.lookup("ssh-devbox")
  void h.connections.lookup("ssh-work")
  await flush()
  assert.equal(h.timers.pending(), 2)
  await h.connections.close()
  assert.deepEqual(h.sessions.map((session) => session.calls), [["close"], ["close"]])
  assert.equal(h.timers.pending(), 0)

  const s = harness()
  void s.connections.lookup("ssh-devbox")
  await flush()
  s.connections.closeSync()
  assert.deepEqual(only(s).calls, ["closeSync"])
  assert.equal(s.timers.pending(), 0)
})
