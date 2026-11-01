import assert from "node:assert/strict"
import test from "node:test"

import { shareableOrigin } from "./shareable-origin.ts"

test("the local desktop origin shares as a servef link", () => {
  assert.equal(shareableOrigin("servef-app://local"), "servef://local")
})

test("ssh desktop origins share as servef ssh links by slug", () => {
  assert.equal(shareableOrigin("servef-app://ssh-devbox"), "servef://ssh/devbox")
  assert.equal(shareableOrigin("servef-app://ssh-work-box-1a2b"), "servef://ssh/work-box-1a2b")
})

test("other origins are unchanged", () => {
  for (const origin of [
    "http://localhost:7971", "https://127.0.0.1:7972", "servef://local", "null", "", "http://servef-app://local",
    "servef-app://app", "servef-app://other", "servef-app://ssh-", "servef-app://ssh-Bad_Host",
  ]) {
    assert.equal(shareableOrigin(origin), origin)
  }
})
