import { existsSync, readdirSync, readFileSync } from "node:fs"
import path from "node:path"
import { parseArgs } from "node:util"

import type { ElectronApplication, Page } from "@playwright/test"

import {
  appURL, createRepository, createSession, daemonStatus, desktopDir, ensureDaemon, keepDiagnostics, launchApp, launchSecond, openRoot,
  readmeLine, removeSession, schemeURL, stopDaemon, type Session,
} from "../test/smoke/harness.ts"

// Memory baseline for decision 0067 (no gate). Scenario: one window on one
// project, idle; a second window on a second project, idle; one navigation.
// Each checkpoint prints Electron per-process, daemon, and language-server tree
// PSS (RSS for reference) from /proc. Linux only; needs a display. Under a
// virtual Wayland compositor:
//   dbus-run-session -- kwin_wayland --virtual --width 1280 --height 800 --exit-with-session \
//     'mise run measure:desktop -- --display-args=--ozone-platform=wayland'

const { values } = parseArgs({
  options: {
    "display-args": { type: "string", default: process.env.SERVEF_DESKTOP_ELECTRON_ARGS ?? "" },
    idle: { type: "string", default: "30" },
    "keep-logs": { type: "string" },
  },
})

interface Memory {
  pss: number
  rss: number
}

interface ProcessRow extends Memory {
  label: string
  pid: number
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

// smaps_rollup values are kB; a vanished process counts as zero.
function memoryOf(pid: number): Memory {
  try {
    const text = readFileSync(`/proc/${pid}/smaps_rollup`, "utf8")
    const field = (name: string) => Number(new RegExp(`^${name}:\\s+(\\d+) kB`, "m").exec(text)?.[1] ?? 0)
    return { pss: field("Pss"), rss: field("Rss") }
  } catch {
    return { pss: 0, rss: 0 }
  }
}

function children(pid: number): number[] {
  const tasks = `/proc/${pid}/task`
  if (!existsSync(tasks)) return []
  const found: number[] = []
  for (const task of readdirSync(tasks)) {
    try {
      for (const child of readFileSync(path.join(tasks, task, "children"), "utf8").split(/\s+/)) {
        if (child !== "") found.push(Number(child))
      }
    } catch {
      // The task exited.
    }
  }
  return found
}

function descendants(pid: number): number[] {
  const all: number[] = []
  for (let queue = children(pid), next = queue.shift(); next !== undefined; next = queue.shift()) {
    all.push(next)
    queue.push(...children(next))
  }
  return all
}

function commandOf(pid: number): string {
  try {
    return readFileSync(`/proc/${pid}/comm`, "utf8").trim()
  } catch {
    return "?"
  }
}

async function electronRows(app: ElectronApplication): Promise<ProcessRow[]> {
  const metrics: unknown = await app.evaluate(({ app: electron }) => electron.getAppMetrics().map((metric) => ({ pid: metric.pid, type: metric.type })))
  if (!Array.isArray(metrics)) throw new Error("app.getAppMetrics returned no list")
  const rows: ProcessRow[] = []
  for (const metric of metrics) {
    if (!isRecord(metric) || typeof metric.pid !== "number" || typeof metric.type !== "string") continue
    rows.push({ label: metric.type, pid: metric.pid, ...memoryOf(metric.pid) })
  }
  return rows
}

function mib(kb: number): string {
  return `${(kb / 1024).toFixed(1).padStart(7)} MiB`
}

function total(rows: readonly Memory[]): Memory {
  return rows.reduce((sum, row) => ({ pss: sum.pss + row.pss, rss: sum.rss + row.rss }), { pss: 0, rss: 0 })
}

function line(label: string, memory: Memory): string {
  return `  ${label.padEnd(28)} PSS ${mib(memory.pss)}   RSS ${mib(memory.rss)}`
}

async function checkpoint(name: string, session: Session, app: ElectronApplication): Promise<void> {
  const electron = await electronRows(app)
  const daemon = await daemonStatus(session)
  const servers = descendants(daemon.pid).map((pid) => ({ label: commandOf(pid), pid, ...memoryOf(pid) }))
  console.log(`\n${name}`)
  for (const row of electron) console.log(line(`electron ${row.label} ${row.pid}`, row))
  console.log(line("electron total", total(electron)))
  console.log(line(`daemon ${daemon.pid}`, memoryOf(daemon.pid)))
  for (const row of servers) console.log(line(`  language server ${row.label} ${row.pid}`, row))
  console.log(line("language servers total", total(servers)))
}

async function idle(seconds: number): Promise<void> {
  console.log(`\n(idle ${seconds} s)`)
  await new Promise((resolve) => setTimeout(resolve, seconds * 1000))
}

async function waitForText(page: Page, text: string): Promise<void> {
  await page.getByRole("main").getByText(text).waitFor({ state: "visible", timeout: 60_000 })
}

if (process.platform !== "linux") {
  console.log("measure:desktop reads /proc and runs on Linux only; on macOS use Activity Monitor's Memory column for Electron Helper processes and servef.")
  process.exit(0)
}

process.env.SERVEF_DESKTOP_ELECTRON_ARGS = values["display-args"]
const seconds = Number(values.idle)
if (!Number.isFinite(seconds) || seconds < 0) throw new Error("--idle must be a number of seconds")

const session = createSession()
let app: ElectronApplication | undefined
try {
  await ensureDaemon(session)
  const alpha = await openRoot(session, createRepository(path.join(session.projects, "alpha"), "Alpha"))
  const beta = await openRoot(session, createRepository(path.join(session.projects, "beta"), "Beta"))
  const launched = await launchApp(session, schemeURL(alpha, "README.md"))
  app = launched
  const first = await launched.firstWindow()
  await waitForText(first, readmeLine("Alpha"))
  await checkpoint("one window, one project", session, launched)
  await idle(seconds)
  await checkpoint("one window, idle", session, launched)

  const opened = launched.waitForEvent("window", { timeout: 60_000 })
  const code = await launchSecond(session, schemeURL(beta, "README.md"))
  if (code !== 0) throw new Error(`second launch exited ${code}`)
  const second = await opened
  await waitForText(second, readmeLine("Beta"))
  await checkpoint("two windows, two projects", session, launched)
  await idle(seconds)
  await checkpoint("two windows, idle", session, launched)

  const target = appURL(beta, "src/main.go")
  await second.evaluate((url) => { location.assign(url) }, target)
  await second.waitForURL(target)
  await second.getByRole("main").locator('[class^="hl-"],[class*=" hl-"]').first().waitFor({ state: "visible", timeout: 60_000 })
  await checkpoint("after one navigation", session, launched)
} catch (error) {
  process.exitCode = 1
  console.error(error instanceof Error ? error.message : error)
  const keep = values["keep-logs"] ?? path.join(desktopDir, "test-results", "measure")
  keepDiagnostics(session, keep)
  console.error(`logs: ${keep}`)
} finally {
  await app?.close().catch(() => undefined)
  await stopDaemon(session)
  removeSession(session)
}
