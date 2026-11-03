import assert from "node:assert/strict"
import test from "node:test"

import { isTabMenuAction, revealablePath, tabMenuItems } from "./tab-menu.ts"
import { documentTab, welcomeTab } from "./workspace-tabs.ts"

const HASH = "0123456789abcdef0123456789abcdef01234567"

function ids(items: ReturnType<typeof tabMenuItems>): string[] {
  return items.map((item) => "separator" in item ? "-" : `${item.id}${item.enabled === false ? " (disabled)" : ""}`)
}

test("the native tab menu mirrors the page menu's enabled states", () => {
  const tab = documentTab("README.md")
  assert.deepEqual(ids(tabMenuItems(tab, 0, 1, false)), ["close", "close-others (disabled)", "close-right (disabled)", "-", "close-all"])
  assert.deepEqual(ids(tabMenuItems(tab, 0, 3, false)), ["close", "close-others", "close-right", "-", "close-all"])
  assert.deepEqual(ids(tabMenuItems(tab, 2, 3, false)), ["close", "close-others", "close-right (disabled)", "-", "close-all"])
})

test("Reveal shows only when offered and for working-tree documents", () => {
  assert.deepEqual(ids(tabMenuItems(documentTab("src/a.ts"), 0, 1, true)).slice(-2), ["-", "reveal"])
  assert.equal(ids(tabMenuItems(documentTab("src/a.ts", HASH), 0, 1, true)).includes("reveal"), false)
  assert.equal(ids(tabMenuItems(welcomeTab(), 0, 1, true)).includes("reveal"), false)
  assert.equal(revealablePath(documentTab("src/a.ts")), "src/a.ts")
  assert.equal(revealablePath(documentTab("src/a.ts", HASH)), null)
})

test("isTabMenuAction accepts the menu's ids only", () => {
  for (const id of ["close", "close-others", "close-right", "close-all", "reveal"]) assert.equal(isTabMenuAction(id), true)
  for (const id of [null, "", "open", "Close"]) assert.equal(isTabMenuAction(id), false)
})
