import { execFileSync } from "node:child_process"
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"

import { expect, test } from "@playwright/test"

// `servef --remote --web` over real TLS, in the built binary (decision 0098).
// The dev proxy every other spec runs behind has no TLS or token.
const binary = process.env.SERVEF_BROWSER_BINARY ?? ""
test.skip(binary === "", "needs the built servef binary")

test("the browser link signs in once, and the server counts the page as a UI", async ({ page, browser }) => {
  const home = mkdtempSync(path.join(tmpdir(), "sfw-"))
  const run = path.join(home, "run")
  const project = path.join(home, "project")
  mkdirSync(run, { mode: 0o700 })
  mkdirSync(project)
  writeFileSync(path.join(project, "guide.md"), "# Remote guide\n")
  const env = {
    ...process.env,
    HOME: home,
    XDG_CONFIG_HOME: path.join(home, "config"),
    XDG_CACHE_HOME: path.join(home, "cache"),
    XDG_DATA_HOME: path.join(home, "data"),
    XDG_RUNTIME_DIR: run,
    NO_COLOR: "1",
  }
  const servef = (...args: string[]) => execFileSync(binary, args, { env, encoding: "utf8" })
  const clients = () => {
    const status: unknown = JSON.parse(servef("remote", "status", "--json"))
    return typeof status === "object" && status !== null && "clients" in status ? status.clients : null
  }
  try {
    const printed = servef(project, "--remote", "--web", "--host", "127.0.0.1", "--port", "0")
    const link = /web:\s+(https:\S+)/.exec(printed)?.[1]
    expect(link, printed).toBeTruthy()
    const signIn = new URL(link ?? "")

    // Anyone without the token gets nothing.
    const stranger = await browser.newContext({ ignoreHTTPSErrors: true })
    const refused = await stranger.request.get(`${signIn.origin}/api/projects`)
    expect(refused.status()).toBe(401)
    const strangerPage = await stranger.newPage()
    const answered = await strangerPage.goto(`${signIn.origin}/`)
    expect(answered?.status()).toBe(401)
    await stranger.close()

    const context = await browser.newContext({ ignoreHTTPSErrors: true })
    const signedIn = await context.newPage()
    await signedIn.goto(signIn.href)
    // The token leaves the address bar, and history, for an HttpOnly cookie.
    expect(signedIn.url()).not.toContain("token")
    const cookies = await context.cookies()
    expect(cookies.map((cookie) => [cookie.name, cookie.httpOnly, cookie.secure, cookie.sameSite])).toEqual([["servef_remote", true, true, "Strict"]])
    expect(await signedIn.evaluate(() => document.cookie)).toBe("")

    await signedIn.goto(`${signIn.origin}/view?${new URLSearchParams({ project, path: "guide.md" }).toString()}`)
    await expect(signedIn.getByRole("textbox", { name: "Markdown source" })).toContainText("# Remote guide")
    await expect.poll(clients).toBe(1)

    await signedIn.close()
    await expect.poll(clients).toBe(0)
    await context.close()
  } finally {
    await page.close()
    servef("daemon", "stop")
    rmSync(home, { recursive: true, force: true })
  }
})
