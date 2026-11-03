import assert from "node:assert/strict"
import { mkdtempSync, rmSync } from "node:fs"
import http from "node:http"
import os from "node:os"
import path from "node:path"
import { after, before, test } from "node:test"
import { createForwarder, isDaemonGone } from "../src/main/forward.ts"
import type { ForwardRequest } from "../src/main/forward.ts"

const dir = mkdtempSync(path.join(os.tmpdir(), "servef-fwd-"))
const socket = path.join(dir, "daemon.sock")
const forwarder = createForwarder()

// Per-test hooks the server consults.
let sseRelease: (() => void) | null = null
let sseClosed: Promise<void> = Promise.resolve()
let markSSEClosed = () => {}
let seen: { method: string; url: string; headers: http.IncomingHttpHeaders; body: string } | null = null

const server = http.createServer((req, res) => {
  const chunks: Buffer[] = []
  req.on("data", (chunk: Buffer) => chunks.push(chunk))
  req.on("end", () => {
    seen = { method: req.method ?? "", url: req.url ?? "", headers: req.headers, body: Buffer.concat(chunks).toString() }
    switch (new URL(req.url ?? "/", "http://localhost").pathname) {
      case "/plain":
        res.setHeader("set-cookie", ["a=1", "b=2"])
        res.setHeader("x-multi", ["one", "two"])
        res.setHeader("keep-alive", "timeout=5")
        res.setHeader("content-type", "text/plain")
        res.end(req.method === "HEAD" ? undefined : "hello")
        return
      case "/echo":
        res.end(`${req.method} ${Buffer.concat(chunks).toString()}`)
        return
      case "/empty":
        res.writeHead(204).end()
        return
      case "/cached":
        res.writeHead(304, { etag: "\"1\"" }).end()
        return
      case "/sse":
        res.writeHead(200, { "content-type": "text/event-stream" })
        res.write("event: status\ndata: 1\n\n")
        res.on("close", () => markSSEClosed())
        sseRelease = () => res.write("event: change\ndata: 2\n\n")
        return
      case "/hangup":
        req.socket.destroy()
        return
      default:
        res.writeHead(404).end()
    }
  })
})

before(() => new Promise<void>((resolve) => server.listen(socket, resolve)))
after(() => {
  forwarder.close()
  server.closeAllConnections()
  server.close()
  rmSync(dir, { recursive: true, force: true })
})

function request(overrides: Partial<ForwardRequest>): ForwardRequest {
  return { target: { socket }, method: "GET", path: "/plain", headers: { host: "localhost" }, body: null, stream: false, ...overrides }
}

function armSSEClosed() {
  sseClosed = new Promise((resolve) => { markSSEClosed = resolve })
}

test("forwards method, path, and headers and streams the response", async () => {
  const response = await forwarder.forward(request({ path: "/plain?q=%20x", headers: { host: "localhost", accept: "text/plain", "sec-fetch-site": "same-origin" } }))
  assert.equal(response.status, 200)
  assert.equal(await response.text(), "hello")
  assert.equal(seen?.url, "/plain?q=%20x")
  assert.equal(seen?.headers.host, "localhost")
  assert.equal(seen?.headers["sec-fetch-site"], "same-origin")
  assert.equal(response.headers.get("content-type"), "text/plain")
  assert.deepEqual(response.headers.getSetCookie(), ["a=1", "b=2"])
  assert.equal(response.headers.get("x-multi"), "one, two")
  for (const name of ["connection", "keep-alive", "transfer-encoding"]) assert.equal(response.headers.has(name), false, name)
})

test("HEAD, 204, and 304 responses carry no body", async () => {
  const head = await forwarder.forward(request({ method: "HEAD" }))
  assert.equal(head.status, 200)
  assert.equal(head.body, null)
  const empty = await forwarder.forward(request({ path: "/empty" }))
  assert.equal(empty.status, 204)
  assert.equal(empty.body, null)
  const cached = await forwarder.forward(request({ path: "/cached" }))
  assert.equal(cached.status, 304)
  assert.equal(cached.body, null)
  assert.equal(cached.headers.get("etag"), "\"1\"")
})

