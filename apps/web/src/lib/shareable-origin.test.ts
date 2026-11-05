import assert from "node:assert/strict"
import test from "node:test"

import { shareableOrigin } from "./shareable-origin.ts"

test("the desktop app's origin shares as a servef link", () => {
  assert.equal(shareableOrigin("servef-app://local"), "servef://local")
})

test("other app origins have no shareable address", () => {
  for (const origin of ["servef-app://other", "servef-app://localx", "servef-app://"]) {
    assert.equal(shareableOrigin(origin), null, origin)
  }
})

test("origins outside the app are unchanged", () => {
  for (const origin of ["http://localhost:7971", "https://127.0.0.1:7972", "servef://local", "null", "", "http://servef-app://local"]) {
    assert.equal(shareableOrigin(origin), origin)
  }
})
