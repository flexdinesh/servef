import assert from "node:assert/strict"
import test from "node:test"

import { shareableOrigin } from "./shareable-origin.ts"

test("the desktop app's origin shares as a srcz link", () => {
  assert.equal(shareableOrigin("srcz-app://local"), "srcz://local")
  assert.equal(shareableOrigin("srcz-app://r-0123456789ab"), "srcz://r-0123456789ab")
})

test("other app origins have no shareable address", () => {
  for (const origin of ["srcz-app://other", "srcz-app://localx", "srcz-app://", "srcz-app://r-012", "srcz-app://r-0123456789AB", "srcz-app://r-0123456789ab:80"]) {
    assert.equal(shareableOrigin(origin), null, origin)
  }
})

test("origins outside the app are unchanged", () => {
  for (const origin of ["http://localhost:7971", "https://127.0.0.1:7972", "srcz://local", "null", "", "http://srcz-app://local"]) {
    assert.equal(shareableOrigin(origin), origin)
  }
})
