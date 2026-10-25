import assert from "node:assert/strict"
import test from "node:test"

import type { CompareContext } from "../features/source-control/compare.ts"
import { documentTab, includesTab, parseStoredTab, reviewTab, sameTab, storedTab, tabKey, tabName, tabSearch } from "./workspace-tabs.ts"

const hash = "3577ec6a1b2c3d4e5f60718293a4b5c6d7e8f901"
const other = "4f76e9a1b2c3d4e5f60718293a4b5c6d7e8f9012"
const context: CompareContext = { base: "main", subject: (candidate) => candidate === hash ? "fix: preserve language server crash diagnostics" : "" }

test("documents and reviews are distinct tabs; one review per compare", () => {
  const working = reviewTab({ kind: "working" })
  assert.equal(sameTab(documentTab("a.md"), documentTab("a.md")), true)
  assert.equal(sameTab(documentTab("a.md"), working), false)
  assert.equal(sameTab(working, reviewTab({ kind: "working" })), true)
  assert.equal(sameTab(reviewTab({ kind: "commit", hash }), reviewTab({ kind: "commit", hash: other })), false)
  assert.equal(includesTab([working], reviewTab({ kind: "branch", base: "", head: "" })), false)
  const keys = [documentTab("working"), working, reviewTab({ kind: "branch", base: "", head: "" }), reviewTab({ kind: "branch", base: "develop", head: "" }),
    reviewTab({ kind: "commit", hash }), reviewTab({ kind: "range", from: other, to: hash })].map(tabKey)
  assert.equal(new Set(keys).size, keys.length)
})

test("tabs name documents by file and reviews by what they compare", () => {
  assert.deepEqual(tabName(documentTab("web/src/App.tsx"), context), { label: "App.tsx", title: "web/src/App.tsx" })
  assert.equal(tabName(reviewTab({ kind: "working" }), context).label, "Uncommitted")
  assert.equal(tabName(reviewTab({ kind: "branch", base: "", head: "" }), context).label, "vs main")
  assert.equal(tabName(reviewTab({ kind: "branch", base: "origin/develop", head: "" }), context).label, "vs origin/develop")
  assert.equal(tabName(reviewTab({ kind: "branch", base: "", head: "" }), { ...context, base: "" }).label, "Branch")
  assert.equal(tabName(reviewTab({ kind: "commit", hash }), context).label, "3577ec6 fix: preserve language server crash dia…")
  assert.equal(tabName(reviewTab({ kind: "commit", hash: other }), context).label, "4f76e9a")
  assert.equal(tabName(reviewTab({ kind: "range", from: other, to: hash }), context).label, "4f76e9a..3577ec6")
})

test("stored tabs round-trip and migrate per-file diff entries to their review", () => {
  for (const tab of [documentTab("a.md"), reviewTab({ kind: "working" }), reviewTab({ kind: "branch", base: "develop", head: "" }),
    reviewTab({ kind: "commit", hash }), reviewTab({ kind: "range", from: other, to: hash })]) {
    assert.deepEqual(parseStoredTab(storedTab(tab)), tab)
  }
  assert.deepEqual(parseStoredTab({ diff: "a.md" }), reviewTab({ kind: "working" }))
  assert.deepEqual(parseStoredTab({ commit: hash, path: "a.md" }), reviewTab({ kind: "commit", hash }))
  assert.deepEqual(parseStoredTab({ branch: "a.md" }), reviewTab({ kind: "branch", base: "", head: "" }))
  assert.deepEqual(parseStoredTab({ from: other, to: hash, path: "a.md" }), reviewTab({ kind: "range", from: other, to: hash }))
  for (const malformed of ["", { diff: "" }, { diff: 1 }, { path: "a.md" }, ["a.md"], null, 3, { commit: "3577ec6" },
    { review: "commit", commit: "x" }, { review: "range", from: other }, { review: "other" }, { branch: "" }]) {
    assert.equal(parseStoredTab(malformed), null, JSON.stringify(malformed))
  }
})

test("tab searches route documents by path and reviews without one", () => {
  assert.deepEqual(tabSearch("p", documentTab("a.md")), { project: "p", path: "a.md" })
  assert.deepEqual(tabSearch("p", reviewTab({ kind: "working" })), { project: "p", review: "working" })
  assert.deepEqual(tabSearch("p", reviewTab({ kind: "branch", base: "", head: "" })), { project: "p", review: "branch" })
  assert.deepEqual(tabSearch("p", reviewTab({ kind: "branch", base: "develop", head: "" })), { project: "p", review: "branch", base: "develop" })
  assert.deepEqual(tabSearch("p", reviewTab({ kind: "commit", hash })), { project: "p", commit: hash })
  assert.deepEqual(tabSearch("p", reviewTab({ kind: "range", from: other, to: hash })), { project: "p", from: other, to: hash })
})
