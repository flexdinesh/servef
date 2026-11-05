// Private local runtime directory for ControlPaths and forward sockets (decision 0070).
import { createHash } from "node:crypto"
import { chmodSync, lstatSync, mkdirSync, readdirSync, unlinkSync, type Stats } from "node:fs"
import { posix } from "node:path"
import { isRecord } from "../../../web/src/api/http.ts"

export interface RuntimeDir {
  dir: string
  // controlPath is <dir>/c-<h12>; forwardSocket is <dir>/f-<h12>.sock;
  // portSocket is <dir>/p-<h12>.sock (an ssh-port id, decision 0076);
  // <h12> = 12 hex of sha256(id).
  controlPath(id: string): string
  forwardSocket(id: string): string
  portSocket(id: string): string
}

export type RuntimeDirResult = { ok: true; runtime: RuntimeDir } | { ok: false; dir: string; detail: string }

export interface RuntimeDirInput {
  platform: NodeJS.Platform
  env: NodeJS.ProcessEnv
  tmpdir: string
  userData: string
  uid: number
}

function hexDigest(text: string, length: number): string {
  return createHash("sha256").update(text).digest("hex").slice(0, length)
}

// runtimePaths names the files inside dir (no filesystem access).
export function runtimePaths(dir: string): RuntimeDir {
  return {
    dir,
    controlPath: (id) => posix.join(dir, `c-${hexDigest(id, 12)}`),
    forwardSocket: (id) => posix.join(dir, `f-${hexDigest(id, 12)}.sock`),
    portSocket: (id) => posix.join(dir, `p-${hexDigest(id, 12)}.sock`),
  }
}

// CONTROL_TEMP_BYTES: ssh binds `<ControlPath>.<16 random chars>` first.
const CONTROL_TEMP_BYTES = 17

// sunPathLimit is the usable sun_path length (bytes, without the NUL).
function sunPathLimit(platform: NodeJS.Platform): number {
  return platform === "darwin" ? 103 : 107
}

function fits(dir: string, platform: NodeJS.Platform): boolean {
  const paths = runtimePaths(dir)
  const limit = sunPathLimit(platform)
  return Buffer.byteLength(paths.controlPath("x"), "utf8") + CONTROL_TEMP_BYTES <= limit
    && Buffer.byteLength(paths.forwardSocket("x"), "utf8") <= limit
}

// chooseRuntimeDir picks the directory (plan "Local runtime files"):
// SERVEF_DESKTOP_RUNTIME_DIR override; Linux $XDG_RUNTIME_DIR/servef-electron-<h8>
// (absolute only) else <tmpdir>/servef-electron-<h8>; macOS $TMPDIR (tmpdir);
// /tmp/servef-electron-<uid>-<h8> when a ControlPath plus 17 bytes would not
// fit sun_path (Linux 107, macOS 103) or the path contains "%" or ":".
export function chooseRuntimeDir(input: RuntimeDirInput): string {
  const override = input.env.SERVEF_DESKTOP_RUNTIME_DIR
  if (override !== undefined && override !== "") return override
  const h8 = hexDigest(input.userData, 8)
  const name = `servef-electron-${h8}`
  const xdg = input.env.XDG_RUNTIME_DIR
  const base = input.platform === "linux" && xdg !== undefined && posix.isAbsolute(xdg) ? xdg : input.tmpdir
  const dir = posix.join(base, name)
  if (posix.isAbsolute(dir) && !dir.includes("%") && !dir.includes(":") && fits(dir, input.platform)) return dir
  return `/tmp/servef-electron-${input.uid}-${h8}`
}

function errorCode(error: unknown): string | undefined {
  if (!isRecord(error)) return undefined
  return typeof error.code === "string" ? error.code : undefined
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

// prepareRuntimeDir creates dir 0700 or verifies it with lstat: a symlink or
// another owner is refused; group/other bits are tightened. Only dir itself
// is created; its parent must exist. A refusal's detail is a sentence about
// dir (for example "it is a symbolic link").
export function prepareRuntimeDir(dir: string): RuntimeDirResult {
  try {
    mkdirSync(dir, { mode: 0o700 })
  } catch (error) {
    if (errorCode(error) !== "EEXIST") return { ok: false, dir, detail: `it cannot be created: ${errorMessage(error)}` }
  }
  let stats: Stats
  try {
    stats = lstatSync(dir)
  } catch (error) {
    return { ok: false, dir, detail: `it cannot be read: ${errorMessage(error)}` }
  }
  if (stats.isSymbolicLink()) return { ok: false, dir, detail: "it is a symbolic link" }
  if (!stats.isDirectory()) return { ok: false, dir, detail: "it is not a directory" }
  const uid = process.getuid?.()
  if (uid !== undefined && stats.uid !== uid) return { ok: false, dir, detail: `it is owned by another user (uid ${stats.uid})` }
  if ((stats.mode & 0o077) !== 0) {
    try {
      chmodSync(dir, 0o700)
    } catch (error) {
      return { ok: false, dir, detail: `its permissions cannot be restricted: ${errorMessage(error)}` }
    }
  }
  return { ok: true, runtime: runtimePaths(dir) }
}

// sweepRuntimeDir runs exit(controlPath) for each c-* entry (bounded by the
// caller), then unlinks it, and unlinks every f-*.sock and p-*.sock. Only those patterns,
// only in runtime.dir; directories and other names are never touched.
export async function sweepRuntimeDir(runtime: RuntimeDir, exit: (controlPath: string) => Promise<void>): Promise<void> {
  let names: string[]
  try {
    names = readdirSync(runtime.dir)
  } catch {
    return
  }
  const controls = names.filter((name) => name.startsWith("c-") && name.length > 2)
  const forwards = names.filter((name) => (name.startsWith("f-") || name.startsWith("p-")) && name.endsWith(".sock") && name.length > 7)
  await Promise.all(controls.map(async (name) => {
    const path = posix.join(runtime.dir, name)
    if (isDirectory(path)) return
    try {
      await exit(path)
    } catch {
      // The master is gone or unresponsive; unlink anyway.
    }
    unlinkIgnoringErrors(path)
  }))
  for (const name of forwards) {
    const path = posix.join(runtime.dir, name)
    if (!isDirectory(path)) unlinkIgnoringErrors(path)
  }
}

function isDirectory(path: string): boolean {
  try {
    return lstatSync(path).isDirectory()
  } catch {
    return false
  }
}

function unlinkIgnoringErrors(path: string): void {
  try {
    unlinkSync(path)
  } catch {
    // Missing or not removable; the sweep is best effort.
  }
}

// unlinkQuietly removes a file, ignoring ENOENT.
export function unlinkQuietly(file: string): void {
  try {
    unlinkSync(file)
  } catch (error) {
    if (errorCode(error) !== "ENOENT") throw error
  }
}
