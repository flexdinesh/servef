import { execFile } from "node:child_process"
import { createHash } from "node:crypto"
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs"
import path from "node:path"
import { promisify } from "node:util"

import { expect, test, type ElectronApplication, type Page } from "@playwright/test"

import { sshBinary, sshSkip, startSshd, type Sshd } from "../ssh/sshd.ts"
import {
  createRepository, createSession, keepDiagnostics, launchApp, readNotices, removeSession, stopDaemon, type Session,
} from "./harness.ts"

// The app against a private sshd (decisions 0070, 0076), in order: the
// Connect page connects to a config alias and shows its host page, a remote
// root opens in an ssh window, a remote walkthrough notifies through the ssh
// connection, a lost master reconnects, the folder picker opens a new remote
// folder, and a remote CLI server opens through a forwarded port. Nothing
// asks for confirmation: the user chose the host.

const run = promisify(execFile)
const skip = sshSkip()
test.skip(skip !== false, typeof skip === "string" ? skip : "")
test.describe.configure({ mode: "serial" })

const ALIAS = "servef-test"
const ID = `ssh-${ALIAS}`
const HOST_PAGE = `servef-app://app/ssh/${ID}`

let session: Session | undefined
let sshd: Sshd | undefined
let app: ElectronApplication | undefined
let alpha = ""
let beta = ""
let projects = ""
let failed = false

interface Paths {
  runtime: string
  dialogLog: string
}
let paths: Paths = { runtime: "", dialogLog: "" }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function current(): { session: Session; sshd: Sshd; app: ElectronApplication } {
  if (!session || !sshd || !app) throw new Error("the app did not start")
  return { session, sshd, app }
}

function extras(fixture: Sshd): Record<string, string> {
  return {
    SERVEF_DESKTOP_SSH_CONFIG: fixture.config,
    SERVEF_DESKTOP_RUNTIME_DIR: paths.runtime,
    SERVEF_DESKTOP_AUTO_CONFIRM: "1",
    SERVEF_DESKTOP_DIALOG_LOG: paths.dialogLog,
  }
}

async function windowOn(app: ElectronApplication, prefix: string): Promise<Page> {
  await expect.poll(() => app.windows().some((page) => page.url().startsWith(prefix)), { timeout: 30_000 }).toBe(true)
  const page = app.windows().find((candidate) => candidate.url().startsWith(prefix))
  if (page === undefined) throw new Error(`no window on ${prefix}`)
  return page
}

function projectOf(page: Page): string | null {
  return new URL(page.url()).searchParams.get("project")
}

// controlPath is the app's ControlMaster socket: <runtime>/c-<12 hex sha256(id)>.
function controlPath(): string {
  return path.join(paths.runtime, `c-${createHash("sha256").update(ID).digest("hex").slice(0, 12)}`)
}

function dialogs(): unknown[] {
  if (!existsSync(paths.dialogLog)) return []
  return readFileSync(paths.dialogLog, "utf8").split("\n").filter((line) => line.trim() !== "").map((line): unknown => JSON.parse(line))
}

async function windowTitles(): Promise<string[]> {
  return current().app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows().map((window) => window.getTitle()))
}

test.beforeAll(async () => {
  if (skip !== false) return
  const created = createSession()
  session = created
  try {
    const fixture = await startSshd()
    sshd = fixture
    paths = { runtime: path.join(fixture.dir, "app"), dialogLog: path.join(created.dir, "dialogs.jsonl") }
    projects = path.join(fixture.dir, "projects")
    mkdirSync(projects, { recursive: true })
    alpha = createRepository(path.join(projects, "alpha"), "Alpha")
    beta = createRepository(path.join(projects, "beta"), "Beta")
    fixture.writeRoots([alpha, beta])
    app = await launchApp(created, "servef://local/", extras(fixture))
    await app.context().tracing.start({ screenshots: true, snapshots: true })
  } catch (error) {
    failed = true
    throw error
  }
})

test.afterEach(async () => {
  const info = test.info()
  if (info.status === info.expectedStatus) return
  failed = true
  for (const [index, page] of (app?.windows() ?? []).entries()) {
    await page.screenshot({ path: info.outputPath(`window-${index}.png`) }).catch(() => undefined)
  }
})

