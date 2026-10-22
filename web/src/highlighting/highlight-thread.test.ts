import assert from "node:assert/strict"
import test from "node:test"

import { highlightsOnMainThread, MAIN_THREAD_MAX_CHARACTERS } from "./highlight-thread.ts"

test("text up to the limit highlights on the main thread, larger text in a worker", () => {
  assert.equal(highlightsOnMainThread(0), true)
  assert.equal(highlightsOnMainThread(MAIN_THREAD_MAX_CHARACTERS), true)
  assert.equal(highlightsOnMainThread(MAIN_THREAD_MAX_CHARACTERS + 1), false)
  assert.equal(highlightsOnMainThread(4 << 20), false)
})
