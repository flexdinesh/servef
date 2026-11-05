import assert from "node:assert/strict"
import test from "node:test"

import type { LanguageServerStatus, NavigationStatus } from "@servef/contracts"
import { navigationStatusQuery, pollsServer } from "./navigation-status.ts"

const gopls: LanguageServerStatus = {
  name: "gopls",
  language: "Go",
  state: "stopped",
  message: "",
  install: [],
  docs: "",
  memory: 0,
  idleMinutes: 10,
  capabilities: null,
}

function status(overrides: Partial<LanguageServerStatus>, enabled = true): NavigationStatus {
  return { supported: true, enabled, eligible: true, reason: "", server: { ...gopls, ...overrides } }
}

test("status polls only while a server starts or runs", () => {
  assert.equal(pollsServer(null), false)
  assert.equal(pollsServer(status({ state: "ready" })), true)
  assert.equal(pollsServer(status({ state: "starting" })), true)
  assert.equal(pollsServer(status({ state: "stopped" })), false)
  assert.equal(pollsServer(status({ state: "ready" }, false)), false)
})

test("status queries ask for a refresh only on Check again", () => {
  assert.equal(navigationStatusQuery("/p", "a.go", false), "project=%2Fp&path=a.go")
  assert.equal(navigationStatusQuery("/p", "a.go", true), "project=%2Fp&path=a.go&refresh=1")
})
