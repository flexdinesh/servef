import assert from "node:assert/strict"
import { test } from "node:test"
import { failurePage, jsonError, reconnectingPage, STOP_AND_RETRY_PATH, unavailablePage } from "../src/main/pages.ts"

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

test("reconnectingPage refreshes itself, escapes text, and keeps the page CSP", async () => {
  const response = reconnectingPage("Reconnecting to <dev>", "ssh: connect to host dev & \"x\" timed out")
  assert.equal(response.status, 503)
  assert.equal(response.headers.get("content-type"), "text/html; charset=utf-8")
  assert.equal(response.headers.get("content-security-policy"), "default-src 'none'; style-src 'unsafe-inline'")
  assert.equal(response.headers.get("cache-control"), "no-store")
  const html = await response.text()
  assert.ok(html.includes("<meta http-equiv=\"refresh\" content=\"3\">"))
  assert.ok(html.includes("<title>Reconnecting to &lt;dev&gt;</title>"))
  assert.ok(html.includes("dev &amp; &quot;x&quot; timed out"))
  assert.ok(html.includes("<a href=\"\">Retry</a>"))
  assert.ok(!html.includes("<script"))
  assert.ok(!html.includes("<form"))
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
