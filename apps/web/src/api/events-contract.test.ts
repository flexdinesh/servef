import assert from "node:assert/strict"
import test from "node:test"

import { isLiveHead } from "./events-contract.ts"

const commit = "0123456789abcdef0123456789abcdef01234567"

test("head events carry the checkout and the stream's previous head", () => {
  const baseline = { branch: "main", commit, parent: "", subject: "Initial commit", previous: null }
  assert.equal(isLiveHead(baseline), true)
  assert.equal(isLiveHead({ ...baseline, parent: commit, previous: { branch: "", commit } }), true)
  // Detached and unborn heads are empty strings, never omitted or null.
  assert.equal(isLiveHead({ branch: "", commit, parent: "", subject: "", previous: null }), true)
  assert.equal(isLiveHead({ branch: "main", commit: "", parent: "", subject: "", previous: null }), true)
  assert.equal(isLiveHead({ ...baseline, branch: null }), false)
  assert.equal(isLiveHead({ ...baseline, commit: 1 }), false)
  const { parent: _parent, ...withoutParent } = baseline
  assert.equal(isLiveHead(withoutParent), false)
  const { subject: _subject, ...withoutSubject } = baseline
  assert.equal(isLiveHead(withoutSubject), false)
  const { previous: _previous, ...withoutPrevious } = baseline
  assert.equal(isLiveHead(withoutPrevious), false)
  assert.equal(isLiveHead({ ...baseline, previous: { branch: "main" } }), false)
  assert.equal(isLiveHead(null), false)
})
