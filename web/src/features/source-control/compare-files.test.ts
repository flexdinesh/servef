import assert from "node:assert/strict"
import test from "node:test"

import { ancestorDirectories, compareFileList, displayOrder, type FileListSources } from "./compare-files.ts"

const sources: FileListSources = {
  working: null,
  workingError: "",
  branch: (base, head) => base === "" && head === "" ? { kind: "ready", files: [], truncated: false } : undefined,
  commit: () => ({ kind: "error", message: "Couldn't read git history: gone." }),
  range: () => undefined,
}

test("a compare's list is loading until read, then ready or failed", () => {
  assert.deepEqual(compareFileList({ kind: "working" }, sources), { kind: "loading" })
  assert.deepEqual(compareFileList({ kind: "working" }, { ...sources, workingError: "Couldn't read git status." }),
    { kind: "error", message: "Couldn't read git status." })
  assert.deepEqual(compareFileList({ kind: "working" }, { ...sources, working: { files: [{ path: "a", status: "added" }], truncated: true } }),
    { kind: "ready", files: [{ path: "a", status: "added" }], truncated: true })
  assert.deepEqual(compareFileList({ kind: "branch", base: "", head: "" }, sources), { kind: "ready", files: [], truncated: false })
  assert.deepEqual(compareFileList({ kind: "branch", base: "develop", head: "" }, sources), { kind: "loading" })
  assert.deepEqual(compareFileList({ kind: "branch", base: "", head: "feat-b" }, sources), { kind: "loading" })
  assert.equal(compareFileList({ kind: "commit", hash: "x" }, sources).kind, "error")
  assert.deepEqual(compareFileList({ kind: "range", from: "x", to: "y" }, sources), { kind: "loading" })
})

test("reviews list files in the tree's order: folders first, then files, by name", () => {
  const files = ["z.md", "src/b.ts", "src/a/c.ts", "a.md", "src/a.ts", "file10.md", "file2.md"].map((path) => ({ path, status: "modified" as const }))
  assert.deepEqual(displayOrder(files).map((file) => file.path), ["src/a/c.ts", "src/a.ts", "src/b.ts", "a.md", "file2.md", "file10.md", "z.md"])
  assert.deepEqual(ancestorDirectories(["src/a/c.ts", "a.md"]), ["src/", "src/a/"])
})
