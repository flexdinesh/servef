import assert from "node:assert/strict"
import test from "node:test"

import { absolutePath, isTreeMenuAction, treeMenuItems, treeRowPath } from "./tree-menu.ts"

test("the row menu offers Reveal and path copies, each a known action", () => {
  const ids = treeMenuItems().flatMap((item) => "separator" in item ? [] : [item.id])
  assert.deepEqual(ids, ["reveal", "copy-path", "copy-relative-path"])
  for (const id of ids) assert.equal(isTreeMenuAction(id), true)
  for (const id of [null, "", "close"]) assert.equal(isTreeMenuAction(id), false)
})

test("row paths drop a directory's trailing slash and join the project", () => {
  assert.equal(treeRowPath("src/"), "src")
  assert.equal(treeRowPath("src/a.ts"), "src/a.ts")
  assert.equal(absolutePath("/home/me/alpha", "src/a.ts"), "/home/me/alpha/src/a.ts")
  assert.equal(absolutePath("/", "a.ts"), "/a.ts")
})
