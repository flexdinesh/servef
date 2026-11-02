import assert from "node:assert/strict"
import { test } from "node:test"
import type { ConnectionLookup } from "../src/main/connections.ts"
import type { ForwardRequest } from "../src/main/forward.ts"
import { STOP_AND_RETRY_PATH } from "../src/main/pages.ts"
import { createProxy } from "../src/main/proxy.ts"

const status = { protocol: 1, version: "1", pid: 1, socket: "/run/servef.sock", roots: [] }
const ready: ConnectionLookup = { kind: "ready", connection: { id: "local", kind: "daemon", origin: "servef-app://local", socket: "/run/servef.sock", status } }
const failed: ConnectionLookup = { kind: "failed", failure: { ok: false, kind: "failed", detail: "daemon did not start" } }
const mismatch: ConnectionLookup = { kind: "failed", failure: { ok: false, kind: "mismatch", detail: "protocol 2", status: { ...status, protocol: 2 } } }

const gone = () => Object.assign(new Error("connect ECONNREFUSED"), { code: "ECONNREFUSED" })

type Step = Response | Error

interface Harness {
  proxy: (request: Request) => Promise<Response>
  forwarded: ForwardRequest[]
  reensures: number
  stops: number
}

// harness answers lookups from lookups (last repeats) and forwards from steps.
function harness(options: { lookups?: ConnectionLookup[]; steps?: Step[]; reensure?: boolean[] } = {}): Harness {
  const lookups = [...(options.lookups ?? [ready])]
  const steps = [...(options.steps ?? [new Response("ok")])]
  const reensure = [...(options.reensure ?? [true])]
  const state: Harness = {
    proxy: () => { throw new Error("unset") },
    forwarded: [],
    reensures: 0,
    stops: 0,
  }
  state.proxy = createProxy({
    packaged: true,
    lookup: async (id) => {
      if (id !== "local") return { kind: "unknown" }
      return (lookups.length > 1 ? lookups.shift() : lookups[0]) ?? ready
    },
    forward: async (request) => {
      state.forwarded.push(request)
      const step = steps.length > 1 ? steps.shift() : steps[0]
      if (step instanceof Error) throw step
      return step ?? new Response("ok")
    },
    reensure: async () => {
      state.reensures += 1
      return (reensure.length > 1 ? reensure.shift() : reensure[0]) ?? false
    },
    stopAndRetry: async () => {
      state.stops += 1
      return true
    },
  })
  return state
}

async function errorCode(response: Response): Promise<unknown> {
  const body: unknown = await response.json()
  return typeof body === "object" && body !== null && "code" in body ? body.code : undefined
}

test("unknown connection ids are 404", async () => {
  const h = harness()
  const response = await h.proxy(new Request("servef-app://ssh-box/api/projects"))
  assert.equal(response.status, 404)
  assert.equal(h.forwarded.length, 0)
})

test("the daemon control API is refused, including encoded paths", async () => {
  const h = harness()
  for (const path of ["/_daemon/status", "/%5Fdaemon/status", "/%5fdaemon/stop", "//_daemon/status", "/_daemon"]) {
    const response = await h.proxy(new Request(`servef-app://local${path}`))
    assert.equal(response.status, 403, path)
  }
  assert.equal(h.forwarded.length, 0)
})

test("malformed percent-encoding is 400", async () => {
  const h = harness()
  assert.equal((await h.proxy(new Request("servef-app://local/%E0%A4%A"))).status, 400)
})

test("a foreign Origin is refused; same or absent Origin passes", async () => {
  const h = harness({ steps: [new Response("a"), new Response("b"), new Response("c")] })
  const foreign = await h.proxy(new Request("servef-app://local/api/projects", { headers: { origin: "servef-app://ssh-box" } }))
  assert.equal(foreign.status, 403)
  assert.equal(h.forwarded.length, 0)
  assert.equal((await h.proxy(new Request("servef-app://local/assets/a.js", { headers: { origin: "servef-app://local" } }))).status, 200)
  assert.equal((await h.proxy(new Request("servef-app://local/api/projects"))).status, 200)
  assert.equal(h.forwarded.length, 2)
})

test("forwards the encoded path and filtered headers with Host and Sec-Fetch-Site", async () => {
  const h = harness()
  const response = await h.proxy(new Request("servef-app://local/api/page?project=%2Fsrc%2Fa&path=a%20b.md", {
    headers: { accept: "application/json", origin: "servef-app://local", cookie: "c=1", referer: "servef-app://local/", "sec-fetch-site": "cross-site", connection: "upgrade" },
  }))
  assert.equal(await response.text(), "ok")
  const [forwarded] = h.forwarded
  assert.ok(forwarded !== undefined)
  assert.equal(forwarded.socket, "/run/servef.sock")
  assert.equal(forwarded.method, "GET")
  assert.equal(forwarded.path, "/api/page?project=%2Fsrc%2Fa&path=a%20b.md")
  assert.deepEqual(forwarded.headers, { accept: "application/json", host: "localhost", "sec-fetch-site": "same-origin" })
  assert.equal(forwarded.body, null)
  assert.equal(forwarded.stream, false)
  assert.ok(forwarded.signal !== undefined)
})

