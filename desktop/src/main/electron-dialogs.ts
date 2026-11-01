// Native dialogs for first-contact ssh confirmation (decision 0070).
// SERVEF_DESKTOP_AUTO_CONFIRM=1 (unpackaged only) picks the first choice and
// appends the prompt as a JSON line to SERVEF_DESKTOP_DIALOG_LOG.
import { dialog, type BrowserWindow, type MessageBoxOptions } from "electron"
import { appendFileSync } from "node:fs"

export interface ConnectChoice {
  // label is the button text (the destination).
  label: string
  // detail lists `ssh -G` user, hostname, and port when known.
  detail: string
}

export interface ConnectPrompt {
  // host is the link's host segment.
  host: string
  choices: ConnectChoice[]
}

// MAX_CHOICES bounds the destination buttons; Cancel comes after them.
const MAX_CHOICES = 4
const TRUST = "A link asked servef to connect. servef runs servef on that host with your SSH configuration. Connect only to hosts you trust."

// confirmConnect returns the chosen index, or null on Cancel.
export async function confirmConnect(parent: BrowserWindow | null, prompt: ConnectPrompt, options: { packaged: boolean; env: NodeJS.ProcessEnv }): Promise<number | null> {
  const choices = prompt.choices.slice(0, MAX_CHOICES)
  if (choices.length === 0) return null
  if (!options.packaged && options.env.SERVEF_DESKTOP_AUTO_CONFIRM === "1") {
    const log = options.env.SERVEF_DESKTOP_DIALOG_LOG
    if (log !== undefined && log !== "") {
      try {
        appendFileSync(log, JSON.stringify({ host: prompt.host, choices }) + "\n")
      } catch (error) {
        console.error("servef: dialog log:", error)
      }
    }
    return 0
  }
  const [only] = choices
  const destination = choices.length === 1 && only !== undefined ? only.label : prompt.host
  const details = choices.map((choice) => choice.detail).filter((detail) => detail !== "")
  const box: MessageBoxOptions = {
    type: "question",
    message: `Connect to ${destination} over SSH?`,
    detail: [...details, TRUST].join("\n\n"),
    buttons: [...choices.map((choice) => choice.label), "Cancel"],
    cancelId: choices.length,
    defaultId: choices.length,
    noLink: true,
  }
  const { response } = parent === null ? await dialog.showMessageBox(box) : await dialog.showMessageBox(parent, box)
  return response >= 0 && response < choices.length ? response : null
}
