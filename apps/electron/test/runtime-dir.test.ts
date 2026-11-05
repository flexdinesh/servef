import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readdirSync, rmSync, statSync, symlinkSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { test } from "node:test"
import { chooseRuntimeDir, prepareRuntimeDir, runtimePaths, sweepRuntimeDir, unlinkQuietly } from "../src/main/runtime-dir.ts"
import type { RuntimeDirInput } from "../src/main/runtime-dir.ts"

const userData = "/home/dee/.config/servef-desktop"
const h8 = createHash("sha256").update(userData).digest("hex").slice(0, 8)

function input(overrides: Partial<RuntimeDirInput>): RuntimeDirInput {
  return { platform: "linux", env: {}, tmpdir: "/tmp", userData, uid: 1000, ...overrides }
}

test("runtime paths hash the id", () => {
  const h12 = createHash("sha256").update("ssh-devbox").digest("hex").slice(0, 12)
  const paths = runtimePaths("/run/x")
  assert.equal(paths.controlPath("ssh-devbox"), `/run/x/c-${h12}`)
  assert.equal(paths.forwardSocket("ssh-devbox"), `/run/x/f-${h12}.sock`)
  const port = createHash("sha256").update("ssh-devbox-7971").digest("hex").slice(0, 12)
  assert.equal(paths.portSocket("ssh-devbox-7971"), `/run/x/p-${port}.sock`)
})

test("Linux uses an absolute XDG_RUNTIME_DIR, else tmpdir", () => {
  assert.equal(chooseRuntimeDir(input({ env: { XDG_RUNTIME_DIR: "/run/user/1000" } })), `/run/user/1000/servef-electron-${h8}`)
  assert.equal(chooseRuntimeDir(input({ env: { XDG_RUNTIME_DIR: "relative/dir" }, tmpdir: "/var/tmp" })), `/var/tmp/servef-electron-${h8}`)
  assert.equal(chooseRuntimeDir(input({ tmpdir: "/var/tmp/" })), `/var/tmp/servef-electron-${h8}`)
})

test("macOS uses TMPDIR and ignores XDG_RUNTIME_DIR", () => {
  assert.equal(chooseRuntimeDir(input({ platform: "darwin", env: { XDG_RUNTIME_DIR: "/run/user/501" }, tmpdir: "/var/folders/ab/T" })), `/var/folders/ab/T/servef-electron-${h8}`)
})

test("SERVEF_DESKTOP_RUNTIME_DIR overrides", () => {
  assert.equal(chooseRuntimeDir(input({ env: { SERVEF_DESKTOP_RUNTIME_DIR: "/tmp/test-run", XDG_RUNTIME_DIR: "/run/user/1000" } })), "/tmp/test-run")
})

test("falls back to /tmp/servef-electron-<uid>-<h8> when the ControlPath would not fit sun_path", () => {
  const fallback = `/tmp/servef-electron-501-${h8}`
  // macOS limit 103: dir + "/c-" + 12 hex + 17 must fit.
  const macBase = "/var/folders/zz/zyxvpxvq6csfxvn_n0000000000000/T"
  const macDir = `${macBase}/servef-electron-${h8}`
  assert.ok(macDir.length + 15 + 17 > 103)
  assert.equal(chooseRuntimeDir(input({ platform: "darwin", tmpdir: macBase, uid: 501 })), fallback)
  // Exactly at the Linux limit fits; one byte more falls back.
  const name = `/servef-electron-${h8}`
  const fitting = `/${"a".repeat(107 - 17 - 15 - name.length - 1)}`
  assert.equal(chooseRuntimeDir(input({ tmpdir: fitting, uid: 501 })), `${fitting}${name}`)
  assert.equal(chooseRuntimeDir(input({ tmpdir: `${fitting}b`, uid: 501 })), fallback)
})

