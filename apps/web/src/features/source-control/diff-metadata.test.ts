import assert from "node:assert/strict"
import { readdir, readFile } from "node:fs/promises"
import test from "node:test"

import { parseDiffFromFile, type FileContents } from "@pierre/diffs"

import { isDiffResponse } from "@servef/contracts"
import { diffContent } from "./diff-content.ts"
import { fileDiffMetadata } from "./diff-metadata.ts"
import { lineDiffOf } from "./line-diff-testing.ts"

const fixtures = new URL("../../../../../testdata/api/default/", import.meta.url)

// plain drops undefined fields, as Pierre leaves some declared but unset.
function plain(value: unknown): unknown {
  const parsed: unknown = JSON.parse(JSON.stringify(value))
  return parsed
}

test("server line diffs lay out the metadata Pierre parses from the same sides", async () => {
  const names = (await readdir(fixtures)).filter((name) => name.startsWith("diff-") && name.endsWith(".json"))
  let compared = 0
  for (const name of names) {
    const response: unknown = JSON.parse(await readFile(new URL(name, fixtures), "utf8"))
    assert.ok(isDiffResponse(response), name)
    const content = diffContent(response)
    if (content.kind !== "text" || !response.lineDiff) continue
    assert.deepEqual(plain(fileDiffMetadata(content.oldFile, content.newFile, response.lineDiff)),
      plain(parseDiffFromFile(content.oldFile, content.newFile)), name)
    compared++
  }
  // Modified, added, deleted, renamed, untracked, and many hunks.
  assert.equal(compared, 6)
})

test("edge layouts match Pierre: empty sides, missing newlines, renames", () => {
  const file = (name: string, contents: string): FileContents => ({ name, contents, lang: "text" })
  const cases: [FileContents | null, FileContents | null][] = [
    [file("a.txt", ""), file("a.txt", "one\n")],
    [file("a.txt", "one\n"), file("a.txt", "")],
    [file("a.txt", "one\ntwo"), file("a.txt", "one\ntwo\n")],
    [file("a.txt", "one\ntwo"), file("a.txt", "one\nTWO")],
    [file("a.txt", "x\n".repeat(30)), file("a.txt", "x\n".repeat(12) + "y\n" + "x\n".repeat(17))],
    [file("old.txt", "same\n"), file("new.txt", "same\n")],
    [file("old.txt", "a\nb\n"), file("new.txt", "a\nc\n")],
    [null, file("a.txt", "")],
    [file("a.txt", "gone"), null],
  ]
  for (const [oldFile, newFile] of cases) {
    const lineDiff = lineDiffOf(oldFile?.contents ?? null, newFile?.contents ?? null)
    assert.deepEqual(plain(fileDiffMetadata(oldFile, newFile, lineDiff)), plain(parseDiffFromFile(oldFile, newFile)),
      JSON.stringify([oldFile, newFile]))
  }
})
