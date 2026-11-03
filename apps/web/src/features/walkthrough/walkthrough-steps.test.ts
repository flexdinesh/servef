import assert from "node:assert/strict"
import test from "node:test"

import type { WalkthroughStep } from "../../api/walkthrough-contract.ts"
import type { CompareContext } from "../source-control/compare.ts"
import { coverRoute, lineDrift, parseStep, routeStep, routeWalkthrough, stepAfter, stepLabel, stepRoute, stepTarget } from "./walkthrough-steps.ts"

const hash = "3577ec6f1d2e4b8a9c0d7e6f5a4b3c2d1e0f9a8b"
const from = "4f76e9a2b3c4d5e6f708192a3b4c5d6e7f8091a2"
const context: CompareContext = { base: "main", subject: () => "" }

const note: WalkthroughStep = { title: "Intro", bodyHTML: "" }
const file: WalkthroughStep = { title: "Entry", bodyHTML: "", file: { path: "cmd/root.go", lines: [12, 20], expect: "func run(" } }
const snapshot: WalkthroughStep = { title: "Before", bodyHTML: "", file: { path: "a.go", lines: [3, 3], at: hash } }
const branch: WalkthroughStep = { title: "Change", bodyHTML: "", diff: { compare: { kind: "branch", base: "main" }, path: "web/a b.ts", side: "old", lines: [4, 9] } }
const range: WalkthroughStep = { title: "Range", bodyHTML: "", diff: { compare: { kind: "range", from, to: hash } } }

test("steps route to Source, snapshots, reviews, and the walkthrough tab", () => {
  assert.deepEqual(stepRoute("/p", "tour", 0, note), { search: { project: "/p", wt: "tour", step: 1 }, hash: "" })
  assert.deepEqual(stepRoute("/p", "tour", 1, file), { search: { project: "/p", path: "cmd/root.go", wt: "tour", step: 2 }, hash: "L12-L20" })
  assert.deepEqual(stepRoute("/p", "tour", 2, snapshot), { search: { project: "/p", path: "a.go", at: hash, wt: "tour", step: 3 }, hash: "L3" })
  assert.deepEqual(stepRoute("/p", "tour", 3, branch), {
    search: { project: "/p", review: "branch", base: "main", wt: "tour", step: 4 },
    hash: "f=web/a%20b.ts&L4-L9",
  })
  assert.deepEqual(stepRoute("/p", "tour", 4, range), { search: { project: "/p", from, to: hash, wt: "tour", step: 5 }, hash: "" })
  assert.deepEqual(coverRoute("/p", "tour"), { search: { project: "/p", wt: "tour" }, hash: "" })
})

test("a step whose compare cannot open reads as a note", () => {
  const broken: WalkthroughStep = { title: "x", bodyHTML: "", diff: { compare: { kind: "commit", commit: "abc" }, path: "a" } }
  assert.deepEqual(stepTarget(broken), { kind: "note" })
  const lineless: WalkthroughStep = { title: "x", bodyHTML: "", diff: { compare: { kind: "working" }, lines: [1, 2] } }
  assert.deepEqual(stepTarget(lineless), { kind: "diff", compare: { kind: "working" }, path: "", side: "additions", lines: null, expect: "" })
})

test("labels name each target", () => {
  assert.equal(stepLabel(note, context), "Note")
  assert.equal(stepLabel(file, context), "cmd/root.go:12–20")
  assert.equal(stepLabel(snapshot, context), "a.go:3 @ 3577ec6")
  assert.equal(stepLabel(branch, context), "vs main · web/a b.ts:4–9 (old)")
  assert.equal(stepLabel(range, context), "4f76e9a..3577ec6")
  assert.equal(stepLabel({ title: "x", bodyHTML: "", file: { path: "a.md" } }, context), "a.md")
})

test("drift compares the first line with the expected prefix", () => {
  assert.equal(lineDrift("func run(", "\tfunc run(ctx context.Context) {"), "unchanged")
  assert.equal(lineDrift("func run(", "func start() {"), "changed")
  assert.equal(lineDrift("func run(", undefined), "changed")
  assert.equal(lineDrift("", "anything"), "unknown")
})

test("stepping moves between the cover and steps and stops at the ends", () => {
  assert.equal(stepAfter(null, 3, 1), 0)
  assert.equal(stepAfter(null, 3, -1), undefined)
  assert.equal(stepAfter(0, 3, -1), null)
  assert.equal(stepAfter(1, 3, 1), 2)
  assert.equal(stepAfter(2, 3, 1), undefined)
  assert.equal(stepAfter(null, 0, 1), undefined)
})

test("route parameters parse strictly", () => {
  assert.equal(parseStep(2, 3), 1)
  assert.equal(parseStep("3", 3), 2)
  assert.equal(parseStep(4, 3), null)
  assert.equal(parseStep(undefined, 3), null)
  assert.equal(routeStep("2"), 2)
  assert.equal(routeStep(0), undefined)
  assert.equal(routeStep("1.5"), undefined)
  assert.equal(routeWalkthrough("tour-1"), "tour-1")
  assert.equal(routeWalkthrough("../x"), undefined)
  assert.equal(routeWalkthrough(""), undefined)
})