test("event stream requests forward as streams", async () => {
  const h = harness()
  await h.proxy(new Request("servef-app://local/api/events?project=%2Fa", { headers: { accept: "text/event-stream" } }))
  assert.equal(h.forwarded[0]?.stream, true)
})

test("request bodies forward for non-GET methods", async () => {
  const h = harness()
  await h.proxy(new Request("servef-app://local/api/walkthroughs", { method: "POST", body: "{}", headers: { "content-type": "application/json" } }))
  const body = h.forwarded[0]?.body ?? null
  assert.ok(body !== null)
  assert.equal(await new Response(body).text(), "{}")
})

test("the daemon's response passes through unchanged", async () => {
  const upstream = new Response("<!doctype html>", { status: 200, headers: { "content-security-policy": "default-src 'self'", etag: "\"1\"" } })
  const h = harness({ steps: [upstream] })
  const response = await h.proxy(new Request("servef-app://local/"))
  assert.equal(response, upstream)
})

test("a gone daemon is re-ensured and a GET retried once", async () => {
  const h = harness({ steps: [gone(), new Response("again")] })
  const response = await h.proxy(new Request("servef-app://local/api/projects"))
  assert.equal(await response.text(), "again")
  assert.equal(h.reensures, 1)
  assert.equal(h.forwarded.length, 2)
})

test("a gone daemon on POST answers 503 daemonRestarted without resending", async () => {
  const h = harness({ steps: [gone(), new Response("never")] })
  const response = await h.proxy(new Request("servef-app://local/api/walkthroughs", { method: "POST", body: "{}" }))
  assert.equal(response.status, 503)
  assert.equal(await errorCode(response), "daemonRestarted")
  assert.equal(h.reensures, 1)
  assert.equal(h.forwarded.length, 1)
})

test("a retry that fails again answers JSON for APIs and a page for documents", async () => {
  const api = harness({ steps: [gone()] })
  const json = await api.proxy(new Request("servef-app://local/api/projects", { headers: { accept: "text/html" } }))
  assert.equal(json.status, 503)
  assert.equal(await errorCode(json), "daemonUnavailable")
  assert.equal(api.forwarded.length, 2)

  const doc = harness({ steps: [gone()] })
  const page = await doc.proxy(new Request("servef-app://local/view", { headers: { "sec-fetch-dest": "document" } }))
  assert.equal(page.status, 503)
  assert.match(page.headers.get("content-type") ?? "", /^text\/html/)

  const noRestart = harness({ steps: [gone()], reensure: [false] })
  const down = await noRestart.proxy(new Request("servef-app://local/api/projects"))
  assert.equal(await errorCode(down), "daemonUnavailable")
  assert.equal(noRestart.forwarded.length, 1)
})

test("an ensure failure shows instructions to documents and JSON to APIs", async () => {
  const doc = harness({ lookups: [failed], reensure: [false] })
  const page = await doc.proxy(new Request("servef-app://local/", { headers: { accept: "text/html,application/xhtml+xml" } }))
  assert.equal(page.status, 503)
  assert.equal(page.headers.get("content-security-policy"), "default-src 'none'; style-src 'unsafe-inline'")
  assert.match(await page.text(), /daemon did not start/)
  assert.equal(doc.reensures, 1, "a document reload retries ensure")

  const api = harness({ lookups: [failed] })
  const json = await api.proxy(new Request("servef-app://local/api/projects", { headers: { accept: "text/html" } }))
  assert.equal(json.status, 503)
  assert.equal(await errorCode(json), "daemonUnavailable")
  assert.equal(api.reensures, 0)
})

test("a document reload after a failure forwards once ensure recovers", async () => {
  const h = harness({ lookups: [failed, ready] })
  const response = await h.proxy(new Request("servef-app://local/", { headers: { "sec-fetch-dest": "document" } }))
  assert.equal(response.status, 200)
  assert.equal(h.reensures, 1)
  assert.equal(h.forwarded.length, 1)
})

test("stop and retry runs only for a mismatched connection", async () => {
  const h = harness({ lookups: [mismatch] })
  const response = await h.proxy(new Request(`servef-app://local${STOP_AND_RETRY_PATH}`, { method: "POST", headers: { origin: "servef-app://local" } }))
  assert.equal(response.status, 303)
  assert.equal(response.headers.get("location"), "/")
  assert.equal(h.stops, 1)

  const page = await h.proxy(new Request("servef-app://local/", { headers: { "sec-fetch-dest": "document" } }))
  assert.match(await page.text(), /Stop the running daemon and retry/)

  const healthy = harness()
  assert.equal((await healthy.proxy(new Request(`servef-app://local${STOP_AND_RETRY_PATH}`, { method: "POST" }))).status, 404)
  assert.equal(healthy.stops, 0)
  assert.equal((await h.proxy(new Request(`servef-app://local${STOP_AND_RETRY_PATH}`))).status, 404, "GET is not honored")
  assert.equal((await h.proxy(new Request("servef-app://local/_desktop/other", { method: "POST" }))).status, 404)
  const foreign = await h.proxy(new Request(`servef-app://local${STOP_AND_RETRY_PATH}`, { method: "POST", headers: { origin: "servef-app://ssh-box" } }))
  assert.equal(foreign.status, 403)
  assert.equal(h.stops, 1)
  assert.equal(h.forwarded.length + healthy.forwarded.length, 0)
})
