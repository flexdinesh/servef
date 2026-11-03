import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs"
import path from "node:path"

import { expect, test, type ElectronApplication } from "@playwright/test"

import { AUTOSTART_FILE, autostartEntry } from "../../src/main/autostart.ts"
import {
  createRepository, createSession, ensureDaemon, keepDiagnostics, launchAppWith, launchSecondWith, removeSession, stopDaemon, type Session,
} from "./harness.ts"

// OS entry points (decision 0072): `--open-path` folders on cold and second
// launches, files refused with a report, and Open at Login's autostart entry.

test.describe.configure({ mode: "serial" })

let session: Session | undefined
let app: ElectronApplication | undefined
let alpha = ""
let beta = ""
let file = ""
let dialogLog = ""
let failed = false

function current(): { session: Session; app: ElectronApplication } {
  if (!session || !app) throw new Error("the app did not start")
  return { session, app }
}

function landing(project: string): string {
  return `servef-app://local/?project=${encodeURIComponent(project)}`
}

function extra(): Record<string, string> {
  return { SERVEF_DESKTOP_AUTO_CONFIRM: "1", SERVEF_DESKTOP_DIALOG_LOG: dialogLog }
}

// roots lists the daemon's saved roots, sorted.
function roots(): string[] {
  const { session } = current()
  if (!existsSync(session.rootsFile)) return []
  const value: unknown = JSON.parse(readFileSync(session.rootsFile, "utf8"))
  if (typeof value !== "object" || value === null || !("roots" in value) || !Array.isArray(value.roots)) throw new Error(`malformed roots file: ${JSON.stringify(value)}`)
  return value.roots.map((root: unknown) => String(root)).sort()
}

function reports(): string[] {
  if (!existsSync(dialogLog)) return []
  return readFileSync(dialogLog, "utf8").split("\n").filter((line) => line.trim() !== "")
}

function windowURLs(): string[] {
  return current().app.windows().map((page) => page.url()).sort()
}

async function clickOpenAtLogin(): Promise<void> {
  await current().app.evaluate(({ Menu }) => {
    const item = Menu.getApplicationMenu()?.getMenuItemById("open-at-login")
    if (!item) throw new Error("no Open at Login menu item")
    item.click()
  })
}

function openAtLoginChecked(): Promise<boolean | null> {
  return current().app.evaluate(({ Menu }) => Menu.getApplicationMenu()?.getMenuItemById("open-at-login")?.checked ?? null)
}

test.beforeAll(async () => {
  const created = createSession()
  session = created
  dialogLog = path.join(created.dir, "dialogs.jsonl")
  try {
    await ensureDaemon(created)
    alpha = createRepository(path.join(created.projects, "alpha"), "Alpha")
    beta = createRepository(path.join(created.projects, "beta"), "Beta")
    file = path.join(alpha, "README.md")
    app = await launchAppWith(created, ["--open-path", alpha], extra())
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
  if (app) await app.close().catch(() => undefined)
  if (!session) return
  await stopDaemon(session)
  if (failed) {
    const destination = path.join(test.info().outputPath(), "session")
    keepDiagnostics(session, destination)
    if (existsSync(dialogLog)) writeFileSync(path.join(destination, "dialogs.jsonl"), readFileSync(dialogLog))
  }
  removeSession(session)
})

test("a cold launch with --open-path adds the folder and opens its project", async () => {
  const page = await current().app.firstWindow()
  await page.waitForURL(landing(alpha))
  expect(await page.evaluate(async () => (await fetch("/api/projects")).status)).toBe(200)
  expect(roots()).toEqual([alpha])
  expect(current().app.windows()).toHaveLength(1)
})

test("a second launch hands its folders over: new projects get windows, open ones are reused", async () => {
  expect(await launchSecondWith(current().session, ["--open-path", beta, alpha], extra())).toBe(0)
  await expect.poll(windowURLs, { timeout: 30_000 }).toEqual([landing(alpha), landing(beta)].sort())
  expect(roots()).toEqual([alpha, beta].sort())
})

test("a file is refused with a report and adds nothing", async () => {
  expect(await launchSecondWith(current().session, ["--open-path", file], extra())).toBe(0)
  await expect.poll(() => reports().length, { timeout: 30_000 }).toBe(1)
  const [line] = reports()
  expect(line).toContain("Could not open folder")
  expect(line).toContain(file)
  expect(current().app.windows()).toHaveLength(2)
  expect(roots()).toEqual([alpha, beta].sort())
})

test("Open at Login writes and removes the autostart entry", async () => {
  const { session, app } = current()
  const entry = path.join(session.autostart, AUTOSTART_FILE)
  expect(await openAtLoginChecked()).toBe(false)
  const command = await app.evaluate(({ app: electron }) => [process.execPath, electron.getAppPath()])
  await clickOpenAtLogin()
  await expect.poll(() => existsSync(entry)).toBe(true)
  expect(readFileSync(entry, "utf8")).toBe(autostartEntry(command))
  await expect.poll(openAtLoginChecked).toBe(true)
  await clickOpenAtLogin()
  await expect.poll(() => existsSync(entry)).toBe(false)
  await expect.poll(openAtLoginChecked).toBe(false)
})

test("a cold launch with only a refused file still shows a window, and a seeded entry shows checked", async () => {
  const { session } = current()
  await current().app.close()
  app = undefined
  mkdirSync(session.autostart, { recursive: true })
  writeFileSync(path.join(session.autostart, AUTOSTART_FILE), autostartEntry(["/opt/servef/servef-desktop"]))
  app = await launchAppWith(session, ["--open-path", file], extra())
  const page = await app.firstWindow()
  await page.waitForURL("servef-app://local/")
  await expect.poll(() => reports().length, { timeout: 30_000 }).toBe(2)
  expect(app.windows()).toHaveLength(1)
  expect(roots()).toEqual([alpha, beta].sort())
  expect(await openAtLoginChecked()).toBe(true)
})
