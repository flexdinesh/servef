import assert from "node:assert/strict"
import test from "node:test"

import { desktopBridge, readDesktopBridge } from "./desktop-bridge.ts"

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

test("other versions, missing functions, and non-objects are no bridge", () => {
  const openFolder = () => {}
  assert.equal(readDesktopBridge({ version: 2, openFolder }), null)
  assert.equal(readDesktopBridge({ version: "1", openFolder }), null)
  assert.equal(readDesktopBridge({ openFolder }), null)
  assert.equal(readDesktopBridge({ version: 1 }), null)
  assert.equal(readDesktopBridge({ version: 1, openFolder: "open" }), null)
  for (const value of [undefined, null, 1, "bridge", [1, openFolder], openFolder]) assert.equal(readDesktopBridge(value), null)
})

test("without a window there is no bridge", () => {
  assert.equal(desktopBridge(), null)
})
