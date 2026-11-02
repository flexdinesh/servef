// Native dialogs: first-contact http server confirmation (decision 0075) and
// reports (decision 0072). SERVEF_DESKTOP_AUTO_CONFIRM=1 (unpackaged only)
// confirms or skips the report, and appends the prompt or report as a JSON
// line to SERVEF_DESKTOP_DIALOG_LOG.
import { dialog, type BrowserWindow, type MessageBoxOptions } from "electron"
import { appendFileSync } from "node:fs"

// confirmServer asks before a link opens a server on another computer; true
// to open it. label is <host>:<port>.
export async function confirmServer(parent: BrowserWindow | null, label: string, options: { packaged: boolean; env: NodeJS.ProcessEnv }): Promise<boolean> {
  if (autoConfirm(options)) {
    logDialog(options.env, { server: label })
    return true
  }
  const box: MessageBoxOptions = {
    type: "question",
    message: `Open the servef server at ${label}?`,
    detail: `A link asked servef to open the server at ${label}. It is not on this computer. Open only servers you trust.`,
    buttons: ["Open", "Cancel"],
    cancelId: 1,
    defaultId: 1,
    noLink: true,
  }
  const { response } = parent === null ? await dialog.showMessageBox(box) : await dialog.showMessageBox(parent, box)
  return response === 0
}

// showReport shows an error message box without blocking the main process.
export async function showReport(parent: BrowserWindow | null, report: { message: string; detail: string }, options: { packaged: boolean; env: NodeJS.ProcessEnv }): Promise<void> {
  if (autoConfirm(options)) {
    logDialog(options.env, { report })
    return
  }
  const box: MessageBoxOptions = { type: "error", message: report.message, detail: report.detail }
  if (parent === null) await dialog.showMessageBox(box)
  else await dialog.showMessageBox(parent, box)
}

function autoConfirm(options: { packaged: boolean; env: NodeJS.ProcessEnv }): boolean {
  return !options.packaged && options.env.SERVEF_DESKTOP_AUTO_CONFIRM === "1"
}

function logDialog(env: NodeJS.ProcessEnv, entry: unknown): void {
  const log = env.SERVEF_DESKTOP_DIALOG_LOG
  if (log === undefined || log === "") return
  try {
    appendFileSync(log, JSON.stringify(entry) + "\n")
  } catch (error) {
    console.error("servef: dialog log:", error)
  }
}
