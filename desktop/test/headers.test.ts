import assert from "node:assert/strict"
import { test } from "node:test"
import { isEventStreamRequest, responseHeaders, upstreamHeaders } from "../src/main/headers.ts"

test("upstreamHeaders keeps the allowlist and sets Host and Sec-Fetch-Site", () => {
  const incoming = new Headers({
    accept: "application/json",
    "accept-language": "en",
    "content-type": "application/json",
    range: "bytes=0-1",
    "if-none-match": "\"etag\"",
    "last-event-id": "7",
    "x-servef-language-servers": "on",
    host: "evil.example",
    origin: "servef-app://local",
    referer: "servef-app://local/view",
    cookie: "a=b",
    "sec-fetch-site": "cross-site",
    "sec-fetch-dest": "document",
    "sec-fetch-mode": "navigate",
    "user-agent": "Electron",
  })
  assert.deepEqual(upstreamHeaders(incoming), {
    accept: "application/json",
    "accept-language": "en",
    "content-type": "application/json",
    range: "bytes=0-1",
    "if-none-match": "\"etag\"",
    "last-event-id": "7",
    "x-servef-language-servers": "on",
    host: "localhost",
    "sec-fetch-site": "same-origin",
  })
})

test("isEventStreamRequest reads Accept", () => {
  assert.equal(isEventStreamRequest(new Headers({ accept: "text/event-stream" })), true)
  assert.equal(isEventStreamRequest(new Headers({ accept: "Text/Event-Stream, */*" })), true)
  assert.equal(isEventStreamRequest(new Headers({ accept: "text/html" })), false)
  assert.equal(isEventStreamRequest(new Headers()), false)
})

test("responseHeaders drops hop-by-hop headers and appends arrays", () => {
  const headers = responseHeaders({
    "content-type": "text/html",
    "content-security-policy": "default-src 'self'",
    etag: "\"1\"",
    connection: "keep-alive",
    "keep-alive": "timeout=5",
    "transfer-encoding": "chunked",
    upgrade: "h2c",
    "set-cookie": ["a=1", "b=2"],
    vary: undefined,
  })
  assert.equal(headers.get("content-type"), "text/html")
  assert.equal(headers.get("content-security-policy"), "default-src 'self'")
  assert.equal(headers.get("etag"), "\"1\"")
  for (const name of ["connection", "keep-alive", "transfer-encoding", "upgrade", "vary"]) assert.equal(headers.has(name), false, name)
  assert.deepEqual(headers.getSetCookie(), ["a=1", "b=2"])
})
