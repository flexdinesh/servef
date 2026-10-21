import assert from "node:assert/strict"
import test from "node:test"

import {
  commitTab, diffTab, documentTab, includesTab, isCommitHash, parseStoredTab, sameTab, shortHash, storedTab, tabKey, tabName,
} from "./workspace-tabs.ts"

const hash = "3577ec6a1b2c3d4e5f60718293a4b5c6d7e8f901"
const other = "4f76e9a1b2c3d4e5f60718293a4b5c6d7e8f9012"

test("documents and diffs of one path are distinct tabs", () => {
  assert.equal(sameTab(documentTab("a.md"), documentTab("a.md")), true)
  assert.equal(sameTab(documentTab("a.md"), diffTab("a.md")), false)
  assert.equal(sameTab(documentTab("a.md"), null), false)
  assert.notEqual(tabKey(documentTab("a.md")), tabKey(diffTab("a.md")))
  assert.equal(includesTab([documentTab("a.md")], diffTab("a.md")), false)
})

test("commit diffs of one path are distinct per commit", () => {
  assert.equal(sameTab(commitTab(hash, "a.md"), commitTab(hash, "a.md")), true)
  assert.equal(sameTab(commitTab(hash, "a.md"), commitTab(other, "a.md")), false)
  assert.equal(sameTab(commitTab(hash, "a.md"), diffTab("a.md")), false)
  assert.notEqual(tabKey(commitTab(hash, "a.md")), tabKey(commitTab(other, "a.md")))
  assert.notEqual(tabKey(commitTab(hash, "a.md")), tabKey(diffTab("a.md")))
})

test("diff tabs name the compared side and expose the full path", () => {
  assert.deepEqual(tabName(documentTab("web/src/App.tsx"), false), { label: "App.tsx", title: "web/src/App.tsx" })
  assert.deepEqual(tabName(diffTab("web/src/App.tsx"), false), { label: "App.tsx (Working Tree)", title: "web/src/App.tsx (Working Tree)" })
  assert.deepEqual(tabName(diffTab("old.md"), true), { label: "old.md (Deleted)", title: "old.md (Deleted)" })
  assert.deepEqual(tabName(commitTab(hash, "internal/lsp/manager.go"), false), { label: "manager.go (3577ec6)", title: "internal/lsp/manager.go (3577ec6)" })
  assert.deepEqual(tabName(commitTab(hash, "gone.md"), true), { label: "gone.md (3577ec6)", title: "gone.md (3577ec6)" })
})

test("commit hashes are full lowercase SHA-1", () => {
  assert.equal(isCommitHash(hash), true)
  assert.equal(shortHash(hash), "3577ec6")
  for (const invalid of ["3577ec6", hash.toUpperCase(), `${hash}0`, "g".repeat(40), 3, null]) {
    assert.equal(isCommitHash(invalid), false, String(invalid))
  }
})

test("stored tabs round-trip and accept legacy paths", () => {
  assert.equal(storedTab(documentTab("a.md")), "a.md")
  assert.deepEqual(storedTab(diffTab("a.md")), { diff: "a.md" })
  assert.deepEqual(storedTab(commitTab(hash, "a.md")), { commit: hash, path: "a.md" })
  assert.deepEqual(parseStoredTab("a.md"), documentTab("a.md"))
  assert.deepEqual(parseStoredTab({ diff: "a.md" }), diffTab("a.md"))
  assert.deepEqual(parseStoredTab({ commit: hash, path: "a.md" }), commitTab(hash, "a.md"))
  for (const malformed of [
    "", { diff: "" }, { diff: 1 }, { path: "a.md" }, ["a.md"], null, 3,
    { commit: hash }, { commit: hash, path: "" }, { commit: "3577ec6", path: "a.md" }, { commit: hash, diff: "a.md" },
  ]) {
    assert.equal(parseStoredTab(malformed), null, JSON.stringify(malformed))
  }
})
