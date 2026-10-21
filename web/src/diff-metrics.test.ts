import assert from "node:assert/strict"
import test from "node:test"

import { DEFAULT_VIRTUAL_FILE_METRICS } from "@pierre/diffs"

import { diffMetrics } from "./diff-metrics.ts"

test("diff rows follow the Font Size setting", () => {
  assert.deepEqual(diffMetrics(1), { ...DEFAULT_VIRTUAL_FILE_METRICS, lineHeight: 22 })
  assert.equal(diffMetrics(1.5).lineHeight, 33)
})
