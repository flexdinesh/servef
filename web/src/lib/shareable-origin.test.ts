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

test("CLI server origins share as their localhost address", () => {
  assert.equal(shareableOrigin("servef-app://cli-7971"), "http://localhost:7971")
  assert.equal(shareableOrigin("servef-app://cli-65535"), "http://localhost:65535")
})

test("other origins are unchanged", () => {
  for (const origin of [
    "http://localhost:7971", "https://127.0.0.1:7972", "servef://local", "null", "", "http://servef-app://local",
    "servef-app://app", "servef-app://other", "servef-app://ssh-", "servef-app://ssh-Bad_Host",
    "servef-app://cli-0", "servef-app://cli-07971", "servef-app://cli-65536", "servef-app://cli-",
  ]) {
    assert.equal(shareableOrigin(origin), origin)
  }
})
