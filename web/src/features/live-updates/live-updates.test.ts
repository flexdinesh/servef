import assert from "node:assert/strict"
import test from "node:test"

import type { LiveChange } from "../../api/events-contract.ts"
import { everything, liveIndicator, liveTargets } from "./live-updates.ts"

const change = (fields: Partial<LiveChange>): LiveChange => ({ paths: [], tree: false, git: false, overflow: false, ...fields })

test("a write to the open document rereads its page, changes, and search", () => {
  assert.deepEqual(liveTargets(change({ paths: ["docs/guide.md"] }), "docs/guide.md"),
    { page: true, changes: true, history: false, search: true })
})

test("a write to another file leaves the page alone", () => {
  assert.deepEqual(liveTargets(change({ paths: ["README.md"] }), "docs/guide.md"),
    { page: false, changes: true, history: false, search: true })
})

test("created or removed entries reread the tree without a document open", () => {
  assert.deepEqual(liveTargets(change({ paths: ["docs/new.md"], tree: true }), ""),
    { page: true, changes: true, history: false, search: true })
})

test("git state rereads changes and history only", () => {
  assert.deepEqual(liveTargets(change({ git: true }), "docs/guide.md"),
    { page: false, changes: true, history: true, search: false })
})

test("overflow and returning reread everything", () => {
  const all = { page: true, changes: true, history: true, search: true }
  assert.deepEqual(liveTargets(change({ overflow: true }), ""), all)
  assert.deepEqual(liveTargets(everything, ""), all)
})

test("the indicator names each mode and explains polling and off", () => {
  assert.deepEqual(liveIndicator({ mode: "watching", reason: "" }).label, "Live")
  const polling = liveIndicator({ mode: "polling", reason: "5000 directories exceed the 4096-directory watch limit." })
  assert.equal(polling.label, "Polling")
  assert.match(polling.description, /4096-directory watch limit/)
  const off = liveIndicator({ mode: "off", reason: "Live updates already follow 8 projects." })
  assert.equal(off.label, "Off")
  assert.match(off.description, /rereads when you return/)
  assert.match(off.description, /8 projects/)
})
