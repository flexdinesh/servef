import { closeSync, fchmodSync, fsyncSync, mkdirSync, openSync, readFileSync, renameSync, unlinkSync, writeSync } from "node:fs"
import path from "node:path"
import { isRecord } from "../../../web/src/api/http.ts"
import { isPlainAbsolute } from "./reveal.ts"

// Recent folders (decision 0081): projects local windows showed, most recent
// first, bounded, persisted in userData/recent-folders.json. They feed File >
// Open Recent, the macOS Dock menu, and the OS recent documents list.

export const RECENT_FILE = "recent-folders.json"
export const MAX_RECENT = 10
const VERSION = 1

// addRecent moves folder to the front, dropping duplicates and anything past MAX_RECENT.
export function addRecent(folders: readonly string[], folder: string): string[] {
  return [folder, ...folders.filter((entry) => entry !== folder)].slice(0, MAX_RECENT)
}

export function removeRecent(folders: readonly string[], folder: string): string[] {
  return folders.filter((entry) => entry !== folder)
}

// parseRecent reads {version: 1, folders: string[]}; unknown versions,
// malformed JSON, and invalid entries yield what is valid, else [].
export function parseRecent(text: string): string[] {
  let value: unknown
  try {
    value = JSON.parse(text)
  } catch {
    return []
  }
  if (!isRecord(value) || value.version !== VERSION || !Array.isArray(value.folders)) return []
  const folders: string[] = []
  for (const entry of value.folders) {
    if (typeof entry === "string" && isPlainAbsolute(entry) && !folders.includes(entry)) folders.push(entry)
  }
  return folders.slice(0, MAX_RECENT)
}

export function serializeRecent(folders: readonly string[]): string {
  return JSON.stringify({ version: VERSION, folders }, null, 2) + "\n"
}

// recentLabel shows a folder with the home directory as "~".
export function recentLabel(folder: string, home: string): string {
  if (home !== "" && home !== path.sep && (folder === home || folder.startsWith(home + path.sep))) return `~${folder.slice(home.length)}`
  return folder
}

export interface RecentStore {
  load(): string[]
  save(folders: readonly string[]): void
}

// createRecentStore persists to file atomically (mode 0600). A missing or
// unreadable file loads as empty.
export function createRecentStore(file: string): RecentStore {
  return {
    load() {
      try {
        return parseRecent(readFileSync(file, "utf8"))
      } catch {
        return []
      }
    },
    save(folders) {
      mkdirSync(path.dirname(file), { recursive: true })
      const temp = `${file}.tmp-${process.pid}`
      try {
        const fd = openSync(temp, "w", 0o600)
        try {
          fchmodSync(fd, 0o600)
          writeSync(fd, serializeRecent(folders))
          fsyncSync(fd)
        } finally {
          closeSync(fd)
        }
        renameSync(temp, file)
      } catch (error) {
        try {
          unlinkSync(temp)
        } catch {
          // The temp file may not exist.
        }
        throw error
      }
    },
  }
}

// newlyShown lists the projects in next that previous did not show, in order:
// the projects a window just started showing.
export function newlyShown(previous: ReadonlySet<string>, next: readonly string[]): string[] {
  return next.filter((project) => !previous.has(project))
}
