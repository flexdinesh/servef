import assert from "node:assert/strict"
import test from "node:test"

import type { FileDiffMetadata } from "@pierre/diffs"

import { parseTextDiff } from "./diff-content.ts"
import { diffLineTarget } from "./diff-line-selection.ts"
import { adjacentHunk, gapForLine, hunkCursor, hunkSelection } from "./diff-hunks.ts"

function diff(before: string | null, after: string | null): FileDiffMetadata {
  const side = (contents: string | null) => contents === null ? null : { name: "notes.txt", contents }
  const fileDiff = parseTextDiff({ kind: "text", oldFile: side(before), newFile: side(after) }, false)
  assert.ok(fileDiff)
  return fileDiff
}

function numbered(count: number, change: (line: number) => string[] = (line) => [`line ${line}`]): string {
  return Array.from({ length: count }, (_, index) => change(index + 1).map((value) => `${value}\n`).join("")).join("")
}

// Line 10 became two lines; line 30 was deleted. Four lines of context
// leave gaps 1-5, 15-25 (new 16-26), and 35-40 collapsed.
const original = numbered(40)
const edited = diff(original, numbered(40, (line) => line === 10 ? ["ten", "TEN"] : line === 30 ? [] : [`line ${line}`]))

test("a hunk selects its rows as shown, context included", () => {
  assert.equal(edited.hunks.length, 2)
  for (const style of ["unified", "split"] as const) {
    assert.deepEqual(hunkSelection(edited, 0, style), { start: 6, side: "additions", end: 15, endSide: "additions" })
    assert.deepEqual(hunkSelection(edited, 1, style), { start: 27, side: "additions", end: 34, endSide: "additions" })
  }
  assert.equal(hunkSelection(edited, 2, "unified"), null)
})

test("a hunk ending in deletions ends on the old side", () => {
  // Without context after it, the last row is a deletion.
  const tail = diff(numbered(5), numbered(3))
  for (const style of ["unified", "split"] as const) {
    assert.deepEqual(hunkSelection(tail, 0, style), { start: 1, side: "additions", end: 5, endSide: "deletions" })
  }
  const deleted = diff(numbered(3), null)
  assert.deepEqual(hunkSelection(deleted, 0, "unified"), { start: 1, side: "deletions", end: 3, endSide: "deletions" })
  const added = diff(null, numbered(3))
  assert.deepEqual(hunkSelection(added, 0, "split"), { start: 1, side: "additions", end: 3, endSide: "additions" })
})

test("a hunk starting with a change starts where its rows start", () => {
  // Line 1 became A: Unified shows the deletion first, Split pairs them.
  const head = diff(numbered(8), numbered(8, (line) => [line === 1 ? "A" : `line ${line}`]))
  assert.deepEqual(hunkSelection(head, 0, "unified"), { start: 1, side: "deletions", end: 5, endSide: "additions" })
  assert.deepEqual(hunkSelection(head, 0, "split"), { start: 1, side: "additions", end: 5, endSide: "additions" })
  // The cross-side selection still copies the new lines it covers.
  const selection = hunkSelection(head, 0, "unified")
  assert.ok(selection)
  assert.deepEqual(diffLineTarget(head, selection, "unified"), { side: "new", range: { start: 1, end: 5 } })
})

test("lines place a cursor inside or between hunks", () => {
  assert.deepEqual(hunkCursor(edited, "additions", 3), { kind: "before", index: 0 })
  assert.deepEqual(hunkCursor(edited, "additions", 7), { kind: "at", index: 0 })
  assert.deepEqual(hunkCursor(edited, "additions", 20), { kind: "before", index: 1 })
  assert.deepEqual(hunkCursor(edited, "deletions", 30), { kind: "at", index: 1 })
  assert.deepEqual(hunkCursor(edited, "additions", 38), { kind: "before", index: 2 })
})

test("hunk steps wrap within the file", () => {
  assert.equal(adjacentHunk(2, { kind: "at", index: 0 }, 1), 1)
  assert.equal(adjacentHunk(2, { kind: "at", index: 1 }, 1), 0)
  assert.equal(adjacentHunk(2, { kind: "at", index: 0 }, -1), 1)
  assert.equal(adjacentHunk(2, { kind: "before", index: 1 }, 1), 1)
  assert.equal(adjacentHunk(2, { kind: "before", index: 1 }, -1), 0)
  assert.equal(adjacentHunk(2, { kind: "before", index: 0 }, -1), 1)
  assert.equal(adjacentHunk(2, { kind: "before", index: 2 }, 1), 0)
  assert.equal(adjacentHunk(0, { kind: "before", index: 0 }, 1), null)
})

test("gaps between hunks hold the unchanged lines they collapse", () => {
  assert.deepEqual(gapForLine(edited, "additions", 2), { hunkIndex: 0, lines: 5, newLine: 2 })
  assert.deepEqual(gapForLine(edited, "additions", 20), { hunkIndex: 1, lines: 11, newLine: 20 })
  // Old lines map to the same unchanged line on the new side.
  assert.deepEqual(gapForLine(edited, "deletions", 20), { hunkIndex: 1, lines: 11, newLine: 21 })
  assert.deepEqual(gapForLine(edited, "additions", 40), { hunkIndex: 2, lines: 6, newLine: 40 })
  for (const line of [6, 15, 34, 41]) assert.equal(gapForLine(edited, "additions", line), null, String(line))
  assert.equal(gapForLine(edited, "deletions", 30), null)
})
