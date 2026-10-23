import assert from "node:assert/strict"
import test from "node:test"

import {
  branchTab, commitTab, diffTab, documentTab, includesTab, isCommitHash, parseStoredTab, rangeTab, routeDiffTab, sameTab, shortHash,
  storedTab, tabKey, tabName,
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

test("branch and range diffs are distinct from other diffs of one path", () => {
  assert.equal(sameTab(branchTab("a.md"), branchTab("a.md")), true)
  assert.equal(sameTab(branchTab("a.md"), diffTab("a.md")), false)
  assert.equal(sameTab(rangeTab(other, hash, "a.md"), rangeTab(other, hash, "a.md")), true)
  assert.equal(sameTab(rangeTab(other, hash, "a.md"), rangeTab(hash, other, "a.md")), false)
  const keys = [branchTab("a.md"), rangeTab(other, hash, "a.md"), commitTab(hash, "a.md"), diffTab("a.md")].map(tabKey)
  assert.equal(new Set(keys).size, keys.length)
})

test("branch tabs name their base once known; range tabs their span", () => {
  assert.deepEqual(tabName(branchTab("web/src/App.tsx"), false, "main"), { label: "App.tsx (vs main)", title: "web/src/App.tsx (vs main)" })
  assert.deepEqual(tabName(branchTab("web/src/App.tsx"), false), { label: "App.tsx (Branch)", title: "web/src/App.tsx (Branch)" })
  assert.deepEqual(tabName(rangeTab(other, hash, "web/src/App.tsx"), false), {
    label: "App.tsx (4f76e9a..3577ec6)", title: "web/src/App.tsx (changes in commits 4f76e9a through 3577ec6)",
  })
})

test("branch and range tabs round-trip through memory", () => {
  assert.deepEqual(storedTab(branchTab("a.md")), { branch: "a.md" })
  assert.deepEqual(storedTab(rangeTab(other, hash, "a.md")), { from: other, to: hash, path: "a.md" })
  assert.deepEqual(parseStoredTab({ branch: "a.md" }), branchTab("a.md"))
  assert.deepEqual(parseStoredTab({ from: other, to: hash, path: "a.md" }), rangeTab(other, hash, "a.md"))
  for (const malformed of [{ branch: "" }, { branch: 1 }, { from: other, path: "a.md" }, { from: other, to: "3577ec6", path: "a.md" },
    { from: other, to: hash }, { from: other, to: hash, path: "" }]) {
    assert.equal(parseStoredTab(malformed), null, JSON.stringify(malformed))
  }
})

test("routes open the diff tab their search names", () => {
  assert.deepEqual(routeDiffTab({ path: "a.md", diff: 1 }), diffTab("a.md"))
  assert.deepEqual(routeDiffTab({ path: "a.md", commit: hash }), commitTab(hash, "a.md"))
  assert.deepEqual(routeDiffTab({ path: "a.md", branch: 1 }), branchTab("a.md"))
  assert.deepEqual(routeDiffTab({ path: "a.md", from: other, to: hash }), rangeTab(other, hash, "a.md"))
  assert.equal(routeDiffTab({ path: "a.md" }), null)
  assert.equal(routeDiffTab({ path: "a.md", from: other }), null)
  assert.equal(routeDiffTab({ branch: 1 }), null)
})
