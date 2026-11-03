// Open at Login (decision 0072): macOS login items through Electron, Linux
// XDG autostart. Registration changes only on an explicit menu click.
import { app, dialog, shell, type BrowserWindow, type MessageBoxOptions } from "electron"
import { autostartPlan, createXdgAutostart } from "./autostart.ts"

export interface LoginItem {
  enabled(): boolean
  // set registers or unregisters; "approval" when macOS waits for the user
  // in System Settings. Throws on failure.
  set(on: boolean): Promise<"ok" | "approval">
  // manual tells users where to change it by hand.
  manual: string
}

const MAC_LOGIN_ITEMS = "System Settings > General > Login Items"
const MAC_LOGIN_ITEMS_URL = "x-apple.systempreferences:com.apple.LoginItems-Settings.extension"

// createLoginItem is null where the app offers no toggle: other platforms,
// unpackaged macOS, and unpackaged Linux without SERVEF_DESKTOP_AUTOSTART_DIR,
// so development never registers a dev Electron.
export function createLoginItem(input: { platform: NodeJS.Platform; packaged: boolean; env: NodeJS.ProcessEnv; home: string; execPath: string; appPath: string }): LoginItem | null {
  if (input.platform === "darwin") {
    if (!input.packaged) return null
    return {
      enabled: () => app.getLoginItemSettings().openAtLogin,
      set: async (on) => {
        app.setLoginItemSettings({ openAtLogin: on })
        const settings = app.getLoginItemSettings()
        if (settings.status === "requires-approval") return "approval"
        if (settings.openAtLogin !== on) throw new Error(`macOS reports login item status ${settings.status}.`)
        return "ok"
      },
      manual: MAC_LOGIN_ITEMS,
    }
  }
  if (input.platform !== "linux") return null
  const plan = autostartPlan(input)
  if (plan === null) return null
  const autostart = createXdgAutostart(plan)
  return {
    enabled: autostart.enabled,
    set: async (on) => {
      autostart.set(on)
      return "ok"
    },
    manual: `the autostart entry ${autostart.file}`,
  }
}

// showLoginApproval explains macOS approval and offers to open Login Items.
export async function showLoginApproval(parent: BrowserWindow | null): Promise<void> {
  const box: MessageBoxOptions = {
    type: "info",
    message: "Allow servef in System Settings",
    detail: `macOS needs your approval in ${MAC_LOGIN_ITEMS}.`,
    buttons: ["Open Login Items", "Close"],
    defaultId: 0,
    cancelId: 1,
    noLink: true,
  }
  const { response } = parent === null ? await dialog.showMessageBox(box) : await dialog.showMessageBox(parent, box)
  if (response === 0) await shell.openExternal(MAC_LOGIN_ITEMS_URL)
}
