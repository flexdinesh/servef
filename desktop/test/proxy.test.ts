import assert from "node:assert/strict"
import { test } from "node:test"
import type { ConnectionLookup } from "../src/main/connections.ts"
import type { SshFailure } from "../src/main/ssh-errors.ts"
import type { ForwardRequest } from "../src/main/forward.ts"
import { STOP_AND_RETRY_PATH } from "../src/main/pages.ts"
import { createProxy } from "../src/main/proxy.ts"

const status = { protocol: 1, version: "1", pid: 1, socket: "/run/servef.sock", roots: [] }
const ready: ConnectionLookup = { kind: "ready", connection: { id: "local", kind: "daemon", origin: "servef-app://local", socket: "/run/servef.sock", status, label: "local" } }
const failed: ConnectionLookup = { kind: "failed", connection: "daemon", failure: { ok: false, kind: "failed", detail: "daemon did not start" }, label: "local" }
const mismatch: ConnectionLookup = { kind: "failed", connection: "daemon", failure: { ok: false, kind: "mismatch", detail: "protocol 2", status: { ...status, protocol: 2 } }, label: "local" }

const sshReady: ConnectionLookup = { kind: "ready", connection: { id: "ssh-devbox", kind: "ssh", origin: "servef-app://ssh-devbox", socket: "/run/f-1.sock", status: { ...status, socket: "/remote/servef.sock" }, label: "devbox" } }
const sshFailure = (kind: SshFailure["kind"], extra: Partial<SshFailure> = {}): ConnectionLookup =>
  ({ kind: "failed", connection: "ssh", failure: { kind, destination: "devbox", detail: `${kind} detail`, ...extra }, label: "devbox" })
const sshConnecting: ConnectionLookup = { kind: "connecting", label: "devbox", failure: { kind: "unreachable", destination: "devbox", detail: "Connection <timed> out" } }

const gone = () => Object.assign(new Error("connect ECONNREFUSED"), { code: "ECONNREFUSED" })

type Step = Response | Error

interface Harness {
  proxy: (request: Request) => Promise<Response>
  forwarded: ForwardRequest[]
  reensures: number
  stops: number
  looked: string[]
  app: Request[]
  dropped: string[]
}

