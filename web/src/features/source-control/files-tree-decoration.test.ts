import assert from "node:assert/strict"
import test from "node:test"

import { fileDecoration } from "./files-tree-decoration.ts"

test("counted stats color additions and deletions, and zero keeps the row's ink", () => {
  assert.deepEqual(fileDecoration({ path: "a.ts", status: "counted", additions: 1200, deletions: 0, blob: "b" }, undefined), {
    text: "+1,200 −0",
    title: "+1,200 −0 lines",
    parts: [{ text: "+1,200", color: "var(--color-success)" }, { text: "\u00a0" }, { text: "−0" }],
  })
  assert.deepEqual(fileDecoration({ path: "a.ts", status: "counted", additions: 0, deletions: 3, blob: "b" }, undefined)?.parts, [
    { text: "+0" }, { text: "\u00a0" }, { text: "−3", color: "var(--color-destructive)" },
  ])
})

test("uncounted files read as plain notes", () => {
  assert.deepEqual(fileDecoration({ path: "logo.bin", status: "binary", blob: "" }, undefined), { text: "binary", title: "binary lines" })
  assert.deepEqual(fileDecoration({ path: "big.txt", status: "tooLarge", blob: "" }, undefined), { text: "large", title: "large lines" })
})

test("renamed files say where they came from until stats arrive", () => {
  assert.deepEqual(fileDecoration(undefined, "old/start.md"), { text: "from old/start.md", title: "Renamed from old/start.md" })
  assert.equal(
    fileDecoration({ path: "start.md", status: "counted", additions: 0, deletions: 0, blob: "b" }, "old/start.md")?.title,
    "Renamed from old/start.md, +0 −0 lines",
  )
})

test("files without stats or a rename have no decoration", () => {
  assert.equal(fileDecoration(undefined, undefined), null)
  assert.equal(fileDecoration({ path: "a.ts", status: "budget", blob: "" }, undefined), null)
})

test("files with uncommitted edits lead with an accent dot", () => {
  assert.deepEqual(fileDecoration({ path: "a.ts", status: "counted", additions: 2, deletions: 0, blob: "b" }, undefined, true), {
    text: "● +2 −0",
    title: "Has uncommitted changes, +2 −0 lines",
    parts: [
      { text: "●", color: "var(--color-accent)" },
      { text: "\u00a0" },
      { text: "+2", color: "var(--color-success)" },
      { text: "\u00a0" },
      { text: "−0" },
    ],
  })
  assert.deepEqual(fileDecoration(undefined, undefined, true), {
    text: "●",
    title: "Has uncommitted changes",
    parts: [{ text: "●", color: "var(--color-accent)" }],
  })
})
