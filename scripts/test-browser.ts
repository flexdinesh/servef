import { spawnSync } from "node:child_process"
import { mkdtempSync, rmSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"

import { requestedPort, serverPort, startProcess, stopProcess, type ManagedProcess } from "./browser-process.ts"

// Chromium aborts in-flight requests when Docker changes host interfaces.
// Re-enter this runner inside a loopback-only namespace before starting any
// servers. Hosts without unprivileged namespaces still use OS-assigned ports.
const isolate = ["--user", "--map-root-user", "--net", "sh", "-c", 'ip link set lo up && exec "$@"', "sh"]
if (process.platform === "linux" && process.env.SERVEF_BROWSER_ISOLATED !== "1"
  && spawnSync("unshare", [...isolate, "true"], { stdio: "ignore" }).status === 0) {
  const wrapper = startProcess("unshare", [...isolate, process.execPath, ...process.argv.slice(1)], {
    ...process.env, SERVEF_BROWSER_ISOLATED: "1",
  })
  const forward = () => { wrapper.child.kill("SIGTERM") }
  process.once("SIGINT", forward)
  process.once("SIGTERM", forward)
  process.exit(await wrapper.exited)
}

const root = mkdtempSync(path.join(tmpdir(), "servef-browser-"))
const processes: ManagedProcess[] = []
let interrupted = false
function interrupt() {
  interrupted = true
  for (const proc of processes) void stopProcess(proc)
}
process.once("SIGINT", interrupt)
process.once("SIGTERM", interrupt)

const env = {
  ...process.env,
  NO_COLOR: "1",
  FORCE_COLOR: "0",
  SERVEF_BROWSER_GIT_ROOT: path.join(root, "git"),
  SERVEF_BROWSER_LIVE_ROOT: path.join(root, "live"),
}

function launch(command: string, args: string[], overrides: NodeJS.ProcessEnv = {}, capture = false) {
  if (interrupted) throw new Error("Browser run interrupted")
  const proc = startProcess(command, args, { ...env, ...overrides }, capture)
  processes.push(proc)
  return proc
}

async function run(command: string, args: string[]) {
  const code = await launch(command, args).exited
  if (code !== 0) throw new Error(`${command} exited with status ${code}`)
}

try {
  const base = requestedPort(process.env.PORT, "PORT", 65532)
  const web = requestedPort(process.env.SERVEF_BROWSER_WEB_PORT, "SERVEF_BROWSER_WEB_PORT")
  const binary = path.join(root, process.platform === "win32" ? "servef.exe" : "servef")
  await run("go", ["build", "-o", binary, "."])
  await run(process.execPath, ["tests/browser/git-fixture.ts"])
  await run(process.execPath, ["tests/browser/live-fixture.ts"])

  async function serve(directory: string, port: number) {
    return serverPort(launch(binary, [directory, "--host", "127.0.0.1", "--port", String(port), "--no-open"], {
      XDG_CACHE_HOME: path.join(root, "cache"),
    }, true))
  }

  const api = await serve("testdata/markdown", base)
  const git = await serve(env.SERVEF_BROWSER_GIT_ROOT, base === 0 ? 0 : base + 2)
  const live = await serve(env.SERVEF_BROWSER_LIVE_ROOT, base === 0 ? 0 : base + 3)
  const vite = await serverPort(launch(process.execPath, [
    "web/node_modules/vite/bin/vite.js", "--configLoader", "native", "--host", "127.0.0.1", "--port", String(web), "--strictPort",
    "--config", "web/vite.config.ts",
  ], { SERVEF_API_TARGET: `http://127.0.0.1:${api}` }, true))

  const result = await launch("playwright", ["test", ...process.argv.slice(2)], {
    SERVEF_BROWSER_API_PORT: String(api),
    SERVEF_BROWSER_GIT_PORT: String(git),
    SERVEF_BROWSER_LIVE_PORT: String(live),
    SERVEF_BROWSER_WEB_PORT: String(vite),
  }).exited
  process.exitCode = interrupted ? 130 : result
} catch (error) {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = interrupted ? 130 : 1
} finally {
  await Promise.all(processes.map(stopProcess))
  rmSync(root, { recursive: true, force: true })
  process.removeListener("SIGINT", interrupt)
  process.removeListener("SIGTERM", interrupt)
}
