import assert from "node:assert/strict"
import test from "node:test"

import { boundedCache } from "./bounded-cache.ts"

test("a bounded cache drops the least recently used entry past its size", () => {
  const cache = boundedCache<number>(2)
  cache.set("a", 1)
  cache.set("b", 2)
  assert.equal(cache.get("a"), 1)
  cache.set("c", 3)
  assert.equal(cache.has("b"), false)
  assert.equal(cache.get("a"), 1)
  assert.equal(cache.get("c"), 3)
  assert.equal(cache.size, 2)
  cache.set("a", 4)
  cache.set("d", 5)
  assert.equal(cache.has("c"), false)
  assert.equal(cache.get("a"), 4)
  cache.delete("a")
  assert.equal(cache.size, 1)
  cache.clear()
  assert.equal(cache.size, 0)
})
