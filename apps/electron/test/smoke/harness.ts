import { execFile, spawn, spawnSync } from "node:child_process"
import { appendFileSync, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs"
import { request } from "node:http"
import { createRequire } from "node:module"
import { tmpdir } from "node:os"
import path from "node:path"
import { fileURLToPath } from "node:url"
import { promisify } from "node:util"

import { _electron, type ElectronApplication } from "@playwright/test"

// Shared by the smoke specs and scripts/measure-memory.ts: an isolated daemon
// (socket, roots file, walkthrough cache), git fixtures, and the app launched
// against them. Nothing touches the user's daemon, roots, or app data.

const run = promisify(execFile)

export const desktopDir = fileURLToPath(new URL("../..", import.meta.url))

export interface Session {
  dir: string
  socket: string
  rootsFile: string
  userData: string
  cache: string
  // Empty XDG_RUNTIME_DIR for CLI calls, so default sockets never reach the user's daemon.
  runtime: string
  noticeLog: string
  electronLog: string
  projects: string
  // XDG autostart directory for Open at Login (decision 0072); never created by the harness.
  autostart: string
}

export interface DaemonStatus {
  pid: number
  protocol: number
}

export interface Notice {
  connection: string
  kind: string
  project: string
  title: string
  body: string
  target: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

export function servefBinary(): string {
  const binary = process.env.SERVEF_DESKTOP_BINARY
  if (!binary) throw new Error("SERVEF_DESKTOP_BINARY must name a servef binary built from this checkout (mise run build:electron builds .dev/electron/servef)")
  const resolved = path.resolve(binary)
  if (!existsSync(resolved)) throw new Error(`SERVEF_DESKTOP_BINARY does not exist: ${resolved}`)
  return resolved
}

// The binary comes from install.js only; never download implicitly here.
export function electronExecutable(): string {
  const require = createRequire(path.join(desktopDir, "package.json"))
  const pkg = path.dirname(require.resolve("electron/package.json"))
  const pathFile = path.join(pkg, "path.txt")
  const executable = existsSync(pathFile) ? path.join(pkg, "dist", readFileSync(pathFile, "utf8")) : ""
  if (!executable || !existsSync(executable)) throw new Error("Electron binary missing: run node apps/electron/node_modules/electron/install.js")
  return executable
}

// Extra Chromium switches, e.g. --ozone-platform=wayland or --disable-gpu.
export function electronArgs(extra = process.env.SERVEF_DESKTOP_ELECTRON_ARGS ?? ""): string[] {
  return ["--no-sandbox", ...extra.split(/\s+/).filter((arg) => arg !== "")]
}

// <base>/sfd-XXXXXX/d.sock must fit sun_path (104 bytes on macOS).
function shortTempBase(): string {
  const base = realpathSync(tmpdir())
  return base.length <= 80 ? base : "/tmp"
}

export function createSession(): Session {
  const dir = mkdtempSync(path.join(shortTempBase(), "sfd-"))
  const session: Session = {
    dir,
    socket: path.join(dir, "d.sock"),
    rootsFile: path.join(dir, "roots.json"),
    userData: path.join(dir, "user-data"),
    cache: path.join(dir, "cache"),
    runtime: path.join(dir, "run"),
    noticeLog: path.join(dir, "notices.jsonl"),
    electronLog: path.join(dir, "electron.log"),
    projects: path.join(dir, "projects"),
    autostart: path.join(dir, "autostart"),
  }
  for (const directory of [session.userData, session.cache, session.runtime, session.projects]) {
    mkdirSync(directory, { recursive: true, mode: 0o700 })
  }
  return session
}

function inherited(): Record<string, string> {
  const env: Record<string, string> = {}
  for (const [key, value] of Object.entries(process.env)) {
    if (value === undefined || key === "ELECTRON_RUN_AS_NODE" || key.startsWith("SERVEF_DESKTOP_") || key.startsWith("GIT_")) continue
    env[key] = value
  }
  return env
}

function cliEnv(session: Session): Record<string, string> {
  return { ...inherited(), XDG_CACHE_HOME: session.cache, XDG_RUNTIME_DIR: session.runtime }
}

// XDG_RUNTIME_DIR stays inherited: Wayland finds its display socket there.
// extra adds dev seams (ssh config, runtime dir, auto-confirm, dialog log).
export function appEnv(session: Session, extra: Record<string, string> = {}): Record<string, string> {
  return {
    ...inherited(),
    SERVEF_DESKTOP_BINARY: servefBinary(),
    SERVEF_DESKTOP_SOCKET: session.socket,
    SERVEF_DESKTOP_ROOTS_FILE: session.rootsFile,
    SERVEF_DESKTOP_USER_DATA: session.userData,
    SERVEF_DESKTOP_NOTICE_LOG: session.noticeLog,
    SERVEF_DESKTOP_AUTOSTART_DIR: session.autostart,
    XDG_CACHE_HOME: session.cache,
    ...extra,
  }
}

function parseStatus(text: string): DaemonStatus {
  const value: unknown = JSON.parse(text)
  if (!isRecord(value) || typeof value.pid !== "number" || typeof value.protocol !== "number") {
    throw new Error(`unexpected daemon status: ${text}`)
  }
  return { pid: value.pid, protocol: value.protocol }
}

export async function ensureDaemon(session: Session): Promise<DaemonStatus> {
  const args = ["daemon", "ensure", "--json", "--socket", session.socket, "--roots-file", session.rootsFile]
  const { stdout } = await run(servefBinary(), args, { env: cliEnv(session), timeout: 30_000 })
  return parseStatus(stdout)
}

export async function daemonStatus(session: Session): Promise<DaemonStatus> {
  const { stdout } = await run(servefBinary(), ["daemon", "status", "--json", "--socket", session.socket], { env: cliEnv(session), timeout: 15_000 })
  return parseStatus(stdout)
}

export async function stopDaemon(session: Session): Promise<void> {
  try {
    await run(servefBinary(), ["daemon", "stop", "--socket", session.socket], { env: cliEnv(session), timeout: 15_000 })
  } catch {
    // Already stopped.
  }
}

export interface ControlResponse {
  status: number
  body: unknown
}

// The daemon control API over the socket: Host localhost, no Origin.
export function control(session: Session, method: string, route: string, body?: unknown): Promise<ControlResponse> {
  return new Promise((resolve, reject) => {
    const payload = body === undefined ? "" : JSON.stringify(body)
    const headers: Record<string, string> = { Host: "localhost" }
    if (payload !== "") {
      headers["Content-Type"] = "application/json"
      headers["Content-Length"] = String(Buffer.byteLength(payload))
    }
    const req = request({ socketPath: session.socket, method, path: route, headers }, (response) => {
      const chunks: Buffer[] = []
      response.on("data", (chunk: Buffer) => { chunks.push(chunk) })
      response.on("end", () => {
        const text = Buffer.concat(chunks).toString("utf8")
        let parsed: unknown = text
        try {
          parsed = JSON.parse(text)
        } catch {
          // Not JSON; keep the text for diagnostics.
        }
        resolve({ status: response.statusCode ?? 0, body: parsed })
      })
      response.on("error", reject)
    })
    req.on("error", reject)
    req.setTimeout(15_000, () => { req.destroy(new Error(`${method} ${route} timed out`)) })
    req.end(payload)
  })
}

// Adds the directory as a root (or finds it within one); returns its project.
export async function openRoot(session: Session, directory: string): Promise<string> {
  const response = await control(session, "POST", "/_daemon/open", { path: directory })
  if ((response.status !== 200 && response.status !== 201) || !isRecord(response.body) || typeof response.body.project !== "string") {
    throw new Error(`open ${directory}: ${response.status} ${JSON.stringify(response.body)}`)
  }
  return response.body.project
}

const gitEnv = {
  ...inherited(),
  GIT_CONFIG_GLOBAL: "/dev/null",
  GIT_CONFIG_NOSYSTEM: "1",
  GIT_AUTHOR_NAME: "Smoke",
  GIT_AUTHOR_EMAIL: "smoke@example.invalid",
  GIT_COMMITTER_NAME: "Smoke",
  GIT_COMMITTER_EMAIL: "smoke@example.invalid",
}

export function git(directory: string, ...args: string[]): string {
  const result = spawnSync("git", ["-c", "commit.gpgsign=false", ...args], { cwd: directory, env: gitEnv, encoding: "utf8" })
  if (result.status !== 0) throw new Error(`git ${args.join(" ")}: ${result.stderr}`)
  return result.stdout.trim()
}

export function readmeLine(title: string): string {
  return `The ${title} fixture README.`
}

// A repository with a README, Go and TypeScript sources, and two commits.
export function createRepository(directory: string, title: string): string {
  mkdirSync(path.join(directory, "src"), { recursive: true })
  writeFileSync(path.join(directory, "README.md"), `# ${title}\n\n${readmeLine(title)}\n`)
  writeFileSync(path.join(directory, "src/main.go"), "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")
  writeFileSync(path.join(directory, "src/app.ts"), "export const answer = 1\n\nexport function run(): number {\n  return answer\n}\n")
  git(directory, "init", "-q", "--initial-branch=main")
  git(directory, "add", ".")
  git(directory, "commit", "-qm", "initial")
  writeFileSync(path.join(directory, "CHANGELOG.md"), "# Changelog\n")
  git(directory, "add", ".")
  git(directory, "commit", "-qm", "docs: add a changelog")
  return realpathSync(directory)
}

export function commitFile(directory: string, name: string, content: string, subject: string): void {
  writeFileSync(path.join(directory, name), content)
  git(directory, "add", name)
  git(directory, "commit", "-qm", subject)
}

export function schemeURL(project: string, file: string): string {
  return `servef://local/view?project=${encodeURIComponent(project)}&path=${encodeURIComponent(file)}`
}

export function appURL(project: string, file: string): string {
  return `servef-app://local/view?project=${encodeURIComponent(project)}&path=${encodeURIComponent(file)}`
}

export function launchApp(session: Session, url: string, extra: Record<string, string> = {}): Promise<ElectronApplication> {
  return launchAppWith(session, [url], extra)
}

// launchAppWith launches the app with args after the app directory (a link, or `--open-path <paths…>`).
export async function launchAppWith(session: Session, args: readonly string[], extra: Record<string, string> = {}): Promise<ElectronApplication> {
  const app = await _electron.launch({
    executablePath: electronExecutable(),
    args: [...electronArgs(), desktopDir, ...args],
    env: appEnv(session, extra),
    cwd: desktopDir,
    timeout: 60_000,
  })
  const log = (chunk: Buffer) => { appendFileSync(session.electronLog, chunk) }
  app.process().stdout?.on("data", log)
  app.process().stderr?.on("data", log)
  return app
}

// Launches Electron again with the same userData; it must hand the URL (or
// args) to the running instance and exit. Resolves its exit code.
export function launchSecond(session: Session, url: string, extra: Record<string, string> = {}): Promise<number | null> {
  return launchSecondWith(session, [url], extra)
}

export function launchSecondWith(session: Session, args: readonly string[], extra: Record<string, string> = {}): Promise<number | null> {
  return new Promise((resolve, reject) => {
    const child = spawn(electronExecutable(), [...electronArgs(), desktopDir, ...args], { env: appEnv(session, extra), cwd: desktopDir, stdio: ["ignore", "pipe", "pipe"] })
    let output = ""
    const collect = (chunk: Buffer) => { output += chunk.toString("utf8") }
    child.stdout.on("data", collect)
    child.stderr.on("data", collect)
    const timer = setTimeout(() => {
      child.kill("SIGKILL")
      reject(new Error(`second launch did not exit within 30 s:\n${output}`))
    }, 30_000)
    child.on("error", (error) => {
      clearTimeout(timer)
      reject(error)
    })
    child.on("exit", (code) => {
      clearTimeout(timer)
      appendFileSync(session.electronLog, `second launch exited ${code}:\n${output}\n`)
      resolve(code)
    })
  })
}

export function readNotices(session: Session): Notice[] {
  if (!existsSync(session.noticeLog)) return []
  const notices: Notice[] = []
  for (const line of readFileSync(session.noticeLog, "utf8").split("\n")) {
    if (line.trim() === "") continue
    const value: unknown = JSON.parse(line)
    if (!isRecord(value)) throw new Error(`notice is not an object: ${line}`)
    const { connection, kind, project, title, body, target } = value
    if (typeof connection !== "string" || typeof kind !== "string" || typeof project !== "string" || typeof title !== "string" || typeof body !== "string" || typeof target !== "string") {
      throw new Error(`malformed notice: ${line}`)
    }
    notices.push({ connection, kind, project, title, body, target })
  }
  return notices
}

export interface Published {
  id: string
  project: string
  created: boolean
}

// Publishes into the session's walkthrough cache with the CLI. Its own
// announcement goes to the (empty) default socket; callers announce explicitly.
export async function publishWalkthrough(session: Session, project: string, id: string): Promise<Published> {
  const draft = {
    project,
    id,
    title: "Smoke tour",
    steps: [{ title: "The README", body: "Where the fixture starts.", file: { path: "README.md" } }],
  }
  const file = path.join(session.dir, `${id}.json`)
  writeFileSync(file, JSON.stringify(draft))
  const { stdout } = await run(servefBinary(), ["walkthrough", "publish", "--no-open", "--json", file], { env: cliEnv(session), timeout: 30_000 })
  const value: unknown = JSON.parse(stdout)
  if (!isRecord(value) || typeof value.id !== "string" || typeof value.project !== "string" || typeof value.created !== "boolean") {
    throw new Error(`unexpected publish result: ${stdout}`)
  }
  return { id: value.id, project: value.project, created: value.created }
}

export interface CliServer {
  port: number
  // stop sends SIGTERM and resolves once the server exited.
  stop(): Promise<void>
}

// Starts `servef browser --no-open --port 0 <directory>` with the session's
// XDG_CACHE_HOME, so its control record is the one the app lists (decision
// 0071). Resolves once it printed its http://localhost:<port>/ address.
export function startCliServer(session: Session, directory: string): Promise<CliServer> {
  return new Promise((resolve, reject) => {
    const child = spawn(servefBinary(), ["browser", "--no-open", "--port", "0", "--", directory], { env: cliEnv(session), stdio: ["ignore", "pipe", "pipe"] })
    const exited = new Promise<void>((done) => child.on("exit", () => done()))
    let output = ""
    const timer = setTimeout(() => {
      child.kill("SIGKILL")
      reject(new Error(`servef browser printed no address within 30 s:\n${output}`))
    }, 30_000)
    const collect = (chunk: Buffer) => {
      output += chunk.toString("utf8")
      appendFileSync(session.electronLog, `[servef browser] ${chunk.toString("utf8")}`)
      const port = /http:\/\/localhost:(\d+)\//.exec(output)?.[1]
      if (port === undefined) return
      clearTimeout(timer)
      resolve({
        port: Number(port),
        stop: async () => {
          if (child.exitCode === null && child.signalCode === null) child.kill("SIGTERM")
          await exited
        },
      })
    }
    child.stdout.on("data", collect)
    child.stderr.on("data", collect)
    child.on("error", (error) => {
      clearTimeout(timer)
      reject(error)
    })
    child.on("exit", (code) => {
      clearTimeout(timer)
      reject(new Error(`servef browser exited ${code}:\n${output}`))
    })
  })
}

// Copies logs worth reading after a failure; sockets and app data stay behind.
export function keepDiagnostics(session: Session, destination: string): void {
  mkdirSync(destination, { recursive: true })
  for (const file of [session.noticeLog, session.electronLog, session.rootsFile, `${session.socket}.log`]) {
    if (existsSync(file)) cpSync(file, path.join(destination, path.basename(file)))
  }
}

export function removeSession(session: Session): void {
  rmSync(session.dir, { recursive: true, force: true })
}
