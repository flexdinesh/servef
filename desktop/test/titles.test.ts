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

test("app pages read servef - Connect whatever the page title", () => {
  assert.equal(windowTitle("anything", { kind: "app", label: "", ready: true }), "servef - Connect")
})

test("titles never use an em dash", () => {
  const connections: TitleConnection[] = [{ kind: "local", label: "", ready: true }, { kind: "app", label: "", ready: true }, { kind: "ssh", label: "box", ready: false }]
  for (const connection of connections) {
    assert.ok(!windowTitle("page", connection).includes(String.fromCodePoint(0x2014)))
  }
})
