import assert from "node:assert/strict"
import test from "node:test"

import {
  clampRailWidth, defaultLayout, maximumRailWidth, minimumRailWidth, paneIds, paneOpen, parseLayout, resizePair, withBase, type PaneId,
} from "./source-control-layout.ts"

test("layout defaults to the branch range, a flat change list, a shown rail, and no choices", () => {
  assert.equal(defaultLayout.range, "branch")
  assert.equal(defaultLayout.changesView, "list")
  assert.equal(defaultLayout.railHidden, false)
  assert.deepEqual(defaultLayout.collapsed, {})
  assert.deepEqual(paneIds, ["working", "commits", "branches"])
  assert.deepEqual(parseLayout(null), defaultLayout)
  assert.deepEqual(parseLayout([]), defaultLayout)
})

test("stored layout keeps well-formed fields and drops the earlier list split", () => {
  const parsed = parseLayout({
    range: "all", listShare: 0.3, bases: { "/a": "develop", "/b": 3, "/c": "" },
    weights: { working: 0.5, commits: -1, branches: "x" }, collapsed: { working: true, commits: "no", branches: false },
    changesView: "tree", railWidth: 20, railHidden: true,
  })
  assert.deepEqual(parsed, {
    range: "all", bases: { "/a": "develop" },
    weights: { working: 0.5, commits: defaultLayout.weights.commits, branches: defaultLayout.weights.branches },
    collapsed: { working: true, branches: false }, changesView: "tree", railWidth: 20, railHidden: true,
  })
  assert.deepEqual(parseLayout({ range: "everything", changesView: "grid", railWidth: Number.NaN, bases: [] }), defaultLayout)
  assert.equal(parseLayout({ railWidth: 99 }).railWidth, maximumRailWidth)
  assert.equal(clampRailWidth(1), minimumRailWidth)
})

test("empty panes start closed until the user chooses, and the choice wins", () => {
  assert.equal(paneOpen(undefined, true), false)
  assert.equal(paneOpen(undefined, false), true)
  // Unknown content reads as content, so panes do not flash closed.
  assert.equal(paneOpen(undefined, null), true)
  assert.equal(paneOpen(false, true), true)
  assert.equal(paneOpen(true, false), false)
})

test("a sash moves weight between its two panes within bounds", () => {
  const weights = { working: 0.3, commits: 0.5, branches: 0.2 }
  const open: PaneId[] = ["working", "commits", "branches"]
  const moved = resizePair(weights, open, "working", "commits", 0.1)
  assert.ok(Math.abs(moved.working - 0.4) < 1e-9)
  assert.ok(Math.abs(moved.commits - 0.4) < 1e-9)
  assert.equal(moved.branches, 0.2)
  const floored = resizePair(weights, open, "working", "commits", -5)
  assert.ok(Math.abs(floored.working - 0.8 * 0.15) < 1e-9)
  assert.ok(Math.abs(floored.working + floored.commits - 0.8) < 1e-9)
})

test("chosen bases are per project, and choosing the default forgets one", () => {
  const chosen = withBase(withBase(defaultLayout, "/a", "develop"), "/b", "origin/main")
  assert.deepEqual(chosen.bases, { "/a": "develop", "/b": "origin/main" })
  assert.deepEqual(withBase(chosen, "/a", "").bases, { "/b": "origin/main" })
})
