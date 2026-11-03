import { existsSync, readFileSync, realpathSync, symlinkSync, writeFileSync } from "node:fs"
import path from "node:path"

import { expect, test, type ElectronApplication, type Page } from "@playwright/test"

import { RECENT_FILE } from "../../src/main/recent-folders.ts"
import {
  appURL, createRepository, createSession, ensureDaemon, keepDiagnostics, launchApp, openRoot, removeSession, schemeURL, stopDaemon, type Session,
} from "./harness.ts"

// Bridge v2 (decision 0081): native context menus, Reveal in File Manager,
// recent folders, and folders dropped on a local window. Menu popups and
// shell.showItemInFolder are recorded in the main process instead of shown.

test.describe.configure({ mode: "serial" })

let session: Session | undefined
let app: ElectronApplication | undefined
let alpha = ""
let beta = ""
let outside = ""
let dialogLog = ""
let failed = false

interface RecordedItem {
  id: string
  label: string
  type: string
  enabled: boolean
}

function current(): { session: Session; app: ElectronApplication } {
  if (!session || !app) throw new Error("the app did not start")
  return { session, app }
}

function landing(project: string): string {
  return `servef-app://local/?project=${encodeURIComponent(project)}`
}

function reports(): string[] {
  if (!existsSync(dialogLog)) return []
  return readFileSync(dialogLog, "utf8").split("\n").filter((line) => line.trim() !== "")
}

function recentFile(): string[] {
  const file = path.join(current().session.userData, RECENT_FILE)
  if (!existsSync(file)) return []
  const value: unknown = JSON.parse(readFileSync(file, "utf8"))
  if (typeof value !== "object" || value === null || !("folders" in value) || !Array.isArray(value.folders)) return []
  return value.folders.map((folder: unknown) => String(folder))
}

// record replaces Menu.prototype.popup and shell.showItemInFolder with
// recorders; choose() sets the item id the next popup clicks.
async function record(): Promise<void> {
  await current().app.evaluate(({ Menu, shell }) => {
    const menus: RecordedItem[][] = []
    const revealed: string[] = []
    Reflect.set(globalThis, "servefSmokeMenus", menus)
    Reflect.set(globalThis, "servefSmokeRevealed", revealed)
    Reflect.set(globalThis, "servefSmokeChoice", null)
    Object.defineProperty(Menu.prototype, "popup", {
      configurable: true,
      writable: true,
      value(this: Electron.Menu, options?: Electron.PopupOptions) {
        menus.push(this.items.map((item) => ({ id: item.id, label: item.label, type: item.type, enabled: item.enabled })))
        const choice: unknown = Reflect.get(globalThis, "servefSmokeChoice")
        this.items.find((item) => item.id === choice)?.click()
        options?.callback?.()
      },
    })
    Object.defineProperty(shell, "showItemInFolder", {
      configurable: true,
      writable: true,
      value: (item: string) => { revealed.push(item) },
    })
  })
}

async function choose(id: string | null): Promise<void> {
  await current().app.evaluate((_electron, choice) => { Reflect.set(globalThis, "servefSmokeChoice", choice) }, id)
}

function menus(): Promise<RecordedItem[][]> {
  return current().app.evaluate(() => {
    const value: unknown = Reflect.get(globalThis, "servefSmokeMenus")
    return Array.isArray(value) ? value.filter((menu): menu is RecordedItem[] => Array.isArray(menu)) : []
  })
}

function revealed(): Promise<string[]> {
  return current().app.evaluate(() => {
    const value: unknown = Reflect.get(globalThis, "servefSmokeRevealed")
    return Array.isArray(value) ? value.filter((item) => typeof item === "string") : []
  })
}

function menuLabels(menu: RecordedItem[] | undefined): string[] {
  return (menu ?? []).map((item) => item.type === "separator" ? "-" : `${item.label}${item.enabled ? "" : " (disabled)"}`)
}

async function firstWindow(): Promise<Page> {
  return current().app.firstWindow()
}