test.afterAll(async () => {
  const output = test.info().outputPath()
  if (app) {
    await app.context().tracing.stop(failed ? { path: path.join(output, "trace.zip") } : undefined).catch(() => undefined)
    await app.close().catch(() => undefined)
  }
  if (session) await stopDaemon(session)
  if (failed && session) {
    const destination = path.join(output, "session")
    keepDiagnostics(session, destination)
    if (existsSync(paths.dialogLog)) writeFileSync(path.join(destination, "dialogs.jsonl"), readFileSync(paths.dialogLog))
    if (sshd) writeFileSync(path.join(destination, "sshd.log"), sshd.log())
  }
  await sshd?.close()
  if (session) removeSession(session)
})

test("the Connect page connects to a config alias and shows its host page", async () => {
  const { app, session } = current()
  const first = await app.firstWindow()
  await first.evaluate(() => { location.assign("servef-app://app/connect") })
  const connect = await windowOn(app, "servef-app://app/")
  const aliases = connect.locator("h2:has-text('SSH hosts from your config') + ul")
  await aliases.getByRole("button", { name: ALIAS, exact: true }).click()
  await expect.poll(() => connect.url(), { timeout: 45_000 }).toBe(HOST_PAGE)
  await expect(connect.getByRole("heading", { level: 1, name: ALIAS })).toBeVisible()
  await expect(connect.getByRole("link", { name: alpha })).toHaveAttribute("href", `servef-app://${ID}/?project=${encodeURIComponent(alpha)}`)
  await expect(connect.getByRole("link", { name: "Browse folders" })).toBeVisible()
  await expect(connect.getByRole("heading", { level: 2, name: "Servers" })).toBeVisible()

  const store: unknown = JSON.parse(readFileSync(path.join(session.userData, "connections.json"), "utf8"))
  const entries = isRecord(store) && Array.isArray(store.ssh) ? store.ssh : []
  expect(entries.map((entry: unknown) => isRecord(entry) ? Object.keys(entry).sort() : null)).toEqual([["addedAt", "destination", "id", "port"]])
  expect(entries.map((entry: unknown) => isRecord(entry) ? [entry.id, entry.destination] : null)).toEqual([[ID, ALIAS]])
  expect(dialogs()).toEqual([])
})

test("a remote root opens in an ssh window", async () => {
  const { app } = current()
  const connect = await windowOn(app, HOST_PAGE)
  // Another origin opens in its own window; the host page stays.
  const href = await connect.getByRole("link", { name: alpha }).getAttribute("href")
  await connect.evaluate((target) => { location.assign(target ?? "") }, href)
  const page = await windowOn(app, `servef-app://${ID}/`)
  expect(projectOf(page)).toBe(alpha)
  await expect.poll(async () => (await windowTitles()).join("\n")).toContain(` - ${ALIAS} (SSH)`)
  expect(await page.evaluate(async () => (await fetch("/api/projects")).status)).toBe(200)
  expect(await page.evaluate(async () => (await fetch("/_daemon/status")).status)).toBe(403)
})

test("a walkthrough published on the host notifies through the ssh connection", async () => {
  const { session, sshd } = current()
  // beta is not the window's project, so the focus gate lets the notice through.
  const draft = { project: beta, id: "remote-tour", title: "Remote tour", steps: [{ title: "The README", body: "Where it starts.", file: { path: "README.md" } }] }
  const file = path.join(sshd.dir, "remote-tour.json")
  writeFileSync(file, JSON.stringify(draft), { mode: 0o644 })
  const published = await sshd.ssh(ALIAS, `${sshd.remoteServef} walkthrough publish --no-open --json ${file}`)
  expect(published.code, published.stderr).toBe(0)
  const result: unknown = JSON.parse(published.stdout.trim().split("\n").at(-1) ?? "null")
  expect(isRecord(result) ? [result.project, result.id] : null).toEqual([beta, "remote-tour"])

  await expect.poll(() => readNotices(session).filter((notice) => notice.kind === "walkthrough" && notice.connection === ID).length, { timeout: 30_000 }).toBeGreaterThan(0)
  const [notice] = readNotices(session).filter((notice) => notice.kind === "walkthrough" && notice.connection === ID)
  const target = new URL(notice?.target ?? "")
  expect(`${target.protocol}//${target.host}${target.pathname}`).toBe(`servef-app://${ID}/view`)
  expect(target.searchParams.get("project")).toBe(beta)
  expect(target.searchParams.get("wt")).toBe("remote-tour")
})

