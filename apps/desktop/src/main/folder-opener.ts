// One opener for every folder source (decision 0072): the Open Folder dialog,
// macOS open-file, `--open-path` argv, and second instances. Each folder is
// opened through the local daemon (adding a root as `servef [path]` does) and
// its project routed to a window.
import type { OpenOutcome } from "./daemon.ts"
import { MAX_OPEN_PATHS, projectLanding } from "./open-paths.ts"

export interface FolderOpenerDeps {
  // socket is the ready local daemon's socket, else null.
  socket(): Promise<string | null>
  // open posts /_daemon/open for path.
  open(socket: string, path: string): Promise<OpenOutcome>
  route(url: string): void
  windowCount(): number
  // origin is servef-app://local.
  origin: string
  // report shows a message without blocking the main process.
  report(message: string, detail: string): void
}

export interface OpenFoldersResult {
  projects: string[]
  failed: { path: string; error: string }[]
  skipped: number
}

export const DAEMON_NOT_RUNNING = "The servef daemon is not running."

export function createFolderOpener(deps: FolderOpenerDeps): { open(paths: readonly string[]): Promise<OpenFoldersResult> } {
  // ensureWindow never leaves the app running with no window: Linux has no tray.
  const ensureWindow = (): void => {
    if (deps.windowCount() === 0) deps.route(`${deps.origin}/`)
  }

  const open = async (paths: readonly string[]): Promise<OpenFoldersResult> => {
    const chosen = paths.slice(0, MAX_OPEN_PATHS)
    const result: OpenFoldersResult = { projects: [], failed: [], skipped: paths.length - chosen.length }
    if (chosen.length === 0) return result
    if (result.skipped > 0) {
      deps.report(`servef opens up to ${MAX_OPEN_PATHS} folders at a time`, `Skipped ${result.skipped} of ${paths.length}:\n${paths.slice(MAX_OPEN_PATHS).join("\n")}`)
    }
    const socket = await deps.socket()
    if (socket === null) {
      result.failed = chosen.map((path) => ({ path, error: DAEMON_NOT_RUNNING }))
      deps.report("Could not open folder", DAEMON_NOT_RUNNING)
      ensureWindow()
      return result
    }
    // Sequential: the daemon saves roots one request at a time, so order stays deterministic.
    for (const path of chosen) {
      const outcome = await deps.open(socket, path)
      if (!outcome.ok) {
        result.failed.push({ path, error: outcome.error })
        continue
      }
      const project = outcome.result.project
      if (result.projects.includes(project)) continue
      result.projects.push(project)
      deps.route(projectLanding(deps.origin, project))
    }
    if (result.failed.length > 0) {
      const title = result.failed.length === 1 ? "Could not open folder" : "Could not open folders"
      deps.report(title, result.failed.map((failure) => `${failure.path}: ${failure.error}`).join("\n"))
    }
    ensureWindow()
    return result
  }
  return { open }
}
