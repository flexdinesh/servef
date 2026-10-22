import assert from "node:assert/strict"
import test from "node:test"

import { gitStatusError, isChangesResponse, isDiffResponse } from "./changes-contract.ts"

const documentType = {
  id: "go", label: "Go", extensions: [".go"], filenames: [], language: "go",
  previewKind: "", previewLabel: "", defaultMode: "source", formattable: false,
}
const side = { content: "package main\n", size: 13, binary: false, tooLarge: false }

test("changes responses require known statuses and rename origins", () => {
  const changes = { repository: true, branch: "main", files: [{ path: "a.go", status: "modified" }], truncated: false, warnings: [] }
  assert.equal(isChangesResponse(changes), true)
  assert.equal(isChangesResponse({ ...changes, files: [{ path: "a.go", status: "copied" }] }), false)
  assert.equal(isChangesResponse({ ...changes, files: [{ path: "b.go", status: "renamed" }] }), false)
  assert.equal(isChangesResponse({ ...changes, files: [{ path: "b.go", oldPath: "a.go", status: "renamed" }] }), true)
  assert.equal(isChangesResponse({ ...changes, files: [{ path: "", status: "added" }] }), false)
  assert.equal(isChangesResponse({ ...changes, branch: null }), false)
  assert.equal(isChangesResponse({ ...changes, warnings: [1] }), false)
  const { truncated: _truncated, ...withoutTruncated } = changes
  assert.equal(isChangesResponse(withoutTruncated), false)
  // A folder has no repository and so no changes.
  assert.equal(isChangesResponse({ ...changes, repository: false }), false)
  assert.equal(isChangesResponse({ ...changes, repository: false, branch: "", files: [] }), true)
})

test("diff responses null exactly the side a status lacks", () => {
  const modified = { path: "a.go", status: "modified", documentType, original: side, modified: side }
  assert.equal(isDiffResponse(modified), true)
  assert.equal(isDiffResponse({ ...modified, status: "added", original: null }), true)
  assert.equal(isDiffResponse({ ...modified, status: "untracked", original: null }), true)
  assert.equal(isDiffResponse({ ...modified, status: "added" }), false)
  assert.equal(isDiffResponse({ ...modified, status: "deleted", modified: null }), true)
  assert.equal(isDiffResponse({ ...modified, status: "deleted" }), false)
  assert.equal(isDiffResponse({ ...modified, original: null }), false)
  assert.equal(isDiffResponse({ ...modified, documentType: { ...documentType, previewKind: "table" } }), false)
  assert.equal(isDiffResponse({ ...modified, modified: { ...side, size: -1 } }), false)
  assert.equal(isDiffResponse({ ...modified, modified: { size: 2, binary: false, tooLarge: false } }), true)
  // Binary and too-large sides never carry content.
  assert.equal(isDiffResponse({ ...modified, modified: { ...side, binary: true } }), false)
  assert.equal(isDiffResponse({ ...modified, modified: { ...side, tooLarge: true } }), false)
})

test("git status errors name the problem once", () => {
  assert.equal(gitStatusError("Couldn't read git status: index is corrupt"), "Couldn't read git status: index is corrupt.")
  assert.equal(gitStatusError("the server didn't respond"), "Couldn't read git status: the server didn't respond.")
  assert.equal(gitStatusError("Couldn't read git status: timed out."), "Couldn't read git status: timed out.")
})
