import assert from "node:assert/strict"
import { test } from "node:test"
import { createSSEParser } from "../src/main/sse.ts"

function parse(chunks: string[]): [string, string][] {
  const events: [string, string][] = []
  const feed = createSSEParser((name, data) => events.push([name, data]))
  for (const chunk of chunks) feed(chunk)
  return events
}

test("parses named events, default names, and multiline data", () => {
  assert.deepEqual(parse(["event: status\ndata: {\"mode\":\"watching\"}\n\ndata: a\ndata: b\n\n"]), [
    ["status", "{\"mode\":\"watching\"}"],
    ["message", "a\nb"],
  ])
})

test("skips comments, ignores id and retry, and needs data to dispatch", () => {
  assert.deepEqual(parse([": heartbeat\n\nevent: ping\n\nid: 4\nretry: 10\nevent: change\ndata:x\n\n"]), [["change", "x"]])
})

test("strips one leading space and keeps the rest", () => {
  assert.deepEqual(parse(["data:  two\n\ndata\n\n"]), [["message", " two"], ["message", ""]])
})

test("handles chunk splits anywhere, including inside CRLF", () => {
  const stream = "event: head\r\ndata: 1\r\n\r\nevent: change\rdata: 2\r\rdata: 3\n\n"
  const expected: [string, string][] = [["head", "1"], ["change", "2"], ["message", "3"]]
  for (let size = 1; size <= stream.length; size++) {
    const chunks: string[] = []
    for (let index = 0; index < stream.length; index += size) chunks.push(stream.slice(index, index + size))
    assert.deepEqual(parse(chunks), expected, `chunk size ${size}`)
  }
  assert.deepEqual(parse(["data: 1\r", "", "\n\r\n"]), [["message", "1"]])
})

test("an unterminated event is not dispatched", () => {
  assert.deepEqual(parse(["event: status\ndata: 1\n"]), [])
})

test("strips a leading byte order mark", () => {
  assert.deepEqual(parse(["﻿data: x\n\n"]), [["message", "x"]])
})
