// Preload bridge v2 (decisions 0067, 0070, 0075, 0081): window.servefDesktop =
// { version: 2, openFolder(), showAppMenu?(x, y), revealPath(project, path),
// showContextMenu(items, x, y) }, exposed to servef-app://local frames only.
// showAppMenu exists outside macOS, where the hidden title bar hides the menu
// bar. The preload also turns folders dropped on the window into
// DROP_PATHS_CHANNEL messages. The UI feature-detects every method.
export const BRIDGE_VERSION = 2
export const OPEN_FOLDER_CHANNEL = "servef:open-folder"
export const APP_MENU_CHANNEL = "servef:app-menu"
// REVEAL_CHANNEL and CONTEXT_MENU_CHANNEL are invoke channels (they answer).
export const REVEAL_CHANNEL = "servef:reveal"
export const CONTEXT_MENU_CHANNEL = "servef:context-menu"
export const DROP_PATHS_CHANNEL = "servef:drop-paths"
export const APP_SCHEME = "servef-app"
// BRIDGE_HOST is the only origin host with a bridge (the local daemon). The
// preload bundles this module, so it imports nothing.
const BRIDGE_HOST = "local"

// isLocalFrame reports a frame URL on servef-app://local: the only origin
// that gets the bridge and whose bridge messages main honors. Remote (ssh),
// CLI, http, and app page frames get none.
export function isLocalFrame(url: string): boolean {
  try {
    const parsed = new URL(url)
    return parsed.protocol === `${APP_SCHEME}:` && parsed.host === BRIDGE_HOST && parsed.username === "" && parsed.password === ""
  } catch {
    return false
  }
}
