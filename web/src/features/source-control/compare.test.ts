import assert from "node:assert/strict"
import test from "node:test"

import type { Commit } from "../../api/commits-contract.ts"
import {
  branchCompare, commitAge, compareKey, compareName, compareSearch, isLiveCompare, legacyCompare, parseReviewFragment, parseStoredCompare,
  rangeCount, reviewFragment, reviewScope, routeCompare, sameCompare, selectedHashes, selectionCompare, storedCompare, type Compare,
} from "./compare.ts"

const first = "1111111111111111111111111111111111111111"
const second = "2222222222222222222222222222222222222222"
const third = "3333333333333333333333333333333333333333"

function commit(hash: string): Commit {
  return { hash, parents: [], subject: hash.slice(0, 1), body: "", author: { name: "a", email: "a@b" }, authorTime: "2026-10-21T14:02:00+00:00" }
}

// list is newest first, as git log lists it.
const list = [commit(first), commit(second), commit(third)]

const compares: Compare[] = [
  { kind: "working" }, { kind: "branch", base: "", head: "", uncommitted: false }, { kind: "branch", base: "origin/main", head: "", uncommitted: false },
  { kind: "branch", base: "feat-a", head: "feat-b", uncommitted: false }, { kind: "branch", base: "", head: "feat-b", uncommitted: false },
  { kind: "commit", hash: first }, { kind: "range", from: third, to: first },
  { kind: "branch", base: "", head: "", uncommitted: true }, { kind: "branch", base: "origin/main", head: "", uncommitted: true },
]

test("compares have distinct keys; only the working tree and branches are live", () => {
  assert.equal(new Set(compares.map(compareKey)).size, compares.length)
  assert.equal(sameCompare({ kind: "commit", hash: first }, { kind: "commit", hash: first }), true)
  assert.equal(sameCompare({ kind: "working" }, null), false)
  assert.deepEqual(compares.map(isLiveCompare), [true, true, true, true, true, false, false, true, true])
})

test("stack layer compares name their head and base", () => {
  const context = { base: "main", subject: () => "" }
  assert.equal(compareName({ kind: "branch", base: "feat-a", head: "feat-b", uncommitted: false }, context).label, "feat-b vs feat-a")
  assert.equal(compareName({ kind: "branch", base: "", head: "feat-b", uncommitted: false }, context).label, "feat-b vs main")
  assert.equal(compareName({ kind: "branch", base: "", head: "", uncommitted: false }, context).label, "vs main")
  assert.deepEqual(compareSearch({ kind: "branch", base: "feat-a", head: "feat-b", uncommitted: false }), { review: "branch", base: "feat-a", head: "feat-b" })
  assert.deepEqual(parseStoredCompare({ review: "branch", base: "main" }), { kind: "branch", base: "main", head: "", uncommitted: false })
})

test("HEAD's branch review can include the working tree", () => {
  const context = { base: "main", subject: () => "" }
  const working = branchCompare("", "", true)
  assert.deepEqual(working, { kind: "branch", base: "", head: "", uncommitted: true })
  // A stack layer's worktree is not checked out.
  assert.deepEqual(branchCompare("feat-a", "feat-b", true), { kind: "branch", base: "feat-a", head: "feat-b", uncommitted: false })
  assert.notEqual(compareKey(working), compareKey(branchCompare("")))
  assert.equal(isLiveCompare(working), true)
  assert.deepEqual(compareName(working, context), {
    label: "vs main + uncommitted", title: "Changes on this branch since it left main, with uncommitted changes",
  })
  assert.equal(compareName(working, { ...context, base: "" }).label, "Branch + uncommitted")
  assert.deepEqual(compareSearch(branchCompare("develop", "", true)), { review: "branch", base: "develop", uncommitted: 1 })
  assert.deepEqual(routeCompare({ review: "branch", uncommitted: "1" }), working)
  assert.deepEqual(routeCompare({ review: "branch", head: "feat-b", uncommitted: "1" }), branchCompare("", "feat-b"))
  assert.deepEqual(routeCompare({ review: "branch", uncommitted: "true" }), branchCompare(""))
  assert.deepEqual(storedCompare(working), { review: "branch", base: "", uncommitted: true })
  assert.deepEqual(parseStoredCompare({ review: "branch", base: "", uncommitted: "yes" }), branchCompare(""))
  assert.equal(reviewScope(working), true)
  assert.equal(reviewScope(branchCompare("develop")), false)
  assert.equal(reviewScope(branchCompare("feat-a", "feat-b")), null)
  assert.equal(reviewScope({ kind: "working" }), null)
  assert.equal(reviewScope({ kind: "commit", hash: first }), null)
})

