import assert from "node:assert/strict"
import test from "node:test"

import type { StackLayer, StackResponse } from "../../api/stack-contract.ts"
import type { Compare } from "./compare.ts"
import { fileParts, onBaseBranch, refChips, rowDiff, stackRows } from "./source-control-panes.ts"

const a = "a".repeat(40)
const b = "b".repeat(40)
const c = "c".repeat(40)

function layer(name: string, head: string, parent: string, current = false, aliases: string[] = []): StackLayer {
  return { name, aliases, head, parent, ahead: 1, current }
}

function stack(layers: StackLayer[]): StackResponse {
  return { repository: true, branch: "feat-b", base: "main", mergeBase: "", head: b, layers, truncated: false }
}

test("HEAD on the base branch, or tracking origin's copy, lists all history", () => {
  assert.equal(onBaseBranch("main", "main"), true)
  assert.equal(onBaseBranch("main", "origin/main"), true)
  assert.equal(onBaseBranch("feat", "main"), false)
  assert.equal(onBaseBranch("", "main"), false)
  assert.equal(onBaseBranch("main", ""), false)
})

test("Diff sits on a range's newest commit, and on any other commit", () => {
  const range: Compare = { kind: "range", from: c, to: a }
  assert.deepEqual(rowDiff(a, range, [a, b, c]), { compare: range, pinned: true })
  assert.equal(rowDiff(b, range, [a, b, c]), null)
  assert.deepEqual(rowDiff("d".repeat(40), range, [a, b, c]), { compare: { kind: "commit", hash: "d".repeat(40) }, pinned: false })
  assert.deepEqual(rowDiff(a, { kind: "commit", hash: a }, [a]), { compare: { kind: "commit", hash: a }, pinned: true })
  assert.deepEqual(rowDiff(a, null, []), { compare: { kind: "commit", hash: a }, pinned: false })
})

test("stack layers name their commits, current first", () => {
  const chips = refChips(stack([layer("feat-c", c, "feat-b"), layer("feat-b", b, "feat-a", true, ["feat-b2"]), layer("", a, "main", false)]))
  assert.deepEqual(chips.get(b), [{ name: "feat-b", current: true }, { name: "feat-b2", current: false }])
  assert.deepEqual(chips.get(c), [{ name: "feat-c", current: false }])
  assert.equal(chips.has(a), false)
  assert.equal(refChips(null).size, 0)
})

test("stack rows diff each layer against the one below, and everything against the base", () => {
  const rows = stackRows(stack([layer("feat-c", c, "feat-b"), layer("feat-b", b, "feat-a", true), layer("feat-a", a, "main")]), "")
  assert.deepEqual(rows.map((row) => row.layerDiff), [
    { kind: "branch", base: "feat-b", head: "feat-c" },
    { kind: "branch", base: "feat-a", head: "" },
    null,
  ])
  assert.deepEqual(rows.map((row) => row.baseDiff), [
    { kind: "branch", base: "", head: "feat-c" },
    { kind: "branch", base: "", head: "" },
    { kind: "branch", base: "", head: "feat-a" },
  ])
  assert.deepEqual(stackRows(null, ""), [])
})

test("flat change rows split names from folders", () => {
  assert.deepEqual(fileParts("web/src/App.tsx"), { name: "App.tsx", folder: "web/src" })
  assert.deepEqual(fileParts("README.md"), { name: "README.md", folder: "" })
})
