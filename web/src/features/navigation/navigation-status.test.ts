import assert from "node:assert/strict"
import test from "node:test"

import type { LanguageServerStatus, NavigationStatus } from "../../navigation-contract.ts"
import { navigationStatusQuery, pollsServer, providerLabel, serverSummary } from "./navigation-status.ts"

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

test("status distinguishes unavailable, missing, disabled, installed and runtime states", () => {
  assert.equal(providerLabel(null), "LSP unknown")
  assert.equal(providerLabel(status({ state: "ready" })), "LSP ready")
  assert.equal(providerLabel(status({ state: "starting" })), "LSP starting…")
  assert.equal(providerLabel(status({ state: "missing" })), "LSP missing")
  assert.equal(providerLabel(status({ state: "stopped" })), "LSP available")
  assert.equal(providerLabel(status({ state: "failed" })), "LSP failed")
  assert.equal(providerLabel(status({ state: "ready" }, false)), "LSP off")
  assert.equal(providerLabel({ supported: false, enabled: true, eligible: false, reason: "", server: null }), "LSP unavailable")
})

test("server summaries explain each state", () => {
  assert.equal(serverSummary({ ...gopls, state: "missing" }, true), "gopls is not installed.")
  assert.equal(serverSummary(gopls, true), "gopls starts on the next navigation.")
  assert.equal(serverSummary(gopls, false), "gopls is installed.")
  assert.equal(serverSummary({ ...gopls, state: "ready", memory: 512 * 1_048_576 }, true), "gopls is running, using 512.0 MB.")
  assert.equal(serverSummary({ ...gopls, state: "failed", message: "exit status 2" }, true), "gopls failed: exit status 2")
})

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
