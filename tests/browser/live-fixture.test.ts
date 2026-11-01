import assert from "node:assert/strict"
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"
import test from "node:test"

import { createLiveFixture, longDocument } from "./live-fixture.ts"

test("live fixtures have independent mutable documents", () => {
  const directory = mkdtempSync(path.join(tmpdir(), "servef-live-test-"))
  try {
    const first = path.join(directory, "first")
    const second = path.join(directory, "second")
    createLiveFixture(first)
    createLiveFixture(second)
    writeFileSync(path.join(first, "long.md"), longDocument("changed"))
    assert.equal(readFileSync(path.join(second, "long.md"), "utf8"), longDocument("original"))
    assert.equal(readFileSync(path.join(first, "long.md"), "utf8"), longDocument("changed"))
  } finally {
    rmSync(directory, { recursive: true, force: true })
  }
})
