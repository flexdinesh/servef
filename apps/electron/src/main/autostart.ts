// Linux Open at Login (decision 0072): an XDG autostart entry the app writes
// or removes only when the user clicks the menu checkbox, or moves once from
// the id before the rename (decision 0092).
import { chmodSync, existsSync, mkdirSync, readFileSync, renameSync, unlinkSync, writeFileSync } from "node:fs"
import path from "node:path"

// AUTOSTART_FILE is the package entry's desktop id, so desktops show one app.
export const AUTOSTART_FILE = "servef-electron.desktop"

// LEGACY_AUTOSTART_FILE is the entry's id before the rename (decision 0092).
export const LEGACY_AUTOSTART_FILE = "servef-desktop.desktop"

// LEGACY_ENTRY matches exactly what the app wrote under that id; only Exec varied.
const LEGACY_ENTRY = /^\[Desktop Entry\]\nType=Application\nName=servef\nComment=Open servef at login\nExec=[^\n]*\nIcon=servef-desktop\nTerminal=false\nX-GNOME-Autostart-enabled=true\n$/

export interface AutostartPlan {
  dir: string
  argv: string[]
}

// autostartPlan: packaged → $XDG_CONFIG_HOME/autostart (absolute only, else
// <home>/.config/autostart), argv [APPIMAGE] when it is absolute, else
// [execPath]. Unpackaged → only when SERVEF_DESKTOP_AUTOSTART_DIR is absolute
// (dev and test seam): argv [execPath, appPath]. Else null.
export function autostartPlan(input: { packaged: boolean; env: NodeJS.ProcessEnv; home: string; execPath: string; appPath: string }): AutostartPlan | null {
  const { env } = input
  if (!input.packaged) {
    const dir = env.SERVEF_DESKTOP_AUTOSTART_DIR
    return absolute(dir) ? { dir, argv: [input.execPath, input.appPath] } : null
  }
  const config = env.XDG_CONFIG_HOME
  const dir = path.join(absolute(config) ? config : path.join(input.home, ".config"), "autostart")
  // An AppImage runs from a temporary mount; APPIMAGE names the image file.
  const image = env.APPIMAGE
  return { dir, argv: [absolute(image) ? image : input.execPath] }
}

function absolute(value: string | undefined): value is string {
  return value !== undefined && value !== "" && path.isAbsolute(value)
}

// Characters that force an Exec argument into double quotes (Desktop Entry spec).
const RESERVED = /[\s"'\\><~|&;$*?#()`]/
const QUOTED_ESCAPES = /["`$\\]/g
const STRING_ESCAPES = new Map([["\\", "\\\\"], ["\n", "\\n"], ["\t", "\\t"], ["\r", "\\r"]])

// desktopExec renders argv as an Exec value: arguments with reserved
// characters are double-quoted with " ` $ \ backslash-escaped, % becomes %%,
// then string escapes (\\, \n, \t, \r) apply to the whole value.
export function desktopExec(argv: readonly string[]): string {
  const quoted = argv.map((argument) => {
    const literal = argument.replaceAll("%", "%%")
    return argument === "" || RESERVED.test(literal) ? `"${literal.replace(QUOTED_ESCAPES, (char) => `\\${char}`)}"` : literal
  }).join(" ")
  return quoted.replace(/[\\\n\t\r]/g, (char) => STRING_ESCAPES.get(char) ?? char)
}

export function autostartEntry(argv: readonly string[]): string {
  return [
    "[Desktop Entry]",
    "Type=Application",
    "Name=servef",
    "Comment=Open servef at login",
    `Exec=${desktopExec(argv)}`,
    "Icon=servef-electron",
    "Terminal=false",
    "X-GNOME-Autostart-enabled=true",
    "",
  ].join("\n")
}

// autostartEnabled: a [Desktop Entry] group whose Hidden is not true and
// X-GNOME-Autostart-enabled is not false.
export function autostartEnabled(text: string | null): boolean {
  if (text === null) return false
  let group: string | null = null
  let entry = false
  for (const raw of text.split("\n")) {
    const line = raw.trim()
    if (line === "" || line.startsWith("#")) continue
    if (line.startsWith("[") && line.endsWith("]")) {
      group = line.slice(1, -1)
      if (group === "Desktop Entry") entry = true
      continue
    }
    if (group !== "Desktop Entry") continue
    const separator = line.indexOf("=")
    if (separator === -1) continue
    const key = line.slice(0, separator).trim()
    const value = line.slice(separator + 1).trim()
    if (key === "Hidden" && value === "true") return false
    if (key === "X-GNOME-Autostart-enabled" && value === "false") return false
  }
  return entry
}

export interface Autostart {
  enabled(): boolean
  // set writes or removes the entry; throws on filesystem errors.
  set(on: boolean): void
  // file is the entry's path, for manual instructions.
  file: string
}

export function createXdgAutostart(plan: AutostartPlan): Autostart {
  const file = path.join(plan.dir, AUTOSTART_FILE)
  const enabled = (): boolean => {
    try {
      return autostartEnabled(readFileSync(file, "utf8"))
    } catch {
      return false
    }
  }
  const set = (on: boolean): void => {
    if (!on) {
      try {
        unlinkSync(file)
      } catch (error) {
        if (!isNotFound(error)) throw error
      }
      return
    }
    mkdirSync(plan.dir, { recursive: true })
    const temp = `${file}.${process.pid}.tmp`
    try {
      writeFileSync(temp, autostartEntry(plan.argv), { mode: 0o644 })
      chmodSync(temp, 0o644)
      renameSync(temp, file)
    } catch (error) {
      try {
        unlinkSync(temp)
      } catch {
        // Never written, or already renamed.
      }
      throw error
    }
  }
  return { enabled, set, file }
}

// migrateLegacyAutostart carries an opt-in made under the old id to the current
// id and executable. An old entry anything else wrote or edited stays; an entry
// already under the current id is kept. A failure retries on the next launch.
export function migrateLegacyAutostart(plan: AutostartPlan, autostart: Autostart): void {
  const legacy = path.join(plan.dir, LEGACY_AUTOSTART_FILE)
  try {
    if (!LEGACY_ENTRY.test(readFileSync(legacy, "utf8"))) return
    if (!existsSync(autostart.file)) autostart.set(true)
    unlinkSync(legacy)
  } catch {
    // No old entry, or it cannot be moved now.
  }
}

function isNotFound(error: unknown): boolean {
  return typeof error === "object" && error !== null && "code" in error && error.code === "ENOENT"
}
