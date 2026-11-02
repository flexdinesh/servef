import assert from "node:assert/strict"
import { chmodSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs"
import os from "node:os"
import path from "node:path"
import { after, test } from "node:test"
import { findOnPath, locateServef } from "../src/main/binary.ts"

const never = () => null
const onPath = (name: string) => `/usr/local/bin/${name}`

test("packaged uses only the bundled binary", () => {
  assert.deepEqual(
    locateServef({ packaged: true, resourcesPath: "/Applications/servef.app/Contents/Resources", env: { SERVEF_DESKTOP_BINARY: "/dev/servef" }, onPath }),
    { path: "/Applications/servef.app/Contents/Resources/bin/servef" },
  )
})

test("unpackaged prefers SERVEF_DESKTOP_BINARY, then PATH", () => {
  assert.deepEqual(locateServef({ packaged: false, resourcesPath: "/r", env: { SERVEF_DESKTOP_BINARY: "/dev/servef" }, onPath }), { path: "/dev/servef" })
  assert.deepEqual(locateServef({ packaged: false, resourcesPath: "/r", env: { SERVEF_DESKTOP_BINARY: "" }, onPath }), { path: "/usr/local/bin/servef" })
  assert.deepEqual(locateServef({ packaged: false, resourcesPath: "/r", env: {}, onPath }), { path: "/usr/local/bin/servef" })
})

test("unpackaged without override or PATH entry is missing", () => {
  const result = locateServef({ packaged: false, resourcesPath: "/r", env: {}, onPath: never })
  assert.ok("missing" in result)
  assert.match(result.missing, /SERVEF_DESKTOP_BINARY/)
})

const dir = mkdtempSync(path.join(os.tmpdir(), "servef-path-"))
after(() => rmSync(dir, { recursive: true, force: true }))

test("findOnPath returns the first executable regular file", () => {
  const plain = path.join(dir, "plain")
  const asDir = path.join(dir, "dir")
  const exec = path.join(dir, "exec")
  const later = path.join(dir, "later")
  for (const each of [plain, asDir, exec, later]) mkdirSync(each)
  writeFileSync(path.join(plain, "servef"), "not executable")
  mkdirSync(path.join(asDir, "servef"))
  writeFileSync(path.join(exec, "servef"), "#!/bin/sh\n")
  chmodSync(path.join(exec, "servef"), 0o755)
  writeFileSync(path.join(later, "servef"), "#!/bin/sh\n")
  chmodSync(path.join(later, "servef"), 0o755)
  const PATH = ["relative", "", plain, asDir, exec, later].join(path.delimiter)
  assert.equal(findOnPath("servef", { PATH }), path.join(exec, "servef"))
  assert.equal(findOnPath("servef", { PATH: [plain, asDir].join(path.delimiter) }), null)
  assert.equal(findOnPath("servef", {}), null)
})
