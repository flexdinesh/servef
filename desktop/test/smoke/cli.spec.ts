import path from "node:path"

import { expect, test, type ElectronApplication, type Page } from "@playwright/test"

import {
  createRepository, createSession, ensureDaemon, keepDiagnostics, launchApp, launchSecond, openRoot, readmeLine, removeSession,
  schemeURL, startCliServer, stopDaemon, type CliServer, type Session,
} from "./harness.ts"

// A running CLI server (servef browser) opened from a pasted localhost URL on
// the Connect page (decision 0071), then stopped.

test.describe.configure({ mode: "serial" })

let session: Session | undefined
let app: ElectronApplication | undefined
let server: CliServer | undefined
let gamma = ""
let failed = false

function current(): { session: Session; app: ElectronApplication; server: CliServer } {
  if (!session || !app || !server) throw new Error("the app did not start")
  return { session, app, server }
}

async function windowOn(app: ElectronApplication, prefix: string): Promise<Page> {
  await expect.poll(() => app.windows().some((page) => page.url().startsWith(prefix)), { timeout: 30_000 }).toBe(true)
  const page = app.windows().find((candidate) => candidate.url().startsWith(prefix))
  if (page === undefined) throw new Error(`no window on ${prefix}`)
  return page
}

function windowTitles(app: ElectronApplication): Promise<string[]> {
  return app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows().map((window) => window.getTitle()))
}

test.beforeAll(async () => {
  const created = createSession()
  session = created
  try {
    await ensureDaemon(created)
    const alpha = await openRoot(created, createRepository(path.join(created.projects, "alpha"), "Alpha"))
    gamma = createRepository(path.join(created.projects, "gamma"), "Gamma")
    server = await startCliServer(created, gamma)
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
  await server?.stop()
  if (!session) return
  await stopDaemon(session)
  if (failed) keepDiagnostics(session, path.join(output, "session"))
  removeSession(session)
})

test("the Connect page lists the running CLI server", async () => {
  const { app, server } = current()
  const first = await app.firstWindow()
  await expect.poll(() => first.url()).toMatch(/^servef-app:\/\/local\//)
  // Another origin opens in its own window.
  await first.evaluate(() => { location.assign("servef-app://app/connect") })
  const connect = await windowOn(app, "servef-app://app/")
  const servers = connect.locator("h2:has-text('Servers on this computer') + ul")
  await expect(servers.getByRole("link", { name: `localhost:${server.port}` })).toHaveAttribute("href", `servef-app://cli-${server.port}/`)
})

test("a pasted localhost URL opens the server's document in a cli window", async () => {
  const { app, server } = current()
  const connect = await windowOn(app, "servef-app://app/")
  const pasted = `http://localhost:${server.port}/view?project=${encodeURIComponent(gamma)}&path=README.md`
  await connect.getByRole("textbox", { name: "servef link" }).fill(pasted)
  await connect.getByRole("button", { name: "Open" }).click()
  const page = await windowOn(app, `servef-app://cli-${server.port}/view?`)
  await expect(page.getByRole("main").getByText(readmeLine("Gamma"))).toBeVisible()
  await expect.poll(async () => (await windowTitles(app)).some((title) => title.endsWith(` - localhost:${server.port}`))).toBe(true)
  // The Connect window went back to the Connect page.
  await expect.poll(() => connect.url()).toBe("servef-app://app/connect")
  // The CLI control endpoint stays out of reach.
  expect(await page.evaluate(async () => (await fetch("/_servef/control", { method: "POST" })).status)).toBe(403)
})

test("a pasted URL for a port without a servef server explains why", async () => {
  const { app } = current()
  const connect = await windowOn(app, "servef-app://app/")
  for (const [pasted, reason] of [
    ["http://localhost:1/", "No servef server is running on port 1."],
    ["https://example.com/", "Not a servef link."],
  ]) {
    await connect.getByRole("textbox", { name: "servef link" }).fill(pasted)
    await connect.getByRole("button", { name: "Open" }).click()
    await expect(connect.getByRole("alert")).toHaveText(reason)
  }
})

test("a servef://localhost:<port> link opens the running server's cli window", async () => {
  const { app, session } = current()
  // Its own server: the stopped-page test below keeps the shared one's window.
  const other = await startCliServer(session, gamma)
  try {
    const query = `project=${encodeURIComponent(gamma)}&path=CHANGELOG.md`
    expect(await launchSecond(session, `servef://localhost:${other.port}/view?${query}`)).toBe(0)
    const page = await windowOn(app, `servef-app://cli-${other.port}/`)
    await expect.poll(() => page.url()).toBe(`servef-app://cli-${other.port}/view?${query}`)
    await expect(page.getByRole("main").getByText("# Changelog")).toBeVisible()
    await page.close()
  } finally {
    await other.stop()
  }
})

test("a stopped CLI server shows the stopped page", async () => {
  const { app, server } = current()
  const page = await windowOn(app, `servef-app://cli-${server.port}/`)
  await server.stop()
  await page.reload()
  await expect(page.getByRole("heading", { name: `The servef server on port ${server.port} stopped` })).toBeVisible()
  // A later reload lists again and finds no server.
  await page.reload()
  await expect(page.getByRole("heading", { name: `No servef server is running on port ${server.port}` })).toBeVisible()
})
