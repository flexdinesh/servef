// Preload bridge v1 (decisions 0067, 0070): window.servefDesktop = { version: 1, openFolder() },
// exposed to servef-app://local frames only.
export const BRIDGE_VERSION = 1
export const OPEN_FOLDER_CHANNEL = "servef:open-folder"
export const APP_SCHEME = "servef-app"
// BRIDGE_HOST is the only origin host with a bridge (the local daemon). The
// preload bundles this module, so it imports nothing.
const BRIDGE_HOST = "local"

// isLocalFrame reports a frame URL on servef-app://local: the only origin
// that gets the bridge and whose OPEN_FOLDER_CHANNEL messages main honors.
// Remote (ssh) and app page frames get none.
export function isLocalFrame(url: string): boolean {
  try {
    const parsed = new URL(url)
    return parsed.protocol === `${APP_SCHEME}:` && parsed.host === BRIDGE_HOST && parsed.username === "" && parsed.password === ""
  } catch {
    return false
  }
}
