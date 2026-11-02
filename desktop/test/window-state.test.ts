import assert from "node:assert/strict"
import { test } from "node:test"
import { DEFAULT_BOUNDS, parseWindowState, serializeWindowState } from "../src/main/window-state.ts"

test("parseWindowState round-trips valid bounds", () => {
  const bounds = { x: -20, y: 40, width: 1440, height: 900 }
  assert.deepEqual(parseWindowState(serializeWindowState(bounds)), bounds)
  assert.deepEqual(parseWindowState(serializeWindowState({ width: 900, height: 700 })), { width: 900, height: 700 })
})

test("parseWindowState clamps sizes and drops non-integer positions", () => {
  assert.deepEqual(parseWindowState(JSON.stringify({ width: 10, height: 1e9, x: 1.5, y: "2" })), { width: 400, height: 10_000 })
  assert.deepEqual(parseWindowState(JSON.stringify({ width: 1000.4, height: 800.6 })), { width: 1000, height: 801 })
})

test("parseWindowState falls back to the default on invalid input", () => {
  for (const text of [null, "", "{", "null", "[]", "{\"width\":\"1\",\"height\":2}", "{\"width\":1}", "{\"width\":1e400,\"height\":800}"]) {
    assert.deepEqual(parseWindowState(text), DEFAULT_BOUNDS, String(text))
  }
})
