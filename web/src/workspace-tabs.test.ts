import assert from "node:assert/strict"
import test from "node:test"

import { diffTab, documentTab, includesTab, parseStoredTab, sameTab, storedTab, tabKey, tabName } from "./workspace-tabs.ts"

test("documents and diffs of one path are distinct tabs", () => {
  assert.equal(sameTab(documentTab("a.md"), documentTab("a.md")), true)
  assert.equal(sameTab(documentTab("a.md"), diffTab("a.md")), false)
  assert.equal(sameTab(documentTab("a.md"), null), false)
  assert.notEqual(tabKey(documentTab("a.md")), tabKey(diffTab("a.md")))
  assert.equal(includesTab([documentTab("a.md")], diffTab("a.md")), false)
})

test("diff tabs name the compared side and expose the full path", () => {
  assert.deepEqual(tabName(documentTab("web/src/App.tsx"), false), { label: "App.tsx", title: "web/src/App.tsx" })
  assert.deepEqual(tabName(diffTab("web/src/App.tsx"), false), { label: "App.tsx (Working Tree)", title: "web/src/App.tsx (Working Tree)" })
  assert.deepEqual(tabName(diffTab("old.md"), true), { label: "old.md (Deleted)", title: "old.md (Deleted)" })
})

test("stored tabs round-trip and accept legacy paths", () => {
  assert.equal(storedTab(documentTab("a.md")), "a.md")
  assert.deepEqual(storedTab(diffTab("a.md")), { diff: "a.md" })
  assert.deepEqual(parseStoredTab("a.md"), documentTab("a.md"))
  assert.deepEqual(parseStoredTab({ diff: "a.md" }), diffTab("a.md"))
  for (const malformed of ["", { diff: "" }, { diff: 1 }, { path: "a.md" }, ["a.md"], null, 3]) {
    assert.equal(parseStoredTab(malformed), null)
  }
})
