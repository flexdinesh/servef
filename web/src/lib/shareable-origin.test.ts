import assert from "node:assert/strict"
import test from "node:test"

import { shareableOrigin } from "./shareable-origin.ts"

test("desktop app origins share as servef links", () => {
  assert.equal(shareableOrigin("servef-app://local"), "servef://local")
  assert.equal(shareableOrigin("servef-app://other"), "servef://other")
})

test("other origins are unchanged", () => {
  for (const origin of ["http://localhost:7971", "https://127.0.0.1:7972", "servef://local", "null", "", "http://servef-app://local"]) {
    assert.equal(shareableOrigin(origin), origin)
  }
})