test.beforeAll(async () => {
  const created = createSession()
  session = created
  dialogLog = path.join(created.dir, "dialogs.jsonl")
  try {
    await ensureDaemon(created)
    alpha = await openRoot(created, createRepository(path.join(created.projects, "alpha"), "Alpha"))
    beta = await openRoot(created, createRepository(path.join(created.projects, "beta"), "Beta"))
    outside = path.join(created.dir, "outside.txt")
    writeFileSync(outside, "not served\n")
    symlinkSync(outside, path.join(alpha, "escape.txt"))
    app = await launchApp(created, schemeURL(alpha, "README.md"), { SERVEF_DESKTOP_AUTO_CONFIRM: "1", SERVEF_DESKTOP_DIALOG_LOG: dialogLog })
    await record()
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

test("local windows get bridge v2", async () => {
  const page = await firstWindow()
  await expect(page.getByRole("main").getByText("The Alpha fixture README.")).toBeVisible()
  expect(await page.evaluate(() => {
    const bridge: unknown = Reflect.get(window, "servefDesktop")
    if (typeof bridge !== "object" || bridge === null) return null
    return [Reflect.get(bridge, "version"), typeof Reflect.get(bridge, "revealPath"), typeof Reflect.get(bridge, "showContextMenu")]
  })).toEqual([2, "function", "function"])
})

test("a tab's native menu reveals its file", async () => {
  const page = await firstWindow()
  await choose("reveal")
  await page.getByRole("tab", { name: "README.md" }).click({ button: "right" })
  await expect.poll(revealed).toEqual([path.join(alpha, "README.md")])
  expect(menuLabels((await menus()).at(-1))).toEqual([
    "Close", "Close Others (disabled)", "Close to the Right (disabled)", "-", "Close All", "-", "Reveal in File Manager",
  ])
  // The page's own menu stays closed.
  await expect(page.getByRole("menu")).toHaveCount(0)
})

test("a tree row's native menu reveals its folder", async () => {
  const page = await firstWindow()
  await choose("reveal")
  await page.locator('[role="treeitem"][data-item-path="src/"]').click({ button: "right" })
  await expect.poll(revealed).toEqual([path.join(alpha, "README.md"), path.join(alpha, "src")])
  expect(menuLabels((await menus()).at(-1))).toEqual(["Reveal in File Manager", "-", "Copy Path", "Copy Relative Path"])
})

test("dismissing a native menu runs nothing", async () => {
  const page = await firstWindow()
  await choose(null)
  const before = (await menus()).length
  await page.getByRole("tab", { name: "README.md" }).click({ button: "right" })
  await expect.poll(async () => (await menus()).length).toBe(before + 1)
  await expect(page.getByRole("tab", { name: "README.md" })).toBeVisible()
  expect(await revealed()).toHaveLength(2)
})

test("main refuses reveals outside the project and malformed bridge calls", async () => {
  const page = await firstWindow()
  const answers = await page.evaluate(async ({ alpha, beta }) => {
    const bridge: unknown = Reflect.get(window, "servefDesktop")
    if (typeof bridge !== "object" || bridge === null) throw new Error("no bridge")
    const reveal = (project: unknown, file: unknown): Promise<unknown> => Reflect.apply(Reflect.get(bridge, "revealPath"), bridge, [project, file])
    const menu = (items: unknown, x: unknown, y: unknown): Promise<unknown> => Reflect.apply(Reflect.get(bridge, "showContextMenu"), bridge, [items, x, y])
    return {
      parent: await reveal(alpha, `../${beta.split("/").at(-1)}/README.md`),
      absolute: await reveal(alpha, "/etc/passwd"),
      relativeProject: await reveal("alpha", "README.md"),
      symlink: await reveal(alpha, "escape.txt"),
      missing: await reveal(alpha, "gone.md"),
      unserved: await reveal("/", "etc/passwd"),
      badId: await menu([{ id: "bad id", label: "X" }], 1, 1),
      outsidePoint: await menu([{ id: "ok", label: "OK" }], -5, 1),
      notItems: await menu("items", 1, 1),
    }
  }, { alpha, beta })
  expect(answers).toEqual({
    parent: false, absolute: false, relativeProject: false, symlink: false, missing: false, unserved: false,
    badId: null, outsidePoint: null, notItems: null,
  })
  expect(await revealed()).toHaveLength(2)
  // Malformed requests are dropped silently; refused paths are reported.
  await expect.poll(() => reports().length).toBe(3)
  const text = reports().join("\n")
  expect(text).toContain("escape.txt: The path leads outside its project.")
  expect(text).toContain("gone.md: The file no longer exists.")
  expect(text).toContain("etc/passwd: The project is not in a folder servef serves.")
})

test("projects local windows show become recent folders in File > Open Recent", async () => {
  const { app } = current()
  const page = await firstWindow()
  await expect.poll(recentFile).toEqual([realpathSync(alpha)])
  await page.evaluate((target) => { location.assign(target) }, appURL(beta, "README.md"))
  await expect.poll(recentFile).toEqual([realpathSync(beta), realpathSync(alpha)])
  const labels = (): Promise<string[]> => app.evaluate(({ Menu }) =>
    Menu.getApplicationMenu()?.getMenuItemById("open-recent")?.submenu?.items.map((item) => item.type === "separator" ? "-" : item.label) ?? [])
  await expect.poll(labels).toEqual([realpathSync(beta), realpathSync(alpha), "-", "Clear Recently Opened"])

  // Choosing a recent folder opens it through the folder opener.
  const opened = app.waitForEvent("window")
  await app.evaluate(({ Menu }) => { Menu.getApplicationMenu()?.getMenuItemById("open-recent-1")?.click() })
  const second = await opened
  await second.waitForURL(landing(realpathSync(alpha)))

  await app.evaluate(({ Menu }) => { Menu.getApplicationMenu()?.getMenuItemById("clear-recent")?.click() })
  await expect.poll(labels).toEqual(["No Recent Folders", "-", "Clear Recently Opened"])
  expect(recentFile()).toEqual([])
})

test("a dropped file goes to the folder opener and is reported", async () => {
  const page = await firstWindow()
  const before = reports().length
  const dropped = path.join(beta, "README.md")
  await page.evaluate(() => {
    const input = document.createElement("input")
    input.type = "file"
    input.id = "smoke-drop"
    input.hidden = true
    document.body.append(input)
  })
  await page.locator("#smoke-drop").setInputFiles(dropped)
  await page.evaluate(() => {
    const input = document.querySelector("#smoke-drop")
    const file = input instanceof HTMLInputElement ? input.files?.[0] : undefined
    if (file === undefined) throw new Error("no file")
    const transfer = new DataTransfer()
    transfer.items.add(file)
    document.body.dispatchEvent(new DragEvent("dragover", { dataTransfer: transfer, bubbles: true, cancelable: true }))
    document.body.dispatchEvent(new DragEvent("drop", { dataTransfer: transfer, bubbles: true, cancelable: true }))
    input?.remove()
  })
  await expect.poll(() => reports().length).toBe(before + 1)
  const line = reports().at(-1) ?? ""
  expect(line).toContain("Could not open folder")
  expect(line).toContain(dropped)
})
