import assert from "node:assert/strict"
import test from "node:test"

import {
  clampLineRange,
  lineAnchor,
  lineRangeLabel,
  lineReference,
  parseLineAnchor,
  selectLine,
  splitSourceLines,
} from "./line-selection.ts"

test("line anchors parse GitHub-style fragments", () => {
  assert.deepEqual(parseLineAnchor("L12"), { start: 12, end: 12 })
  assert.deepEqual(parseLineAnchor("#L12"), { start: 12, end: 12 })
  assert.deepEqual(parseLineAnchor("L12-L20"), { start: 12, end: 20 })
  assert.deepEqual(parseLineAnchor("L12-20"), { start: 12, end: 20 })
  assert.deepEqual(parseLineAnchor("L20-L12"), { start: 12, end: 20 })
})

test("non-line fragments are not line anchors", () => {
  for (const hash of ["", "L", "L0", "L01", "L-3", "l12", "L12-", "L12-L", "L1.5", "heading", "L12x", "L99999999999999999999"]) {
    assert.equal(parseLineAnchor(hash), null, hash)
  }
})

test("line anchors format single lines and ranges", () => {
  assert.equal(lineAnchor({ start: 7, end: 7 }), "L7")
  assert.equal(lineAnchor({ start: 7, end: 9 }), "L7-L9")
  assert.deepEqual(parseLineAnchor(lineAnchor({ start: 3, end: 40 })), { start: 3, end: 40 })
})

test("ranges clamp to the document and vanish past its end", () => {
  assert.deepEqual(clampLineRange({ start: 2, end: 9 }, 5), { start: 2, end: 5 })
  assert.deepEqual(clampLineRange({ start: 5, end: 5 }, 5), { start: 5, end: 5 })
  assert.equal(clampLineRange({ start: 6, end: 9 }, 5), null)
  assert.equal(clampLineRange(null, 5), null)
})

test("plain clicks select one line; Shift extends from the anchor", () => {
  assert.deepEqual(selectLine(null, null, 4, false), { start: 4, end: 4 })
  assert.deepEqual(selectLine({ start: 2, end: 8 }, 2, 5, false), { start: 5, end: 5 })
  assert.deepEqual(selectLine({ start: 4, end: 4 }, 4, 9, true), { start: 4, end: 9 })
  assert.deepEqual(selectLine({ start: 4, end: 9 }, 4, 1, true), { start: 1, end: 4 })
  // Without a remembered anchor (e.g. after loading #L10-L12), Shift extends from the start.
  assert.deepEqual(selectLine({ start: 10, end: 12 }, null, 20, true), { start: 10, end: 20 })
  assert.deepEqual(selectLine(null, null, 3, true), { start: 3, end: 3 })
})

test("references and labels use editor conventions", () => {
  assert.equal(lineReference("web/src/App.tsx", { start: 12, end: 12 }), "web/src/App.tsx:12")
  assert.equal(lineReference("web/src/App.tsx", { start: 12, end: 20 }), "web/src/App.tsx:12-20")
  assert.equal(lineRangeLabel({ start: 12, end: 12 }), "Ln 12")
  assert.equal(lineRangeLabel({ start: 12, end: 20 }), "Ln 12–20")
})

test("source lines drop one trailing newline only", () => {
  assert.deepEqual(splitSourceLines("a\nb\n"), ["a", "b"])
  assert.deepEqual(splitSourceLines("a\r\nb\n\n"), ["a", "b", ""])
  assert.deepEqual(splitSourceLines(""), [""])
})