test("compares round-trip through routes and memory", () => {
  for (const compare of compares) {
    assert.deepEqual(routeCompare(compareSearch(compare)), compare)
    assert.deepEqual(parseStoredCompare(storedCompare(compare)), compare)
  }
  // A path makes a route a document, not a review.
  assert.equal(routeCompare({ path: "a.md", commit: first }), null)
  assert.equal(routeCompare({ commit: "1111111" }), null)
  assert.equal(routeCompare({ from: first }), null)
})

test("per-file diff routes from before reviews open their review at the file", () => {
  assert.deepEqual(legacyCompare({ path: "a.md", diff: 1 }), { compare: { kind: "working" }, path: "a.md" })
  assert.deepEqual(legacyCompare({ path: "a.md", diff: "1" }), { compare: { kind: "working" }, path: "a.md" })
  assert.deepEqual(legacyCompare({ path: "a.md", commit: first }), { compare: { kind: "commit", hash: first }, path: "a.md" })
  assert.deepEqual(legacyCompare({ path: "a.md", branch: 1 }), { compare: { kind: "branch", base: "", head: "", uncommitted: false }, path: "a.md" })
  assert.deepEqual(legacyCompare({ path: "a.md", from: third, to: first }), { compare: { kind: "range", from: third, to: first }, path: "a.md" })
  assert.equal(legacyCompare({ path: "a.md" }), null)
  assert.equal(legacyCompare({ commit: first }), null)
})

test("review fragments name a file and its selected lines", () => {
  assert.equal(reviewFragment("web/src/App.tsx"), "f=web/src/App.tsx")
  assert.equal(reviewFragment("a b&c.md", { start: 12, end: 20, side: "additions" }), "f=a%20b%26c.md&R12-R20")
  assert.deepEqual(parseReviewFragment("#f=a%20b%26c.md&R12-R20"), {
    path: "a b&c.md", lines: { start: 12, end: 20, side: "additions", endSide: "additions" },
  })
  assert.deepEqual(parseReviewFragment("f=web/src/App.tsx"), { path: "web/src/App.tsx", lines: null })
  assert.deepEqual(parseReviewFragment("f=a.md&nonsense"), { path: "a.md", lines: null })
  for (const invalid of ["", "L12", "f=", "f=%E0%A4%A"]) assert.equal(parseReviewFragment(invalid), null, invalid)
})

test("selecting commits from an anchor spans a contiguous run in either direction", () => {
  assert.deepEqual(selectionCompare(list, first, first), { kind: "commit", hash: first })
  assert.deepEqual(selectionCompare(list, first, third), { kind: "range", from: third, to: first })
  assert.deepEqual(selectionCompare(list, third, second), { kind: "range", from: third, to: second })
  assert.equal(selectionCompare(list, first, "f".repeat(40)), null)
  assert.deepEqual(selectedHashes(list, { kind: "range", from: third, to: first }), [first, second, third])
  assert.deepEqual(selectedHashes(list, { kind: "commit", hash: second }), [second])
  assert.deepEqual(selectedHashes(list, { kind: "range", from: first, to: third }), [])
  assert.deepEqual(selectedHashes(list, { kind: "working" }), [])
  assert.equal(rangeCount(list, { kind: "range", from: second, to: first }), 2)
  assert.equal(rangeCount([commit(first)], { kind: "range", from: third, to: first }), 0)
})

test("commit ages read short and long", () => {
  const now = Date.parse("2026-10-21T17:02:00+00:00")
  assert.deepEqual(commitAge("2026-10-21T14:02:00+00:00", now), { short: "3h", long: "3 hours ago" })
  assert.deepEqual(commitAge("2026-10-21T17:03:00+00:00", now), { short: "now", long: "just now" })
})
