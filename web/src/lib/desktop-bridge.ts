import { isRecord } from "../api/http.ts"

declare global {
  interface Window {
    // servefDesktop is the desktop app's preload bridge; browsers have none.
    servefDesktop?: unknown
  }
}

// DesktopBridge is the desktop app's preload bridge, version 1: openFolder
// asks the app to choose a folder and open it as a project; showAppMenu,
// present outside macOS, pops the app menu at a window point (decision 0075).
export interface DesktopBridge {
  version: 1
  openFolder(): void
  showAppMenu: ((x: number, y: number) => void) | null
}

// readDesktopBridge accepts only a version 1 bridge with its functions; any
// other value means no bridge. A missing or non-function showAppMenu is null.
export function readDesktopBridge(value: unknown): DesktopBridge | null {
  if (!isRecord(value) || value.version !== 1) return null
  const openFolder = value.openFolder
  if (typeof openFolder !== "function") return null
  const showAppMenu = value.showAppMenu
  return {
    version: 1,
    openFolder() {
      openFolder.call(value)
    },
    showAppMenu: typeof showAppMenu === "function"
      ? (x, y) => {
        showAppMenu.call(value, x, y)
      }
      : null,
  }
}

// desktopBridge is the bridge the desktop app exposes, or null in browsers.
export function desktopBridge(): DesktopBridge | null {
  return typeof window === "undefined" ? null : readDesktopBridge(window.servefDesktop)
}
