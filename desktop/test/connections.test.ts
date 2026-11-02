import assert from "node:assert/strict"
import { test } from "node:test"
import { createConnections, LOCAL_CONNECTION } from "../src/main/connections.ts"
import type { ConnectionLookup } from "../src/main/connections.ts"
import type { EnsureResult } from "../src/main/daemon.ts"

const status = (pid: number) => ({ protocol: 1, version: "1", pid, socket: "/run/servef.sock", roots: [] })

function controlledEnsure() {
  const pending: ((result: EnsureResult) => void)[] = []
  let calls = 0
  const ensure = () => {
    calls += 1
    return new Promise<EnsureResult>((resolve) => pending.push(resolve))
  }
  return { ensure, pending, calls: () => calls }
}

test("first lookup starts ensure and concurrent callers share it", async () => {
  const control = controlledEnsure()
  const connections = createConnections(control.ensure)
  const first = connections.lookup(LOCAL_CONNECTION)
  const second = connections.lookup(LOCAL_CONNECTION)
  await Promise.resolve()
  assert.equal(control.calls(), 1)
  control.pending[0]?.({ ok: true, status: status(1) })
  const [a, b] = await Promise.all([first, second])
  assert.deepEqual(a, b)
  assert.equal(a.kind, "ready")
  assert.ok(a.kind === "ready")
  assert.deepEqual(a.connection, { id: "local", kind: "daemon", origin: "servef-app://local", socket: "/run/servef.sock", status: status(1) })
  // Later lookups reuse the result.
  assert.equal((await connections.lookup(LOCAL_CONNECTION)).kind, "ready")
  assert.equal(control.calls(), 1)
})

test("other connection ids are unknown and never ensure", async () => {
  const control = controlledEnsure()
  const connections = createConnections(control.ensure)
  assert.deepEqual(await connections.lookup("ssh-box"), { kind: "unknown" })
  assert.equal(await connections.reensure("ssh-box"), false)
  assert.equal(control.calls(), 0)
})

test("reensure is single-flight and lookups wait for it", async () => {
  let calls = 0
  let next: EnsureResult = { ok: false, kind: "failed", detail: "down" }
  const connections = createConnections(async () => {
    calls += 1
    await new Promise((resolve) => setImmediate(resolve))
    return next
  })
  assert.equal((await connections.lookup(LOCAL_CONNECTION)).kind, "failed")
  next = { ok: true, status: status(2) }
  const results = await Promise.all([connections.reensure(LOCAL_CONNECTION), connections.reensure(LOCAL_CONNECTION), connections.lookup(LOCAL_CONNECTION)])
  assert.equal(calls, 2)
  assert.equal(results[0], true)
  assert.equal(results[1], true)
  assert.equal(results[2].kind, "ready")
})

test("a rejected ensure is a failed lookup", async () => {
  const connections = createConnections(async () => { throw new Error("spawn failed") })
  const lookup = await connections.lookup(LOCAL_CONNECTION)
  assert.ok(lookup.kind === "failed")
  assert.equal(lookup.failure.detail, "spawn failed")
})

test("listeners hear state changes only", async () => {
  let next: EnsureResult = { ok: true, status: status(1) }
  const connections = createConnections(async () => next)
  const heard: [string, ConnectionLookup["kind"]][] = []
  const unsubscribe = connections.subscribe((id, lookup) => heard.push([id, lookup.kind]))
  await connections.lookup(LOCAL_CONNECTION)
  await connections.reensure(LOCAL_CONNECTION)
  next = { ok: true, status: status(9) }
  await connections.reensure(LOCAL_CONNECTION)
  next = { ok: false, kind: "mismatch", detail: "protocol 2" }
  await connections.reensure(LOCAL_CONNECTION)
  unsubscribe()
  next = { ok: true, status: status(1) }
  await connections.reensure(LOCAL_CONNECTION)
  assert.deepEqual(heard, [["local", "ready"], ["local", "ready"], ["local", "failed"]])
})
