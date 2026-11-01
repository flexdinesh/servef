import assert from "node:assert/strict"
import { test } from "node:test"
import { failureInstructions } from "../src/main/instructions.ts"
import { failurePage, jsonError, STOP_AND_RETRY_PATH, unavailablePage } from "../src/main/pages.ts"

const status = { protocol: 2, version: "2.0.0", pid: 1, socket: "/run/s.sock", roots: [] }

test("missing instructions differ for packaged and dev builds", () => {
  const packaged = failureInstructions({ ok: false, kind: "missing", detail: "no binary" }, true)
  assert.match(packaged.paragraphs.join(" "), /Reinstall the servef app/)
  assert.equal(packaged.stopAndRetry, false)
  const dev = failureInstructions({ ok: false, kind: "missing", detail: "no binary" }, false)
  assert.match(dev.paragraphs.join(" "), /SERVEF_DESKTOP_BINARY/)
  assert.deepEqual(dev.commands, ["mise run dev:desktop"])
})

test("mismatch instructions name both protocols and offer stop and retry", () => {
  const instructions = failureInstructions({ ok: false, kind: "mismatch", detail: "x", status }, true)
  const text = instructions.paragraphs.join(" ")
  assert.match(text, /protocol 2/)
  assert.match(text, /protocol 1/)
  assert.match(text, /2\.0\.0/)
  assert.deepEqual(instructions.commands, ["servef daemon stop"])
  assert.equal(instructions.stopAndRetry, true)
})

test("failed instructions point at the daemon log when the socket is known", () => {
  assert.match(failureInstructions({ ok: false, kind: "failed", detail: "boom", status }, true).paragraphs.join(" "), /\/run\/s\.sock\.log/)
  const generic = failureInstructions({ ok: false, kind: "failed", detail: "timed out" }, true)
  assert.equal(generic.paragraphs[0], "timed out")
  assert.doesNotMatch(generic.paragraphs.join(" "), /\.log/)
})

test("failurePage escapes text, sets its own CSP, and posts stop and retry", async () => {
  const response = failurePage({ title: "<b>t</b>", paragraphs: ["a & \"b\" <script>"], commands: ["servef daemon stop"], stopAndRetry: true })
  assert.equal(response.status, 503)
  assert.equal(response.headers.get("content-type"), "text/html; charset=utf-8")
  assert.equal(response.headers.get("content-security-policy"), "default-src 'none'; style-src 'unsafe-inline'")
  assert.equal(response.headers.get("cache-control"), "no-store")
  const html = await response.text()
  assert.ok(!html.includes("<script>"))
  assert.ok(html.includes("&lt;b&gt;t&lt;/b&gt;"))
  assert.ok(html.includes("a &amp; &quot;b&quot; &lt;script&gt;"))
  assert.ok(html.includes("<a href=\"\">Retry</a>"))
  assert.ok(html.includes(`<form method="post" action="${STOP_AND_RETRY_PATH}"><button>Stop the running daemon and retry</button></form>`))
  assert.ok(html.includes("prefers-color-scheme"))
  const plain = await failurePage({ title: "t", paragraphs: [], commands: [], stopAndRetry: false }).text()
  assert.ok(!plain.includes("<form"))
})

test("unavailablePage and jsonError", async () => {
  const page = unavailablePage("<gone>")
  assert.equal(page.status, 503)
  assert.ok((await page.text()).includes("&lt;gone&gt;"))
  const error = jsonError(503, "daemonUnavailable", "down")
  assert.equal(error.status, 503)
  assert.match(error.headers.get("content-type") ?? "", /^application\/json/)
  assert.deepEqual(await error.json(), { error: "down", code: "daemonUnavailable" })
})
