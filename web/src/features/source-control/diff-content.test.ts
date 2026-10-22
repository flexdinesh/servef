import assert from "node:assert/strict"
import test from "node:test"

import type { DiffResponse, DiffSide } from "../../changes-contract.ts"
import type { FileType } from "../../page-contract.ts"
import { binaryChangeLabel, diffContent, parseTextDiff } from "./diff-content.ts"
import { diffStats, diffStatsLabel } from "./diff-stats.ts"

const documentType: FileType = {
  id: "go", label: "Go", extensions: [".go"], filenames: [], language: "go",
  previewKind: "", previewLabel: "", defaultMode: "source", formattable: false,
}

function text(content: string): DiffSide {
  return { content, size: content.length, binary: false, tooLarge: false }
}

function response(original: DiffSide | null, modified: DiffSide | null, status: DiffResponse["status"] = "modified"): DiffResponse {
  return { path: "main.go", status, documentType, original, modified }
}

test("text diffs carry both sides with the server's language", () => {
  const content = diffContent({ ...response(text("a\n"), text("b\n"), "renamed"), oldPath: "old.go" })
  assert.equal(content.kind, "text")
  if (content.kind !== "text") return
  assert.deepEqual(content.oldFile, { name: "old.go", contents: "a\n", lang: "go" })
  assert.deepEqual(content.newFile, { name: "main.go", contents: "b\n", lang: "go" })
  const added = diffContent(response(null, text("b\n"), "added"))
  assert.equal(added.kind === "text" && added.oldFile, null)
})

test("binary and large sides turn off the diff", () => {
  assert.deepEqual(diffContent(response({ size: 10, binary: true, tooLarge: false }, text("x"))), { kind: "binary", before: 10, after: 1 })
  assert.deepEqual(diffContent(response(null, { size: 9, binary: true, tooLarge: false }, "added")), { kind: "binary", before: null, after: 9 })
  assert.deepEqual(diffContent(response(text("x"), { size: 5_000_000, binary: false, tooLarge: true })), { kind: "tooLarge", size: 5_000_000 })
})

test("binary changes name the sizes on each side", () => {
  assert.equal(binaryChangeLabel(12_288, 14_336), "Binary file changed · 12.0 KB → 14.0 KB")
  assert.equal(binaryChangeLabel(null, 512), "Binary file added · 512 B")
  assert.equal(binaryChangeLabel(512, null), "Binary file deleted · 512 B")
})

test("stats count changed lines and whitespace can be ignored", () => {
  const content = diffContent(response(text("one\ntwo\nthree\n"), text("one\n  two\nfour\nfive\n")))
  const parsed = parseTextDiff(content, false)
  assert.ok(parsed)
  assert.deepEqual(diffStats(parsed), { additions: 3, deletions: 2 })
  assert.equal(diffStatsLabel(diffStats(parsed)), "+3 −2")
  const trimmed = parseTextDiff(content, true)
  assert.ok(trimmed)
  assert.deepEqual(diffStats(trimmed), { additions: 2, deletions: 1 })
  const indented = parseTextDiff(diffContent(response(text("a\n"), text("  a\n"))), true)
  assert.deepEqual(indented?.hunks, [])
  assert.equal(parseTextDiff(diffContent(response({ size: 1, binary: true, tooLarge: false }, text("x"))), false), null)
})