test("a lost ssh master shows reconnecting and recovers", async () => {
  const { app } = current()
  const page = await windowOn(app, `servef-app://${ID}/`)
  // Record every title change: reconnecting lasts about the 1 s backoff.
  await app.evaluate(({ BrowserWindow }) => {
    const seen: string[] = []
    Reflect.set(globalThis, "servefSmokeTitles", seen)
    setInterval(() => {
      for (const window of BrowserWindow.getAllWindows()) {
        const title = window.getTitle()
        if (!seen.includes(title)) seen.push(title)
      }
    }, 25)
  })
  const seen = () => app.evaluate(() => {
    const titles: unknown = Reflect.get(globalThis, "servefSmokeTitles")
    return Array.isArray(titles) ? titles.filter((title) => typeof title === "string") : []
  })
  expect(existsSync(controlPath())).toBe(true)
  await run(sshBinary(), ["-O", "exit", "-o", `ControlPath=${controlPath()}`, "-F", "none", "--", ALIAS], { timeout: 10_000 })

  await expect.poll(async () => (await seen()).some((title) => title.endsWith(` - ${ALIAS} (SSH, reconnecting)`)), { timeout: 30_000 }).toBe(true)
  await expect.poll(async () => (await windowTitles()).join("\n"), { timeout: 45_000 }).toContain(` - ${ALIAS} (SSH)`)
  expect(existsSync(controlPath())).toBe(true)
  expect(await page.evaluate(async () => (await fetch("/api/projects")).status)).toBe(200)
})

test("the folder picker opens a new remote folder in an ssh window", async () => {
  const { app } = current()
  const delta = createRepository(path.join(projects, "delta"), "Delta")
  const connect = await windowOn(app, HOST_PAGE)
  await connect.goto(`${HOST_PAGE}/browse?path=${encodeURIComponent(projects)}`)
  await expect(connect.getByRole("heading", { level: 1, name: `Browse ${ALIAS}` })).toBeVisible()
  await connect.getByRole("link", { name: "delta", exact: true }).click()
  await expect(connect.getByText(delta, { exact: true })).toBeVisible()
  await connect.getByRole("button", { name: "Open this folder" }).click()
  await expect.poll(() => app.windows().some((page) => projectOf(page) === delta && page.url().startsWith(`servef-app://${ID}/`)), { timeout: 30_000 }).toBe(true)
  // The picker's window returns to the host page, which lists the new root.
  await expect.poll(() => connect.url()).toBe(HOST_PAGE)
  await expect(connect.getByRole("link", { name: delta })).toBeVisible()
  expect(dialogs()).toEqual([])
})

test("a remote CLI server opens through a forwarded port", async () => {
  const { app, sshd } = current()
  const gamma = createRepository(path.join(projects, "gamma"), "Gamma")
  const server = await sshd.startServer(gamma)
  try {
    const connect = await windowOn(app, HOST_PAGE)
    await connect.reload()
    const label = `127.0.0.1:${server.port}`
    await connect.getByRole("button", { name: label }).click()
    const page = await windowOn(app, `servef-app://${ID}-${server.port}/`)
    await expect.poll(async () => (await windowTitles()).some((title) => title.endsWith(` - ${ALIAS}:${server.port} (SSH)`))).toBe(true)
    const projectsJSON = await page.evaluate(async () => (await fetch("/api/projects")).text())
    expect(projectsJSON).toContain(JSON.stringify(gamma))
    expect(await page.evaluate(async () => (await fetch("/_servef/control", { method: "POST" })).status)).toBe(403)
    const port = `p-${createHash("sha256").update(`${ID}-${server.port}`).digest("hex").slice(0, 12)}.sock`
    expect(existsSync(path.join(paths.runtime, port))).toBe(true)
    // Closing its only window cancels the forward.
    await page.close()
    await expect.poll(() => existsSync(path.join(paths.runtime, port)), { timeout: 15_000 }).toBe(false)
  } finally {
    await server.close()
  }
})
