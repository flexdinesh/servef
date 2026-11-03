import assert from "node:assert/strict"
import test from "node:test"

import type { FileResult } from "./search-engine.ts"
import { adjacentMatch, isSearchOutcome, resultRows, resultSummary } from "./search-results.ts"

function file(path: string, count: number): FileResult {
  return {
    path,
    name: path.split("/").at(-1) ?? path,
    matches: Array.from({ length: count }, (_, index) => ({
      line: index + 1, column: 0, endColumn: 1, preview: "x", highlight: { start: 0, end: 1 },
    })),
  }
}

const files = [file("a.md", 2), file("b/c.md", 1)]

test("rows list each file followed by its matches unless collapsed", () => {
  assert.deepEqual(resultRows(files, new Set()).map((row) => row.key), [
    "file:a.md", "match:a.md:0", "match:a.md:1", "file:b/c.md", "match:b/c.md:0",
  ])
  assert.deepEqual(resultRows(files, new Set(["a.md"])).map((row) => row.key), ["file:a.md", "file:b/c.md", "match:b/c.md:0"])
})

test("adjacent matches step through all files and wrap", () => {
  assert.deepEqual(adjacentMatch(files, null, 1), { path: "a.md", matchIndex: 0 })
  assert.deepEqual(adjacentMatch(files, null, -1), { path: "b/c.md", matchIndex: 0 })
  assert.deepEqual(adjacentMatch(files, { path: "a.md", matchIndex: 1 }, 1), { path: "b/c.md", matchIndex: 0 })
  assert.deepEqual(adjacentMatch(files, { path: "b/c.md", matchIndex: 0 }, 1), { path: "a.md", matchIndex: 0 })
  assert.deepEqual(adjacentMatch(files, { path: "a.md", matchIndex: 0 }, -1), { path: "b/c.md", matchIndex: 0 })
  assert.equal(adjacentMatch([], null, 1), null)
})

test("summaries count results and files like VS Code", () => {
  assert.equal(resultSummary(1, 1, false), "1 result in 1 file")
  assert.equal(resultSummary(3, 2, false), "3 results in 2 files")
  assert.equal(resultSummary(2000, 12, true), "2,000+ results in 12 files")
})

test("worker outcomes are validated before use", () => {
  assert.equal(isSearchOutcome({ kind: "results", files, matchCount: 3, truncated: false }), true)
  assert.equal(isSearchOutcome({ kind: "invalid", field: "pattern", message: "Invalid regular expression." }), true)
  assert.equal(isSearchOutcome({ kind: "results", files: [{ path: "a.md" }], matchCount: 1, truncated: false }), false)
  assert.equal(isSearchOutcome({ kind: "invalid", field: "other", message: "" }), false)
})
