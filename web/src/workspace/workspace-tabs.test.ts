import assert from "node:assert/strict"
import test from "node:test"

import type { CompareContext } from "../features/source-control/compare.ts"
import { documentKey, documentTab, includesTab, parseStoredTab, reviewTab, routeAt, sameTab, storedTab, tabKey, tabName, tabSearch, walkthroughTab } from "./workspace-tabs.ts"

const hash = "3577ec6a1b2c3d4e5f60718293a4b5c6d7e8f901"
const other = "4f76e9a1b2c3d4e5f60718293a4b5c6d7e8f9012"
const context: CompareContext = { base: "main", subject: (candidate) => candidate === hash ? "fix: preserve language server crash diagnostics" : "" }

test("documents and reviews are distinct tabs; one review per compare", () => {
  const working = reviewTab({ kind: "working" })
  assert.equal(sameTab(documentTab("a.md"), documentTab("a.md")), true)
  assert.equal(sameTab(documentTab("a.md"), working), false)
  assert.equal(sameTab(working, reviewTab({ kind: "working" })), true)
  assert.equal(sameTab(reviewTab({ kind: "commit", hash }), reviewTab({ kind: "commit", hash: other })), false)
  assert.equal(includesTab([working], reviewTab({ kind: "branch", base: "", head: "", uncommitted: false })), false)
  const keys = [documentTab("working"), working, reviewTab({ kind: "branch", base: "", head: "", uncommitted: false }), reviewTab({ kind: "branch", base: "develop", head: "", uncommitted: false }),
    reviewTab({ kind: "commit", hash }), reviewTab({ kind: "range", from: other, to: hash })].map(tabKey)
  assert.equal(new Set(keys).size, keys.length)
})

test("tabs name documents by file and reviews by what they compare", () => {
  assert.deepEqual(tabName(documentTab("web/src/App.tsx"), context), { label: "App.tsx", title: "web/src/App.tsx" })
  assert.equal(tabName(reviewTab({ kind: "working" }), context).label, "Uncommitted")
  assert.equal(tabName(reviewTab({ kind: "branch", base: "", head: "", uncommitted: false }), context).label, "vs main")
  assert.equal(tabName(reviewTab({ kind: "branch", base: "origin/develop", head: "", uncommitted: false }), context).label, "vs origin/develop")
  assert.equal(tabName(reviewTab({ kind: "branch", base: "", head: "", uncommitted: false }), { ...context, base: "" }).label, "Branch")
  assert.equal(tabName(reviewTab({ kind: "commit", hash }), context).label, "3577ec6 fix: preserve language server crash dia…")
  assert.equal(tabName(reviewTab({ kind: "commit", hash: other }), context).label, "4f76e9a")
  assert.equal(tabName(reviewTab({ kind: "range", from: other, to: hash }), context).label, "4f76e9a..3577ec6")
})

test("stored tabs round-trip and migrate per-file diff entries to their review", () => {
  for (const tab of [documentTab("a.md"), reviewTab({ kind: "working" }), reviewTab({ kind: "branch", base: "develop", head: "", uncommitted: false }),
    reviewTab({ kind: "commit", hash }), reviewTab({ kind: "range", from: other, to: hash })]) {
    assert.deepEqual(parseStoredTab(storedTab(tab)), tab)
  }
  assert.deepEqual(parseStoredTab({ diff: "a.md" }), reviewTab({ kind: "working" }))
  assert.deepEqual(parseStoredTab({ commit: hash, path: "a.md" }), reviewTab({ kind: "commit", hash }))
  assert.deepEqual(parseStoredTab({ branch: "a.md" }), reviewTab({ kind: "branch", base: "", head: "", uncommitted: false }))
  assert.deepEqual(parseStoredTab({ from: other, to: hash, path: "a.md" }), reviewTab({ kind: "range", from: other, to: hash }))
  for (const malformed of ["", { diff: "" }, { diff: 1 }, { path: "a.md" }, ["a.md"], null, 3, { commit: "3577ec6" },
    { review: "commit", commit: "x" }, { review: "range", from: other }, { review: "other" }, { branch: "" }]) {
    assert.equal(parseStoredTab(malformed), null, JSON.stringify(malformed))
  }
})

