import assert from "node:assert/strict"
import test from "node:test"

import { desktopBridge, readDesktopBridge, type NativeMenuItem } from "./desktop-bridge.ts"

test("a version 1 bridge opens folders through the exposed function", () => {
  const calls: unknown[] = []
  const exposed = { version: 1, openFolder(this: unknown) { calls.push(this) } }
  const bridge = readDesktopBridge(exposed)
  assert.equal(bridge?.version, 1)
  const open = bridge?.openFolder
  open?.()
  assert.deepEqual(calls, [exposed])
})

test("showAppMenu passes the point through when exposed and is null otherwise", () => {
  const calls: unknown[] = []
  const exposed = { version: 1, openFolder() {}, showAppMenu(this: unknown, x: number, y: number) { calls.push([this, x, y]) } }
  readDesktopBridge(exposed)?.showAppMenu?.(4, 32)
  assert.deepEqual(calls, [[exposed, 4, 32]])
  assert.equal(readDesktopBridge({ version: 1, openFolder() {} })?.showAppMenu, null)
  assert.equal(readDesktopBridge({ version: 1, openFolder() {}, showAppMenu: "menu" })?.showAppMenu, null)
})

test("closeWindow calls the native method when exposed and is null otherwise", () => {
  const calls: unknown[] = []
  const exposed = { version: 2, openFolder() {}, closeWindow(this: unknown) { calls.push(this) } }
  readDesktopBridge(exposed)?.closeWindow?.()
  assert.deepEqual(calls, [exposed])
  assert.equal(readDesktopBridge({ version: 2, openFolder() {} })?.closeWindow, null)
  assert.equal(readDesktopBridge({ version: 2, openFolder() {}, closeWindow: "close" })?.closeWindow, null)
})

test("a version 2 bridge reveals paths and resolves native menu choices", async () => {
  const calls: unknown[] = []
  const exposed = {
    version: 2,
    openFolder() {},
    revealPath(this: unknown, project: string, path: string) {
      calls.push([this, project, path])
      return Promise.resolve(true)
    },
    showContextMenu(this: unknown, items: unknown, x: number, y: number) {
      calls.push([this, items, x, y])
      return Promise.resolve("reveal")
    },
  }
  const bridge = readDesktopBridge(exposed)
  assert.equal(bridge?.version, 2)
  bridge?.revealPath?.("/src/a", "README.md")
  const items: NativeMenuItem[] = [{ id: "reveal", label: "Reveal in File Manager" }, { separator: true }]
  assert.equal(await bridge?.showContextMenu?.(items, 3, 4), "reveal")
  assert.deepEqual(calls, [[exposed, "/src/a", "README.md"], [exposed, items, 3, 4]])
})

test("native menu answers other than a string, and failures, are no choice", async () => {
  for (const answer of [null, undefined, 3, { id: "x" }]) {
    const bridge = readDesktopBridge({ version: 2, openFolder() {}, showContextMenu: () => Promise.resolve(answer) })
    assert.equal(await bridge?.showContextMenu?.([], 0, 0), null)
  }
  const failing = readDesktopBridge({ version: 2, openFolder() {}, showContextMenu: () => Promise.reject(new Error("closed")) })
  assert.equal(await failing?.showContextMenu?.([], 0, 0), null)
  const throwing = readDesktopBridge({ version: 2, openFolder() {}, revealPath: () => Promise.reject(new Error("refused")) })
  assert.doesNotThrow(() => throwing?.revealPath?.("/a", "b"))
})

test("each later method is feature-detected: a version 1 bridge or non-functions are null", () => {
  const v1 = readDesktopBridge({ version: 1, openFolder() {} })
  assert.equal(v1?.revealPath, null)
  assert.equal(v1?.showContextMenu, null)
  const odd = readDesktopBridge({ version: 2, openFolder() {}, revealPath: "reveal", showContextMenu: {} })
  assert.equal(odd?.revealPath, null)
  assert.equal(odd?.showContextMenu, null)
})

test("a remote window's bridge has its window's methods and nothing of this machine", () => {
  const remote = readDesktopBridge({ version: 2, closeWindow() {}, showAppMenu() {}, showContextMenu: () => Promise.resolve(null) })
  assert.notEqual(remote, null)
  assert.equal(remote?.openFolder, null)
  assert.equal(remote?.revealPath, null)
  assert.equal(typeof remote?.closeWindow, "function")
  assert.equal(typeof remote?.showAppMenu, "function")
  assert.equal(typeof remote?.showContextMenu, "function")
  assert.equal(readDesktopBridge({ version: 1, openFolder: "open" })?.openFolder, null)
})

test("invalid versions and non-objects are no bridge", () => {
  const openFolder = () => {}
  assert.equal(readDesktopBridge({ version: 0, openFolder }), null)
  assert.equal(readDesktopBridge({ version: 1.5, openFolder }), null)
  assert.equal(readDesktopBridge({ version: "1", openFolder }), null)
  assert.equal(readDesktopBridge({ openFolder }), null)
  for (const value of [undefined, null, 1, "bridge", [1, openFolder], openFolder]) assert.equal(readDesktopBridge(value), null)
})

test("without a window there is no bridge", () => {
  assert.equal(desktopBridge(), null)
})
