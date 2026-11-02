import { readdirSync, readlinkSync, writeFileSync } from "node:fs"
import path from "node:path"

import { expect, test, type ElectronApplication, type Page } from "@playwright/test"

import {
  appURL, commitFile, control, createRepository, createSession, daemonStatus, ensureDaemon, keepDiagnostics, launchApp,
  launchSecond, openRoot, publishWalkthrough, readmeLine, readNotices, removeSession, schemeURL, stopDaemon, type Session,
} from "./harness.ts"

// One app against one isolated daemon, in order: each step builds on the last
// (decision 0062 shell, 0066 lease levels and desktop events).

test.describe.configure({ mode: "serial" })

let session: Session | undefined
let app: ElectronApplication | undefined
let alpha = ""
let beta = ""
let failed = false

const tokens = '[class^="hl-"],[class*=" hl-"]'

function projectOf(page: Page): string | null {
  return new URL(page.url()).searchParams.get("project")
}

function current(): { session: Session; app: ElectronApplication } {
  if (!session || !app) throw new Error("the app did not start")
  return { session, app }
}

async function firstWindow(): Promise<Page> {
  return current().app.firstWindow()
}

async function assign(page: Page, url: string): Promise<void> {
  await page.evaluate((target) => { location.assign(target) }, url)
  await page.waitForURL(url)
}

test.beforeAll(async () => {
  const created = createSession()
  session = created
  try {
    const status = await ensureDaemon(created)
    expect(status.pid).toBeGreaterThan(0)
    alpha = await openRoot(created, createRepository(path.join(created.projects, "alpha"), "Alpha"))
    beta = await openRoot(created, createRepository(path.join(created.projects, "beta"), "Beta"))
    app = await launchApp(created, schemeURL(alpha, "README.md"))
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
  if (!session) return
  await stopDaemon(session)
  if (failed) keepDiagnostics(session, path.join(output, "session"))
  removeSession(session)
})

test("a servef:// link opens the document through the daemon", async () => {
  const page = await firstWindow()
  await expect.poll(() => page.url()).toMatch(/^servef-app:\/\/local\/view\?/)
  expect(projectOf(page)).toBe(alpha)
  await expect(page.getByRole("main").getByText(readmeLine("Alpha"))).toBeVisible()
  const live = page.locator(".status-live")
  await expect(live).toHaveAttribute("data-mode", "watching")
  await expect(live).toContainText("Live")
})

test("an edit on disk rereads over the proxied event stream", async () => {
  const page = await firstWindow()
  writeFileSync(path.join(alpha, "README.md"), "# Alpha\n\nEdited on disk.\n")
  await expect(page.getByRole("main").getByText("Edited on disk.")).toBeVisible()
})

test("code highlights in the worker", async () => {
  const page = await firstWindow()
  await assign(page, appURL(alpha, "src/main.go"))
  await expect(page.getByRole("main").locator(tokens).first()).toBeVisible()
})

test("the renderer cannot reach the daemon control API", async () => {
  const page = await firstWindow()
  const statuses = await page.evaluate(async () => {
    const paths = ["/_daemon/status", "/%5Fdaemon/status", "/%5fdaemon/status", "/_daemon/events"]
    return Promise.all(paths.map(async (route) => (await fetch(route)).status))
  })
  expect(statuses).toEqual([403, 403, 403, 403])
  expect(await page.evaluate(async () => (await fetch("/api/projects")).status)).toBe(200)
})

test("navigation outside the app origin stays out of the window", async () => {
  const { app } = current()
  const page = await firstWindow()
  const before = page.url()
  const windows = app.windows().length
  await page.evaluate(() => { location.href = "file:///etc/passwd" })
  await page.evaluate(() => { window.open("file:///etc/passwd") })
  // Record http(s) hand-offs instead of opening the host browser.
  const recording = await app.evaluate(({ shell }) => {
    const calls: string[] = []
    const record = async (url: string) => { calls.push(url) }
    try {
      Object.defineProperty(shell, "openExternal", { value: record, configurable: true, writable: true })
    } catch {
      return false
    }
    Reflect.set(globalThis, "servefSmokeExternal", calls)
    return Object.is(shell.openExternal, record)
  })
  if (recording) {
    await page.evaluate(() => { window.open("https://example.com/") })
    await expect.poll(() => app.evaluate(() => {
      const calls: unknown = Reflect.get(globalThis, "servefSmokeExternal")
      return Array.isArray(calls) ? calls.filter((call) => typeof call === "string") : []
    })).toEqual(["https://example.com/"])
  } else {
    test.info().annotations.push({ type: "skipped", description: "shell.openExternal could not be recorded; https hand-off unchecked" })
  }
  await page.waitForTimeout(1_000)
  expect(page.url()).toBe(before)
  expect(app.windows()).toHaveLength(windows)
})

test("a second launch routes its link into the running app", async () => {
  const { app, session } = current()
  const opened = app.waitForEvent("window", { timeout: 60_000 })
  expect(await launchSecond(session, schemeURL(beta, "README.md"))).toBe(0)
  const second = await opened
  await expect(second.getByRole("main").getByText(readmeLine("Beta"))).toBeVisible()
  expect(projectOf(second)).toBe(beta)
})

test("a commit in a project the window left notifies", async () => {
  const { session } = current()
  const page = await firstWindow()
  await assign(page, appURL(beta, "README.md"))
  await expect(page.getByRole("main").getByText(readmeLine("Beta"))).toBeVisible()
  commitFile(alpha, "NOTES.md", "# Notes\n", "docs: smoke commit")
  await expect.poll(() => readNotices(session).filter((notice) => notice.kind === "commit" && notice.project === alpha)
    .map((notice) => `${notice.title} ${notice.body}`).join("\n"), { timeout: 45_000 }).toContain("smoke commit")
})

test("a walkthrough announcement notifies", async () => {
  const { session } = current()
  const published = await publishWalkthrough(session, alpha, "smoke-tour")
  expect(published.project).toBe(alpha)
  const response = await control(session, "POST", "/_daemon/walkthroughs", published)
  expect(response.status).toBe(202)
  await expect.poll(() => readNotices(session).some((notice) => notice.kind === "walkthrough" && notice.project === alpha), { timeout: 30_000 }).toBe(true)
})

test("a killed daemon is ensured again by the next request", async () => {
  const { session } = current()
  const page = await firstWindow()
  const before = await daemonStatus(session)
  process.kill(before.pid, "SIGKILL")
  // Wait until the old daemon no longer answers (or one already replaced it).
  await expect.poll(async () => {
    try {
      return (await daemonStatus(session)).pid !== before.pid
    } catch {
      return true
    }
  }).toBe(true)
  expect(await page.evaluate(async () => (await fetch("/api/projects")).status)).toBe(200)
  const after = await daemonStatus(session)
  expect(after.pid).not.toBe(before.pid)
  // The app started this daemon: it must hold none of Electron's descriptors.
  const held = readdirSync(`/proc/${after.pid}/fd`).map((fd) => {
    try {
      return readlinkSync(`/proc/${after.pid}/fd/${fd}`)
    } catch {
      return ""
    }
  })
  expect(held.filter((target) => target.startsWith(session.userData))).toEqual([])
})
