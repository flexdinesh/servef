import assert from "node:assert/strict"
import test from "node:test"

import { fileTypeCounts, filterFileTree } from "./file-type-filter.ts"
import type { TreeNode } from "./page-contract.ts"

function file(path: string, fileType: string): TreeNode {
  return { name: path.split("/").at(-1) ?? path, path, fileType, children: [], isDir: false, open: false, selected: false }
}

const tree: TreeNode[] = [
  { name: "config", path: "config", fileType: "", isDir: true, open: true, selected: false,
    children: [file("config/settings.json", "json"), file("config/settings.yaml", "yaml")] },
  { name: "docs", path: "docs", fileType: "", isDir: true, open: false, selected: false,
    children: [file("docs/readme.md", "markdown")] },
  file("package.json", "json"),
]

test("type visibility prunes empty folders without mutating document or expansion metadata", () => {
  const visible = filterFileTree(tree, ["json"])
  assert.deepEqual(visible.map((node) => node.path), ["config", "package.json"])
  assert.deepEqual(visible[0]?.children.map((node) => node.path), ["config/settings.json"])
  assert.equal(visible[0]?.open, true)
  assert.equal(tree[0]?.children.length, 2)
  assert.deepEqual(filterFileTree(tree, []), tree)
  assert.deepEqual(filterFileTree(tree, ["go"]), [])
})

test("multiple families intersect visibility and no selection shows all files", () => {
  const visible = filterFileTree(tree, ["json", "yaml"])
  assert.deepEqual(visible.map((node) => node.path), ["config", "package.json"])
  assert.deepEqual(visible[0]?.children.map((node) => node.path), ["config/settings.json", "config/settings.yaml"])
  assert.deepEqual(filterFileTree(tree, []), tree)
})

test("family counts aggregate files across directories", () => {
  assert.deepEqual([...fileTypeCounts(tree)], [["json", 2], ["yaml", 1], ["markdown", 1]])
})