test("falls back when the path contains % or :", () => {
  assert.equal(chooseRuntimeDir(input({ env: { XDG_RUNTIME_DIR: "/run/user/100%" } })), `/tmp/servef-electron-1000-${h8}`)
  assert.equal(chooseRuntimeDir(input({ tmpdir: "/tmp/a:b" })), `/tmp/servef-electron-1000-${h8}`)
})

function withDir(run: (dir: string) => void | Promise<void>): Promise<void> {
  const dir = mkdtempSync(join(tmpdir(), "servef-runtime-"))
  return Promise.resolve(run(dir)).finally(() => rmSync(dir, { recursive: true, force: true }))
}

test("prepareRuntimeDir creates the directory 0700", () => withDir((dir) => {
  const target = join(dir, "run")
  const result = prepareRuntimeDir(target)
  assert.ok(result.ok)
  assert.equal(result.runtime.dir, target)
  assert.equal(statSync(target).mode & 0o777, 0o700)
  // An existing private directory is accepted again.
  assert.ok(prepareRuntimeDir(target).ok)
}))

test("prepareRuntimeDir tightens group and other bits", () => withDir((dir) => {
  const target = join(dir, "run")
  mkdirSync(target)
  chmodSync(target, 0o755)
  assert.ok(prepareRuntimeDir(target).ok)
  assert.equal(statSync(target).mode & 0o777, 0o700)
}))

test("prepareRuntimeDir refuses a symlink, a file, and a missing parent", () => withDir((dir) => {
  const real = join(dir, "real")
  mkdirSync(real, { mode: 0o700 })
  const link = join(dir, "link")
  symlinkSync(real, link)
  const linked = prepareRuntimeDir(link)
  assert.deepEqual(linked, { ok: false, dir: link, detail: "it is a symbolic link" })
  const file = join(dir, "file")
  writeFileSync(file, "")
  assert.deepEqual(prepareRuntimeDir(file), { ok: false, dir: file, detail: "it is not a directory" })
  const orphan = prepareRuntimeDir(join(dir, "missing", "run"))
  assert.equal(orphan.ok, false)
  assert.equal(existsSync(join(dir, "missing")), false)
}))

test("sweepRuntimeDir exits and unlinks c-*, unlinks f-*.sock and p-*.sock, and leaves everything else", () => withDir(async (dir) => {
  for (const name of ["c-aaaaaaaaaaaa", "c-bbbbbbbbbbbb", "f-aaaaaaaaaaaa.sock", "f-x.sock", "p-aaaaaaaaaaaa.sock", "f-keep", "p-keep", "keep.sock", "connections.json", "xc-1"]) writeFileSync(join(dir, name), "")
  mkdirSync(join(dir, "c-directory"))
  mkdirSync(join(dir, "sub"))
  writeFileSync(join(dir, "sub", "c-nested"), "")
  const exited: string[] = []
  await sweepRuntimeDir(runtimePaths(dir), async (controlPath) => {
    exited.push(controlPath)
    if (controlPath.endsWith("bbbbbbbbbbbb")) throw new Error("master gone")
  })
  assert.deepEqual(exited.sort(), [join(dir, "c-aaaaaaaaaaaa"), join(dir, "c-bbbbbbbbbbbb")])
  assert.deepEqual(readdirSync(dir).sort(), ["c-directory", "connections.json", "f-keep", "keep.sock", "p-keep", "sub", "xc-1"])
  assert.deepEqual(readdirSync(join(dir, "sub")), ["c-nested"])
}))

test("sweepRuntimeDir on a missing directory does nothing", async () => {
  await sweepRuntimeDir(runtimePaths("/nonexistent/servef-runtime-test"), async () => {
    throw new Error("not called")
  })
})

test("unlinkQuietly ignores ENOENT only", () => withDir((dir) => {
  const file = join(dir, "f")
  writeFileSync(file, "")
  unlinkQuietly(file)
  assert.equal(existsSync(file), false)
  unlinkQuietly(file)
  mkdirSync(join(dir, "d"))
  assert.throws(() => unlinkQuietly(join(dir, "d")))
}))
