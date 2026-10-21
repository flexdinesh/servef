import assert from "node:assert/strict"
import test from "node:test"

import { parseSidebar } from "./use-sidebar.ts"

test("stored sidebar views include Source Control and fall back to the Explorer", () => {
  assert.deepEqual(parseSidebar({ view: "source-control", hidden: false }), { view: "source-control", hidden: false })
  assert.deepEqual(parseSidebar({ view: "search", hidden: true }), { view: "search", hidden: true })
  assert.deepEqual(parseSidebar({ view: "timeline", hidden: true }), { view: "explorer", hidden: true })
  assert.deepEqual(parseSidebar(null), { view: "explorer", hidden: false })
})
