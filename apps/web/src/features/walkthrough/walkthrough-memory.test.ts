import assert from "node:assert/strict"
import test from "node:test"

import { emptyWalkthroughMemory, parseWalkthroughMemory, rememberSeen, rememberStep, unseenCount } from "./walkthrough-memory.ts"

test("memory parses defensively", () => {
  assert.deepEqual(parseWalkthroughMemory(null), emptyWalkthroughMemory)
  assert.deepEqual(parseWalkthroughMemory({ seen: { "/p": "2026-10-26T00:00:00Z", "/q": 3 }, steps: { "/p": { tour: 2, bad: -1, worse: "x" } } }), {
    seen: { "/p": "2026-10-26T00:00:00Z" },
    steps: { "/p": { tour: 2 } },
  })
})

test("remembering puts the latest first and bounds the lists", () => {
  let memory = emptyWalkthroughMemory
  for (let index = 0; index < 120; index++) memory = rememberStep(memory, "/p", `w${index}`, index)
  const ids = Object.keys(memory.steps["/p"] ?? {})
  assert.equal(ids.length, 100)
  assert.equal(ids[0], "w119")
  memory = rememberStep(memory, "/p", "w50", 7)
  assert.equal(Object.keys(memory.steps["/p"] ?? {})[0], "w50")
  assert.equal(memory.steps["/p"]?.w50, 7)
  const seen = rememberSeen(memory, "/p", "t")
  assert.equal(seen.seen["/p"], "t")
  assert.equal(rememberSeen(seen, "/p", "t"), seen)
})

test("unseen counts walkthroughs newer than the last seen", () => {
  const times = ["2026-10-26T10:00:00Z", "2026-10-26T09:00:00Z", "2026-10-25T09:00:00Z"]
  assert.equal(unseenCount(times, undefined), 3)
  assert.equal(unseenCount(times, "2026-10-26T09:00:00Z"), 1)
  assert.equal(unseenCount(times, "2026-10-26T10:00:00Z"), 0)
})