// harness answers lookups for its connection id (local unless ssh is set)
// from lookups (last repeats), forwards from steps, and app pages with 200.
function harness(options: { lookups?: ConnectionLookup[]; steps?: Step[]; reensure?: boolean[]; id?: string } = {}): Harness {
  const lookups = [...(options.lookups ?? [ready])]
  const steps = [...(options.steps ?? [new Response("ok")])]
  const reensure = [...(options.reensure ?? [true])]
  const known = options.id ?? "local"
  const state: Harness = {
    proxy: () => { throw new Error("unset") },
    forwarded: [],
    reensures: 0,
    stops: 0,
    looked: [],
    app: [],
    dropped: [],
  }
  state.proxy = createProxy({
    packaged: true,
    release: "1.2.3",
    appPages: async (request) => {
      state.app.push(request)
      return new Response("app page")
    },
    lookup: async (id) => {
      state.looked.push(id)
      if (id !== known) return { kind: "unknown" }
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
    dropCli: (id) => { state.dropped.push(id) },
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
  assert.deepEqual(forwarded.target, { socket: "/run/servef.sock" })
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

test("app host requests go to app pages after the Origin guard, without a lookup", async () => {
  const h = harness()
  const page = await h.proxy(new Request("servef-app://app/connect"))
  assert.equal(await page.text(), "app page")
  const same = await h.proxy(new Request("servef-app://app/forget", { method: "POST", body: "id=ssh-devbox", headers: { origin: "servef-app://app", "content-type": "application/x-www-form-urlencoded" } }))
  assert.equal(same.status, 200)
  for (const origin of ["servef-app://local", "servef-app://ssh-devbox", "null", "https://example.com"]) {
    const foreign = await h.proxy(new Request("servef-app://app/forget", { method: "POST", body: "id=ssh-devbox", headers: { origin } }))
    assert.equal(foreign.status, 403, origin)
    assert.equal(await errorCode(foreign), "crossOrigin")
  }
  assert.equal(h.app.length, 2)
  assert.deepEqual(h.looked, [])
  assert.equal(h.forwarded.length, 0)
})

test("an unknown ssh origin is 404 and reaches nothing", async () => {
  const h = harness()
  for (const path of ["/", "/api/projects", "/_desktop/stop-and-retry"]) {
    const response = await h.proxy(new Request(`servef-app://ssh-stranger${path}`, { method: path.startsWith("/_desktop") ? "POST" : "GET", headers: { "sec-fetch-dest": "document" } }))
    assert.equal(response.status, 404, path)
    assert.equal(await errorCode(response), "unknownConnection")
  }
  assert.equal(h.forwarded.length + h.reensures + h.stops, 0)
})

test("a ready ssh connection forwards to its local forward socket", async () => {
  const h = harness({ id: "ssh-devbox", lookups: [sshReady] })
  const response = await h.proxy(new Request("servef-app://ssh-devbox/api/projects", { headers: { origin: "servef-app://ssh-devbox" } }))
  assert.equal(await response.text(), "ok")
  assert.deepEqual(h.forwarded[0]?.target, { socket: "/run/f-1.sock" })
  assert.equal(h.forwarded[0]?.headers.host, "localhost")
  const foreign = await h.proxy(new Request("servef-app://ssh-devbox/api/projects", { headers: { origin: "servef-app://local" } }))
  assert.equal(foreign.status, 403)
})

test("the daemon control API is refused for ssh connections", async () => {
  const h = harness({ id: "ssh-devbox", lookups: [sshReady] })
  for (const path of ["/_daemon/status", "/%5Fdaemon/stop", "//_daemon/events"]) {
    assert.equal((await h.proxy(new Request(`servef-app://ssh-devbox${path}`))).status, 403, path)
  }
  assert.equal(h.forwarded.length, 0)
})

test("ssh failures show ssh instructions to documents and JSON to APIs", async () => {
  const hostKey = harness({ id: "ssh-devbox", lookups: [sshFailure("hostKey")], reensure: [false] })
  const page = await hostKey.proxy(new Request("servef-app://ssh-devbox/", { headers: { "sec-fetch-dest": "document" } }))
  assert.equal(page.status, 503)
  assert.equal(page.headers.get("content-security-policy"), "default-src 'none'; style-src 'unsafe-inline'")
  const html = await page.text()
  assert.match(html, /hostKey detail/)
  assert.match(html, /Run ssh devbox in a terminal once to verify the host key, then Retry\./)
  assert.ok(!html.includes("<form"))
  assert.equal(hostKey.reensures, 1, "a document reload retries the connection")

  const missing = harness({ id: "ssh-devbox", lookups: [sshFailure("servefMissing", { os: "Linux", arch: "x86_64" })], reensure: [false] })
  const install = await (await missing.proxy(new Request("servef-app://ssh-devbox/view?project=%2Fa", { headers: { accept: "text/html" } }))).text()
  assert.match(install, /servef is not installed on devbox/)
  assert.match(install, /releases\/download\/v1\.2\.3\/servef_1\.2\.3_linux_amd64\.tar\.gz/)
  assert.match(install, /The app never installs anything on devbox/)

  const api = harness({ id: "ssh-devbox", lookups: [sshFailure("auth")] })
  const json = await api.proxy(new Request("servef-app://ssh-devbox/api/projects"))
  assert.equal(json.status, 503)
  assert.equal(await errorCode(json), "daemonUnavailable")
  assert.equal(api.reensures, 0)
})

test("a document reload of a failed ssh connection forwards once it recovers", async () => {
  const h = harness({ id: "ssh-devbox", lookups: [sshFailure("unreachable"), sshReady] })
  const response = await h.proxy(new Request("servef-app://ssh-devbox/", { headers: { "sec-fetch-dest": "document" } }))
  assert.equal(response.status, 200)
  assert.equal(h.reensures, 1)
  assert.deepEqual(h.forwarded[0]?.target, { socket: "/run/f-1.sock" })
})

test("a reconnecting ssh connection shows a refreshing page without re-ensuring", async () => {
  const h = harness({ id: "ssh-devbox", lookups: [sshConnecting] })
  const page = await h.proxy(new Request("servef-app://ssh-devbox/view?project=%2Fa", { headers: { "sec-fetch-dest": "document" } }))
  assert.equal(page.status, 503)
  const html = await page.text()
  assert.ok(html.includes("<title>Reconnecting to devbox</title>"))
  assert.ok(html.includes("<meta http-equiv=\"refresh\" content=\"3\">"))
  assert.ok(html.includes("Connection &lt;timed&gt; out"))
  const json = await h.proxy(new Request("servef-app://ssh-devbox/api/projects"))
  assert.equal(json.status, 503)
  assert.equal(await errorCode(json), "daemonUnavailable")
  assert.equal(h.reensures, 0)
  assert.equal(h.forwarded.length, 0)
})

test("stop and retry runs for a mismatched ssh connection", async () => {
  const h = harness({ id: "ssh-devbox", lookups: [sshFailure("mismatch", { status: { ...status, protocol: 2 } })] })
  const page = await (await h.proxy(new Request("servef-app://ssh-devbox/", { headers: { "sec-fetch-dest": "document" } }))).text()
  assert.match(page, /Stop the running daemon and retry/)
  const response = await h.proxy(new Request(`servef-app://ssh-devbox${STOP_AND_RETRY_PATH}`, { method: "POST", headers: { origin: "servef-app://ssh-devbox" } }))
  assert.equal(response.status, 303)
  assert.equal(h.stops, 1)

  const other = harness({ id: "ssh-devbox", lookups: [sshFailure("hostKey")] })
  assert.equal((await other.proxy(new Request(`servef-app://ssh-devbox${STOP_AND_RETRY_PATH}`, { method: "POST" }))).status, 404)
  assert.equal(other.stops, 0)
})

test("a reset before the response head re-ensures and retries a GET", async () => {
  const reset = Object.assign(new Error("socket hang up"), { code: "ECONNRESET" })
  const h = harness({ id: "ssh-devbox", lookups: [sshReady], steps: [reset, new Response("again")] })
  const response = await h.proxy(new Request("servef-app://ssh-devbox/api/projects"))
  assert.equal(await response.text(), "again")
  assert.equal(h.reensures, 1)
  assert.equal(h.forwarded.length, 2)
})

// CLI servers (decision 0071).

const cliReady: ConnectionLookup = { kind: "ready", connection: { id: "cli-7971", kind: "cli", origin: "servef-app://cli-7971", host: "127.0.0.1", port: 7971, label: "localhost:7971" } }
const reset = () => Object.assign(new Error("socket hang up"), { code: "ECONNRESET" })
const documentHeaders = { "sec-fetch-dest": "document", accept: "text/html" }

test("a ready CLI server forwards over TCP with Host localhost:<port>", async () => {
  const h = harness({ id: "cli-7971", lookups: [cliReady] })
  const response = await h.proxy(new Request("servef-app://cli-7971/api/projects", { headers: { origin: "servef-app://cli-7971", accept: "application/json" } }))
  assert.equal(await response.text(), "ok")
  const [forwarded] = h.forwarded
  assert.ok(forwarded !== undefined)
  assert.deepEqual(forwarded.target, { host: "127.0.0.1", port: 7971 })
  assert.deepEqual(forwarded.headers, { accept: "application/json", host: "localhost:7971", "sec-fetch-site": "same-origin" })
  const foreign = await h.proxy(new Request("servef-app://cli-7971/api/projects", { headers: { origin: "servef-app://local" } }))
  assert.equal(foreign.status, 403)
})

test("the CLI control endpoint is refused for every connection", async () => {
  for (const id of ["cli-7971", "local"]) {
    const h = harness({ id, lookups: [id === "local" ? ready : cliReady] })
    for (const path of ["/_servef/control", "/%5Fservef/control", "//_servef/control", "/_servef"]) {
      const response = await h.proxy(new Request(`servef-app://${id}${path}`, { method: "POST" }))
      assert.equal(response.status, 403, `${id}${path}`)
    }
    assert.equal(h.forwarded.length, 0)
    assert.equal(h.looked.length, 0)
  }
})

test("an unknown CLI port shows a no-server page to documents and 404 JSON to the API", async () => {
  const h = harness({ id: "cli-7971", lookups: [cliReady] })
  const page = await h.proxy(new Request("servef-app://cli-8000/?project=%2Fsrc", { headers: documentHeaders }))
  assert.equal(page.status, 404)
  assert.match(page.headers.get("content-type") ?? "", /text\/html/)
  const html = await page.text()
  assert.match(html, /No servef server is running on port 8000/)
  assert.match(html, /<a href="">Retry<\/a>/)
  const api = await h.proxy(new Request("servef-app://cli-8000/api/projects"))
  assert.equal(api.status, 404)
  assert.equal(await errorCode(api), "unknownConnection")
  assert.equal(h.forwarded.length, 0)
})

test("a stopped CLI server is dropped and shows the stopped page; nothing is re-ensured", async () => {
  const h = harness({ id: "cli-7971", lookups: [cliReady, { kind: "unknown" }], steps: [gone()] })
  const page = await h.proxy(new Request("servef-app://cli-7971/", { headers: documentHeaders }))
  assert.equal(page.status, 503)
  const html = await page.text()
  assert.match(html, /The servef server on port 7971 stopped/)
  assert.match(html, /servef browser --port 7971/)
  assert.deepEqual(h.dropped, ["cli-7971"])
  assert.equal(h.reensures, 0)
  assert.equal(h.forwarded.length, 1)
})

test("a CLI read retries once when the list still reports the server", async () => {
  const h = harness({ id: "cli-7971", lookups: [cliReady], steps: [reset(), new Response("again")] })
  const response = await h.proxy(new Request("servef-app://cli-7971/api/projects"))
  assert.equal(await response.text(), "again")
  assert.deepEqual(h.dropped, ["cli-7971"])
  assert.equal(h.forwarded.length, 2)
  assert.equal(h.reensures, 0)
})

test("a CLI write that finds the server gone is not resent", async () => {
  const h = harness({ id: "cli-7971", lookups: [cliReady], steps: [gone(), new Response("never")] })
  const response = await h.proxy(new Request("servef-app://cli-7971/api/walkthroughs?project=%2Fsrc", { method: "DELETE" }))
  assert.equal(response.status, 503)
  assert.equal(await errorCode(response), "serverStopped")
  assert.equal(h.forwarded.length, 1)
})
