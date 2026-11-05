import { isRecord } from "../api/http.ts"

declare global {
  interface Window {
    // servefDesktop is the desktop app's preload bridge; browsers have none.
    servefDesktop?: unknown
  }
}

// NativeMenuItem describes one native context menu row (decision 0081): an
// action the UI runs when its id comes back, or a separator. accelerator is a
// display hint (e.g. "CmdOrCtrl+W"); nothing registers it.
export type NativeMenuItem =
  | { id: string; label: string; enabled?: boolean; accelerator?: string }
  | { separator: true }

// DesktopBridge is the desktop app's preload bridge (version 1 or later).
// openFolder asks the app to choose a folder and open it as a project. Each
// later method is feature-detected and null when absent: showAppMenu (outside
// macOS, decision 0075) pops the app menu at a window point; revealPath shows
// a project file in the OS file manager; showContextMenu pops a native menu
// and resolves the chosen id, or null when dismissed (decision 0081).
export interface DesktopBridge {
  version: number
  openFolder(): void
  showAppMenu: ((x: number, y: number) => void) | null
  revealPath: ((project: string, path: string) => void) | null
  showContextMenu: ((items: readonly NativeMenuItem[], x: number, y: number) => Promise<string | null>) | null
}

// readDesktopBridge accepts a bridge of version 1 or later with openFolder;
// any other value means no bridge. Methods that are missing or not functions
// are null.
export function readDesktopBridge(value: unknown): DesktopBridge | null {
  if (!isRecord(value) || typeof value.version !== "number" || !Number.isInteger(value.version) || value.version < 1) return null
  const { openFolder, showAppMenu, revealPath, showContextMenu } = value
  if (typeof openFolder !== "function") return null
  return {
    version: value.version,
    openFolder() {
      openFolder.call(value)
    },
    showAppMenu: typeof showAppMenu === "function"
      ? (x, y) => {
        showAppMenu.call(value, x, y)
      }
      : null,
    revealPath: typeof revealPath === "function"
      ? (project, path) => {
        // The app reports refusals itself.
        Promise.resolve(revealPath.call(value, project, path)).catch(() => undefined)
      }
      : null,
    showContextMenu: typeof showContextMenu === "function"
      ? async (items, x, y) => {
        try {
          const chosen: unknown = await showContextMenu.call(value, items, x, y)
          return typeof chosen === "string" ? chosen : null
        } catch {
          return null
        }
      }
      : null,
  }
}

// desktopBridge is the bridge the desktop app exposes, or null in browsers.
export function desktopBridge(): DesktopBridge | null {
  return typeof window === "undefined" ? null : readDesktopBridge(window.servefDesktop)
}
