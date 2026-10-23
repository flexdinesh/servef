import assert from "node:assert/strict"
import test from "node:test"

import { clampShare, defaultLayout, maximumShare, minimumShare, parseLayout } from "./source-control-layout.ts"

test("layout defaults to the branch range with both sections open and halved", () => {
  assert.deepEqual(defaultLayout, { range: "branch", changesCollapsed: false, commitsCollapsed: false, changesShare: 0.5 })
  assert.deepEqual(parseLayout(null), defaultLayout)
  assert.deepEqual(parseLayout([]), defaultLayout)
})

test("stored layout keeps well-formed fields and clamps the split", () => {
  assert.deepEqual(parseLayout({ range: "all", changesCollapsed: true, commitsCollapsed: false, changesShare: 0.3 }),
    { range: "all", changesCollapsed: true, commitsCollapsed: false, changesShare: 0.3 })
  assert.deepEqual(parseLayout({ range: "everything", changesCollapsed: "yes", changesShare: Number.NaN }), defaultLayout)
  assert.equal(parseLayout({ changesShare: 2 }).changesShare, maximumShare)
  assert.equal(parseLayout({ changesShare: -1 }).changesShare, minimumShare)
  assert.equal(clampShare(0.4), 0.4)
})
