import { isRecord } from "../api/http.ts"

declare global {
  interface Window {
    // servefDesktop is the desktop app's preload bridge; browsers have none.
    servefDesktop?: unknown
  }
}

// DesktopBridge is the desktop app's preload bridge, version 1: openFolder
// asks the app to choose a folder and open it as a project.
export interface DesktopBridge {
  version: 1
  openFolder(): void
}

// readDesktopBridge accepts only a version 1 bridge with its functions; any
// other value means no bridge.
export function readDesktopBridge(value: unknown): DesktopBridge | null {
  if (!isRecord(value) || value.version !== 1) return null
  const openFolder = value.openFolder
  if (typeof openFolder !== "function") return null
  return {
    version: 1,
    openFolder() {
      openFolder.call(value)
    },
  }
}

// desktopBridge is the bridge the desktop app exposes, or null in browsers.
export function desktopBridge(): DesktopBridge | null {
  return typeof window === "undefined" ? null : readDesktopBridge(window.servefDesktop)
}
