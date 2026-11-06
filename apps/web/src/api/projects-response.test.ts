import assert from "node:assert/strict"
import { readFile } from "node:fs/promises"
import test from "node:test"

import { isProjectsResponse } from "@srcz/contracts"

import { isRecord } from "./http.ts"

async function projectsFixture(): Promise<Record<string, unknown>> {
  const value: unknown = JSON.parse(await readFile(new URL("../../../../testdata/api/default/projects.json", import.meta.url), "utf8"))
  assert.ok(isRecord(value))
  return value
}

test("the projects fixture carries the CLI session key, its served roots joined", async () => {
  const value = await projectsFixture()
  assert.ok(isProjectsResponse(value))
  assert.equal(value.sessionKey, value.roots.map((root) => root.path).join("\n"))
})

test("projects responses require a string session key", async () => {
  const { sessionKey: _sessionKey, ...withoutKey } = await projectsFixture()
  assert.equal(isProjectsResponse(withoutKey), false)
  assert.equal(isProjectsResponse({ ...withoutKey, sessionKey: null }), false)
  assert.equal(isProjectsResponse({ ...withoutKey, sessionKey: 1 }), false)
  assert.equal(isProjectsResponse({ ...withoutKey, sessionKey: "daemon" }), true)
})