test("tab searches route documents by path and reviews without one", () => {
  assert.deepEqual(tabSearch("p", documentTab("a.md")), { project: "p", path: "a.md" })
  assert.deepEqual(tabSearch("p", reviewTab({ kind: "working" })), { project: "p", review: "working" })
  assert.deepEqual(tabSearch("p", reviewTab({ kind: "branch", base: "", head: "", uncommitted: false })), { project: "p", review: "branch" })
  assert.deepEqual(tabSearch("p", reviewTab({ kind: "branch", base: "develop", head: "", uncommitted: false })), { project: "p", review: "branch", base: "develop" })
  assert.deepEqual(tabSearch("p", reviewTab({ kind: "commit", hash })), { project: "p", commit: hash })
  assert.deepEqual(tabSearch("p", reviewTab({ kind: "range", from: other, to: hash })), { project: "p", from: other, to: hash })
})

test("snapshots are document tabs of a commit, distinct from the working file", () => {
  const snapshot = documentTab("web/src/App.tsx", hash)
  assert.equal(sameTab(snapshot, documentTab("web/src/App.tsx")), false)
  assert.equal(sameTab(snapshot, documentTab("web/src/App.tsx", other)), false)
  assert.equal(tabKey(snapshot), `document:${hash}:web/src/App.tsx`)
  assert.equal(tabKey(documentTab("web/src/App.tsx")), "document:web/src/App.tsx")
  assert.deepEqual(tabName(snapshot, context), { label: "App.tsx @ 3577ec6", title: `web/src/App.tsx at ${hash}` })
  assert.deepEqual(tabSearch("p", snapshot), { project: "p", path: "web/src/App.tsx", at: hash })
  assert.equal(documentKey("a.md"), "a.md")
  assert.equal(documentKey("a.md", hash), `${hash}:a.md`)
})

test("stored snapshots round-trip; plain paths stay working-tree documents", () => {
  const snapshot = documentTab("a.md", hash)
  assert.deepEqual(storedTab(snapshot), { path: "a.md", at: hash })
  assert.deepEqual(parseStoredTab(storedTab(snapshot)), snapshot)
  assert.deepEqual(parseStoredTab("a.md"), documentTab("a.md"))
  for (const malformed of [{ path: "a.md", at: "3577ec6" }, { path: "", at: hash }, { path: 1, at: hash }, { at: hash }, { path: "a.md", at: hash.toUpperCase() }]) {
    assert.equal(parseStoredTab(malformed), null, JSON.stringify(malformed))
  }
})

test("routes read a snapshot commit only as a full hash with a path", () => {
  assert.equal(routeAt("a.md", hash), hash)
  assert.equal(routeAt(undefined, hash), undefined)
  assert.equal(routeAt("a.md", "3577ec6"), undefined)
  assert.equal(routeAt("a.md", hash.toUpperCase()), undefined)
  assert.equal(routeAt("a.md", 1), undefined)
})

test("walkthrough tabs key, name, store, and route by ID", () => {
  const tab = walkthroughTab("tour")
  assert.equal(tabKey(tab), "walkthrough:tour")
  assert.equal(sameTab(tab, walkthroughTab("tour")), true)
  assert.equal(sameTab(tab, documentTab("tour")), false)
  assert.deepEqual(tabName(tab, context), { label: "Walkthrough", title: "Walkthrough tour" })
  assert.deepEqual(tabName(tab, context, (id) => id === "tour" ? "How login works" : ""), { label: "How login works", title: "Walkthrough: How login works" })
  assert.deepEqual(storedTab(tab), { walkthrough: "tour" })
  assert.deepEqual(parseStoredTab({ walkthrough: "tour" }), tab)
  assert.equal(parseStoredTab({ walkthrough: "" }), null)
  assert.equal(parseStoredTab({ walkthrough: 3 }), null)
  assert.deepEqual(tabSearch("/p", tab), { project: "/p", wt: "tour" })
})
