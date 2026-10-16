import assert from "node:assert/strict"
import test from "node:test"

import type { LanguageServerStatus, NavigationStatus } from "./navigation-contract.ts"
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
}

function status(overrides: Partial<LanguageServerStatus>, provider: NavigationStatus["provider"] = "language-server", languageServers = true): NavigationStatus {
  return { navigable: true, languageServers, provider, server: { ...gopls, ...overrides } }
}

test("the status bar names the server only while it answers", () => {
  assert.equal(providerLabel(status({ state: "ready" })), "gopls")
  assert.equal(providerLabel(status({ state: "starting" })), "gopls…")
  assert.equal(providerLabel(status({ state: "missing" }, "syntax")), "Syntax")
  assert.equal(providerLabel({ navigable: true, languageServers: true, provider: "syntax", server: null }), "Syntax")
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
  assert.equal(pollsServer(status({ state: "ready" }, "language-server", false)), false)
})

test("status queries ask for a refresh only on Check again", () => {
  assert.equal(navigationStatusQuery("/p", "a.go", false), "project=%2Fp&path=a.go")
  assert.equal(navigationStatusQuery("/p", "a.go", true), "project=%2Fp&path=a.go&refresh=1")
})