test("streams request bodies", async () => {
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new TextEncoder().encode("{\"path\":"))
      controller.enqueue(new TextEncoder().encode("\"/a\"}"))
      controller.close()
    },
  })
  const response = await forwarder.forward(request({ method: "POST", path: "/echo", body, headers: { host: "localhost", "content-type": "application/json" } }))
  assert.equal(await response.text(), "POST {\"path\":\"/a\"}")
})

test("event streams arrive incrementally and cancel closes the daemon request", async () => {
  armSSEClosed()
  const response = await forwarder.forward(request({ path: "/sse", stream: true, headers: { host: "localhost", accept: "text/event-stream" } }))
  assert.ok(response.body !== null)
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  const first = await reader.read()
  assert.equal(decoder.decode(first.value), "event: status\ndata: 1\n\n")
  assert.ok(sseRelease !== null)
  sseRelease()
  const second = await reader.read()
  assert.equal(decoder.decode(second.value), "event: change\ndata: 2\n\n")
  await reader.cancel()
  await sseClosed
})

test("aborting the signal after the head closes the daemon request", async () => {
  armSSEClosed()
  const controller = new AbortController()
  const response = await forwarder.forward(request({ path: "/sse", stream: true, signal: controller.signal }))
  assert.equal(response.status, 200)
  controller.abort()
  await sseClosed
})

test("an aborted signal rejects before sending", async () => {
  const controller = new AbortController()
  controller.abort()
  await assert.rejects(forwarder.forward(request({ signal: controller.signal })), { name: "AbortError" })
})

test("a missing socket rejects with a daemon-gone error", async () => {
  await assert.rejects(forwarder.forward(request({ target: { socket: path.join(dir, "missing.sock") } })), (error: unknown) => isDaemonGone(error))
})

test("a connection closed before the response head is a daemon-gone error", async () => {
  await assert.rejects(forwarder.forward(request({ path: "/hangup" })), (error: unknown) => isDaemonGone(error))
})

test("keep-alive requests do not leak socket listeners", async () => {
  const warnings: string[] = []
  const onWarning = (warning: Error) => warnings.push(warning.name)
  process.on("warning", onWarning)
  try {
    for (let round = 0; round < 40; round++) {
      const response = await forwarder.forward(request({}))
      await response.text()
    }
    await new Promise((resolve) => setImmediate(resolve))
  } finally {
    process.off("warning", onWarning)
  }
  assert.deepEqual(warnings, [])
})

test("isDaemonGone recognizes connect failures, resets, and broken pipes only", () => {
  assert.equal(isDaemonGone(Object.assign(new Error("x"), { code: "ECONNREFUSED" })), true)
  assert.equal(isDaemonGone(Object.assign(new Error("x"), { code: "ENOENT" })), true)
  assert.equal(isDaemonGone(Object.assign(new Error("x"), { code: "ECONNRESET" })), true)
  assert.equal(isDaemonGone(Object.assign(new Error("x"), { code: "EPIPE" })), true)
  assert.equal(isDaemonGone(Object.assign(new Error("x"), { code: "ETIMEDOUT" })), false)
  assert.equal(isDaemonGone(new Error("x")), false)
  assert.equal(isDaemonGone(null), false)
})

// TCP targets (CLI servers, decision 0071).

test("TCP targets forward to a loopback port with the caller's Host", async () => {
  const tcp = http.createServer((req, res) => res.end(`${req.method} ${req.url} ${req.headers.host ?? ""}`))
  await new Promise<void>((resolve) => tcp.listen(0, "127.0.0.1", resolve))
  const address = tcp.address()
  assert.ok(address !== null && typeof address === "object")
  try {
    for (const host of ["127.0.0.1", "localhost"]) {
      const response = await forwarder.forward(request({ target: { host, port: address.port }, path: "/api/projects?x=1", headers: { host: `localhost:${address.port}` } }))
      assert.equal(await response.text(), `GET /api/projects?x=1 localhost:${address.port}`, host)
    }
    // The Unix socket target still answers alongside the TCP one.
    assert.equal(await (await forwarder.forward(request({}))).text(), "hello")
  } finally {
    tcp.closeAllConnections()
    await new Promise<void>((resolve) => tcp.close(() => resolve()))
  }
  await assert.rejects(forwarder.forward(request({ target: { host: "127.0.0.1", port: address.port } })), (error: unknown) => isDaemonGone(error))
})
