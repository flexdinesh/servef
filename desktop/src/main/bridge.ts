// Preload bridge v1 (decision 0067): window.servefDesktop = { version: 1, openFolder() }.
export const BRIDGE_VERSION = 1
export const OPEN_FOLDER_CHANNEL = "servef:open-folder"
export const APP_SCHEME = "servef-app"

// isAppFrame reports a sender frame URL served by the servef-app:// proxy.
export function isAppFrame(url: string): boolean {
  try {
    return new URL(url).protocol === `${APP_SCHEME}:`
  } catch {
    return false
  }
}
