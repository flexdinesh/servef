import assert from "node:assert/strict"
import test from "node:test"

import { fileDecoration, viewedCSS } from "./files-tree-decoration.ts"

test("counted stats report additions and deletions, including zero", () => {
  assert.equal(fileDecoration({ path: "a.ts", status: "counted", additions: 1200, deletions: 0, blob: "b" }, undefined)?.text, "+1,200 −0")
  assert.equal(fileDecoration({ path: "a.ts", status: "counted", additions: 0, deletions: 3, blob: "b" }, undefined)?.text, "+0 −3")
})

test("uncounted files read as plain notes", () => {
  assert.equal(fileDecoration({ path: "logo.bin", status: "binary", blob: "" }, undefined)?.text, "binary")
  assert.equal(fileDecoration({ path: "big.txt", status: "tooLarge", blob: "" }, undefined)?.text, "large")
})

test("renamed files say where they came from until stats arrive", () => {
  assert.ok(fileDecoration(undefined, "old/start.md")?.text.includes("old/start.md"))
  assert.ok(fileDecoration({ path: "start.md", status: "counted", additions: 0, deletions: 0, blob: "b" }, "old/start.md")?.title.includes("old/start.md"))
})

test("files without stats or a rename have no decoration", () => {
  assert.equal(fileDecoration(undefined, undefined), null)
  assert.equal(fileDecoration({ path: "a.ts", status: "budget", blob: "" }, undefined), null)
})

test("files with uncommitted edits retain their mark with and without stats", () => {
  assert.equal(fileDecoration({ path: "a.ts", status: "counted", additions: 2, deletions: 0, blob: "b" }, undefined, { marked: true })?.text, "● +2 −0")
  assert.equal(fileDecoration(undefined, undefined, { marked: true })?.text, "●")
})

test("viewed files retain their status with and without stats", () => {
  for (const decoration of [
    fileDecoration({ path: "a.ts", status: "counted", additions: 2, deletions: 1, blob: "b" }, undefined, { viewed: true }),
    fileDecoration({ path: "logo.bin", status: "binary", blob: "b" }, undefined, { viewed: true }),
    fileDecoration(undefined, undefined, { viewed: true }),
  ]) assert.ok(decoration?.title.includes("Viewed"))
})

test("a marked, viewed file names both marks", () => {
  const title = fileDecoration({ path: "a.ts", status: "counted", additions: 1, deletions: 1, blob: "b" }, "old.ts", { marked: true, viewed: true })?.title
  for (const detail of ["uncommitted", "old.ts", "+1 −1", "Viewed"]) assert.ok(title?.includes(detail))
})

test("viewed CSS quotes hostile paths and excludes selected rows from dimming", () => {
  assert.equal(viewedCSS([]), "")
  const css = viewedCSS(['odd"name\\\n].md'])
  const selector = '[data-item-path="odd\\"name\\\\\\a ].md"]'
  assert.ok(css.includes(`${selector}) > [data-item-section="decoration"]::before`))
  assert.ok(css.includes(`${selector}:not([data-item-selected])`))
  assert.ok(!css.includes('odd"name'))
})
