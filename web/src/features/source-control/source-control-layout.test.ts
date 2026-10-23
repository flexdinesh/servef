import assert from "node:assert/strict"
import test from "node:test"

import { clampShare, defaultLayout, maximumShare, minimumShare, parseLayout, withBase } from "./source-control-layout.ts"

test("layout defaults to the branch range, an even split, and no chosen bases", () => {
  assert.deepEqual(defaultLayout, { range: "branch", listShare: 0.5, bases: {} })
  assert.deepEqual(parseLayout(null), defaultLayout)
  assert.deepEqual(parseLayout([]), defaultLayout)
})

test("stored layout keeps well-formed fields, clamps the split, and carries the earlier split over", () => {
  assert.deepEqual(parseLayout({ range: "all", listShare: 0.3, bases: { "/a": "develop", "/b": 3, "/c": "" } }),
    { range: "all", listShare: 0.3, bases: { "/a": "develop" } })
  assert.equal(parseLayout({ changesShare: 0.4, changesCollapsed: true }).listShare, 0.4)
  assert.deepEqual(parseLayout({ range: "everything", listShare: Number.NaN, bases: [] }), defaultLayout)
  assert.equal(parseLayout({ listShare: 2 }).listShare, maximumShare)
  assert.equal(parseLayout({ listShare: -1 }).listShare, minimumShare)
  assert.equal(clampShare(0.4), 0.4)
})

test("chosen bases are per project, and choosing the default forgets one", () => {
  const chosen = withBase(withBase(defaultLayout, "/a", "develop"), "/b", "origin/main")
  assert.deepEqual(chosen.bases, { "/a": "develop", "/b": "origin/main" })
  assert.deepEqual(withBase(chosen, "/a", "").bases, { "/b": "origin/main" })
})
