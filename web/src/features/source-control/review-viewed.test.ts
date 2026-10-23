import assert from "node:assert/strict"
import test from "node:test"

import { isViewed, parseViewed, withViewed } from "./review-viewed.ts"

const blob = "a".repeat(40)
const changed = "b".repeat(40)

test("a viewed mark holds only while the file's blob matches", () => {
  const marks = withViewed({}, "/p", "a.md", blob, true)
  assert.equal(isViewed(marks, "/p", "a.md", blob), true)
  assert.equal(isViewed(marks, "/p", "a.md", changed), false)
  assert.equal(isViewed(marks, "/q", "a.md", blob), false)
  assert.equal(isViewed(marks, "/p", "a.md", ""), false)
  assert.equal(isViewed(withViewed(marks, "/p", "a.md", blob, false), "/p", "a.md", blob), false)
  assert.equal(isViewed(withViewed({}, "/p", "a.md", "", true), "/p", "a.md", ""), false)
})

test("stored marks keep well-formed entries only", () => {
  assert.deepEqual(parseViewed({ "/p": { "a.md": blob, "b.md": "short", "c.md": 3 }, "/q": [], "/r": "x", "/s": {} }), { "/p": { "a.md": blob }, "/s": {} })
  assert.deepEqual(parseViewed(null), {})
})
