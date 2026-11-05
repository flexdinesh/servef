import assert from "node:assert/strict"
import { test } from "node:test"
import { systemTimers } from "../src/main/timers.ts"

test("systemTimers runs callbacks and clears handles", async () => {
  const before = systemTimers.now()
  let ran = 0
  const cleared = systemTimers.setTimeout(() => { ran += 10 }, 1)
  systemTimers.clearTimeout(cleared)
  await new Promise<void>((resolve) => systemTimers.setTimeout(() => { ran += 1; resolve() }, 5))
  assert.equal(ran, 1)
  assert.ok(systemTimers.now() >= before)
  // Clearing a fired or unknown handle is harmless.
  systemTimers.clearTimeout(cleared)
  systemTimers.clearTimeout({ id: -1 })
})
