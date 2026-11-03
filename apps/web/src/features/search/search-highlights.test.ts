import assert from "node:assert/strict"
import test from "node:test"

import { highlightedParts } from "./search-highlights.ts"

test("highlight segments preserve hostile text without markup interpretation", () => {
  assert.deepEqual(highlightedParts("<script>alpha</script>", [{ start: 8, end: 13 }]), [
    { highlighted: false, text: "<script>" },
    { highlighted: true, text: "alpha" },
    { highlighted: false, text: "</script>" },
  ])
})

test("overlapping or out-of-range highlights cannot duplicate text", () => {
  const text = "abcdef"
  const parts = highlightedParts(text, [{ start: -3, end: 3 }, { start: 2, end: 99 }])
  assert.equal(parts.map((part) => part.text).join(""), text)
})
