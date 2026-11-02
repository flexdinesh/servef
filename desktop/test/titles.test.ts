import assert from "node:assert/strict"
import { test } from "node:test"
import { windowTitle } from "../src/main/titles.ts"
import type { TitleConnection } from "../src/main/titles.ts"

test("local windows keep the page title", () => {
  assert.equal(windowTitle("README.md - servef", { kind: "local", label: "local", ready: true }), "README.md - servef")
  assert.equal(windowTitle("servef", { kind: "local", label: "", ready: false }), "servef")
})

test("ssh windows name the destination and show reconnecting while not ready", () => {
  assert.equal(windowTitle("README.md", { kind: "ssh", label: "devbox", ready: true }), "README.md - devbox (SSH)")
  assert.equal(windowTitle("README.md", { kind: "ssh", label: "dee@10.0.0.5", ready: false }), "README.md - dee@10.0.0.5 (SSH, reconnecting)")
})

test("CLI server windows name localhost:<port>", () => {
  assert.equal(windowTitle("README.md", { kind: "cli", label: "localhost:7971", ready: true }), "README.md - localhost:7971")
})

test("http windows name host:port; forwarded remote servers add (SSH)", () => {
  assert.equal(windowTitle("README.md", { kind: "http", label: "10.0.0.5:7971", ready: true }), "README.md - 10.0.0.5:7971")
  assert.equal(windowTitle("README.md", { kind: "ssh-port", label: "devbox:7971", ready: true }), "README.md - devbox:7971 (SSH)")
})

test("app pages keep their own titles", () => {
  assert.equal(windowTitle("servef - Connect", { kind: "app", label: "", ready: true }), "servef - Connect")
  assert.equal(windowTitle("servef - devbox", { kind: "app", label: "", ready: true }), "servef - devbox")
})

test("titles never use an em dash", () => {
  const connections: TitleConnection[] = [{ kind: "local", label: "", ready: true }, { kind: "app", label: "", ready: true }, { kind: "ssh", label: "box", ready: false }, { kind: "cli", label: "localhost:1", ready: true }, { kind: "http", label: "h:1", ready: true }, { kind: "ssh-port", label: "box:1", ready: true }]
  for (const connection of connections) {
    assert.ok(!windowTitle("page", connection).includes(String.fromCodePoint(0x2014)))
  }
})
