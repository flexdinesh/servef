import assert from "node:assert/strict"
import test from "node:test"

import { gitStatusError, historyError } from "./source-control-messages.ts"

test("git status errors name the problem once", () => {
  assert.equal(gitStatusError("Couldn't read git status: index is corrupt"), "Couldn't read git status: index is corrupt.")
  assert.equal(gitStatusError("the server didn't respond"), "Couldn't read git status: the server didn't respond.")
  assert.equal(gitStatusError("Couldn't read git status: timed out."), "Couldn't read git status: timed out.")
})

test("git history errors name the problem once", () => {
  assert.equal(historyError("Couldn't read git history: object not found"), "Couldn't read git history: object not found.")
  assert.equal(historyError("the server didn't respond"), "Couldn't read git history: the server didn't respond.")
  assert.equal(historyError("Couldn't read git history: timed out!"), "Couldn't read git history: timed out!")
})
