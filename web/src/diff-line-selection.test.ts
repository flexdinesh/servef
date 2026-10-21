import assert from "node:assert/strict"
import test from "node:test"

import type { FileDiffMetadata } from "@pierre/diffs"

import { parseTextDiff } from "./diff-content.ts"
import { diffLineAnchor, diffLineTarget, parseDiffLineAnchor } from "./diff-line-selection.ts"

function diff(before: string | null, after: string | null): FileDiffMetadata {
  const side = (contents: string | null) => contents === null ? null : { name: "notes.txt", contents }
  const fileDiff = parseTextDiff({ kind: "text", oldFile: side(before), newFile: side(after) }, false)
  assert.ok(fileDiff)
  return fileDiff
}

function lines(...values: string[]): string {
  return values.map((value) => `${value}\n`).join("")
}

// Old b (2) became B and X (new 2-3); a, c, d, e are unchanged.
const edited = diff(lines("a", "b", "c", "d", "e"), lines("a", "B", "X", "c", "d", "e"))

test("diff anchors read and write GitHub-style sides", () => {
  assert.deepEqual(parseDiffLineAnchor("#R4"), { start: 4, side: "additions", end: 4, endSide: "additions" })
  assert.deepEqual(parseDiffLineAnchor("#L6-L4"), { start: 4, side: "deletions", end: 6, endSide: "deletions" })
  assert.deepEqual(parseDiffLineAnchor("#R4-6"), { start: 4, side: "additions", end: 6, endSide: "additions" })
  assert.deepEqual(parseDiffLineAnchor("#L5-R8"), { start: 5, side: "deletions", end: 8, endSide: "additions" })
  for (const hash of ["", "#", "#L0", "#X1", "#R1-", "#R1-X2", "#12", "#R99999999999999999999"]) {
    assert.equal(parseDiffLineAnchor(hash), null, hash)
  }

  assert.equal(diffLineAnchor({ start: 4, side: "additions", end: 4, endSide: "additions" }), "R4")
  assert.equal(diffLineAnchor({ start: 4, end: 4 }), "R4")
  assert.equal(diffLineAnchor({ start: 4, side: "deletions", end: 6 }), "L4-L6")
  assert.equal(diffLineAnchor({ start: 6, side: "deletions", end: 4 }), "L4-L6")
  assert.equal(diffLineAnchor({ start: 5, side: "deletions", end: 5, endSide: "additions" }), "L5-R5")
})

test("one side keeps its own line numbers", () => {
  for (const style of ["unified", "split"] as const) {
    assert.deepEqual(diffLineTarget(edited, { start: 3, side: "additions", end: 1, endSide: "additions" }, style),
      { side: "new", range: { start: 1, end: 3 } })
    assert.deepEqual(diffLineTarget(edited, { start: 2, side: "deletions", end: 2 }, style),
      { side: "old", range: { start: 2, end: 2 } })
    // Unchanged lines on the old side (Split's left column) stay old lines.
    assert.deepEqual(diffLineTarget(edited, { start: 3, side: "deletions", end: 5, endSide: "deletions" }, style),
      { side: "old", range: { start: 3, end: 5 } })
  }
})

test("a range across sides covers the new lines between its ends", () => {
  for (const style of ["unified", "split"] as const) {
    // Deleted b down to added X: new B and X.
    assert.deepEqual(diffLineTarget(edited, { start: 2, side: "deletions", end: 3, endSide: "additions" }, style),
      { side: "new", range: { start: 2, end: 3 } })
    // The same range dragged upward.
    assert.deepEqual(diffLineTarget(edited, { start: 3, side: "additions", end: 2, endSide: "deletions" }, style),
      { side: "new", range: { start: 2, end: 3 } })
    // a down to deleted b: Unified shows b before B; Split shows them side by side.
    assert.deepEqual(diffLineTarget(edited, { start: 1, side: "additions", end: 2, endSide: "deletions" }, style),
      { side: "new", range: { start: 1, end: style === "unified" ? 1 : 2 } })
    // Deleted b down to c: everything new from B.
    assert.deepEqual(diffLineTarget(edited, { start: 2, side: "deletions", end: 4, endSide: "additions" }, style),
      { side: "new", range: { start: 2, end: 4 } })
  }
})

test("rows follow the shown layout: Unified lists deletions first, Split pairs them", () => {
  // Old b and c (2-3) became X (new 2). Unified: a, b, c, X, d. Split: a, b|X, c, d.
  const shrunk = diff(lines("a", "b", "c", "d"), lines("a", "X", "d"))
  const aToC = { start: 1, side: "additions" as const, end: 3, endSide: "deletions" as const }
  assert.deepEqual(diffLineTarget(shrunk, aToC, "unified"), { side: "new", range: { start: 1, end: 1 } })
  assert.deepEqual(diffLineTarget(shrunk, aToC, "split"), { side: "new", range: { start: 1, end: 2 } })
  const cToD = { start: 3, side: "deletions" as const, end: 3, endSide: "additions" as const }
  assert.deepEqual(diffLineTarget(shrunk, cToD, "unified"), { side: "new", range: { start: 2, end: 3 } })
  assert.deepEqual(diffLineTarget(shrunk, cToD, "split"), { side: "new", range: { start: 3, end: 3 } })
})

test("lines outside hunks keep their numbers on both sides", () => {
  const before = Array.from({ length: 40 }, (_, index) => `line ${index + 1}`)
  const after = [...before.slice(0, 19), "inserted", ...before.slice(19)]
  const long = diff(lines(...before), lines(...after))
  // New line 20 was inserted; old 30 is new 31.
  assert.deepEqual(diffLineTarget(long, { start: 31, side: "additions", end: 31 }, "split"), { side: "new", range: { start: 31, end: 31 } })
  assert.deepEqual(diffLineTarget(long, { start: 30, side: "deletions", end: 30 }, "split"), { side: "old", range: { start: 30, end: 30 } })
  // Split: old 5 on the left down to the inserted line covers new 5-20.
  assert.deepEqual(diffLineTarget(long, { start: 5, side: "deletions", end: 20, endSide: "additions" }, "split"),
    { side: "new", range: { start: 5, end: 20 } })
  // Old 30 sits beside new 31.
  assert.deepEqual(diffLineTarget(long, { start: 20, side: "additions", end: 30, endSide: "deletions" }, "split"),
    { side: "new", range: { start: 20, end: 31 } })
  // Past either file's end selects nothing.
  assert.equal(diffLineTarget(long, { start: 42, side: "additions", end: 42 }, "unified"), null)
  assert.equal(diffLineTarget(long, { start: 41, side: "deletions", end: 41 }, "unified"), null)
  assert.deepEqual(diffLineTarget(long, { start: 41, side: "additions", end: 41 }, "unified"), { side: "new", range: { start: 41, end: 41 } })
})

test("added and deleted files have one side", () => {
  const added = diff(null, lines("a", "b"))
  assert.deepEqual(diffLineTarget(added, { start: 1, side: "additions", end: 2 }, "split"), { side: "new", range: { start: 1, end: 2 } })
  assert.equal(diffLineTarget(added, { start: 1, side: "deletions", end: 1 }, "split"), null)
  const deleted = diff(lines("a", "b"), null)
  assert.deepEqual(diffLineTarget(deleted, { start: 2, side: "deletions", end: 2 }, "unified"), { side: "old", range: { start: 2, end: 2 } })
  assert.equal(diffLineTarget(deleted, { start: 1, side: "additions", end: 1 }, "unified"), null)
})
