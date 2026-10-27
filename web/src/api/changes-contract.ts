import { isRecord } from "./http.ts"
import { isFileType, type FileType } from "./page-contract.ts"

// Transport contracts for /api/changes and /api/diff (decision 0024).

export type ChangeStatus = "modified" | "added" | "untracked" | "deleted" | "renamed"

export interface ChangedFile {
  path: string
  // oldPath is the HEAD path of a renamed file.
  oldPath?: string
  status: ChangeStatus
}

export interface ChangesResponse {
  // repository is false for a folder project; files are then empty.
  repository: boolean
  branch: string
  files: ChangedFile[]
  truncated: boolean
  warnings: string[]
}

// DiffSide is one side of a working-tree diff. content is absent when the side
// is binary or too large to diff.
export interface DiffSide {
  content?: string
  size: number
  binary: boolean
  tooLarge: boolean
}

export interface DiffResponse {
  path: string
  oldPath?: string
  status: ChangeStatus
  documentType: FileType
  // original is HEAD, null when added or untracked; modified is the working
  // tree, null when deleted.
  original: DiffSide | null
  modified: DiffSide | null
}

export function isChangeStatus(value: unknown): value is ChangeStatus {
  return value === "modified" || value === "added" || value === "untracked" || value === "deleted" || value === "renamed"
}

export function isChangedFile(value: unknown): value is ChangedFile {
  return isRecord(value)
    && typeof value.path === "string"
    && value.path !== ""
    && isChangeStatus(value.status)
    && (value.oldPath === undefined || typeof value.oldPath === "string")
    && (value.status !== "renamed" || typeof value.oldPath === "string")
}

export function isChangesResponse(value: unknown): value is ChangesResponse {
  return isRecord(value)
    && typeof value.repository === "boolean"
    && typeof value.branch === "string"
    && Array.isArray(value.files)
    && value.files.every(isChangedFile)
    && (value.repository || value.files.length === 0)
    && typeof value.truncated === "boolean"
    && Array.isArray(value.warnings)
    && value.warnings.every((warning: unknown) => typeof warning === "string")
}

function isDiffSide(value: unknown): value is DiffSide {
  return isRecord(value)
    && (value.content === undefined || typeof value.content === "string")
    && typeof value.size === "number"
    && value.size >= 0
    && typeof value.binary === "boolean"
    && typeof value.tooLarge === "boolean"
    // Binary and too-large sides never carry content.
    && (!(value.binary || value.tooLarge) || value.content === undefined)
}

export function isDiffResponse(value: unknown): value is DiffResponse {
  if (!isRecord(value) || !isChangeStatus(value.status)) return false
  const added = value.status === "added" || value.status === "untracked"
  return typeof value.path === "string"
    && (value.oldPath === undefined || typeof value.oldPath === "string")
    && isFileType(value.documentType)
    && (added ? value.original === null : isDiffSide(value.original))
    && (value.status === "deleted" ? value.modified === null : isDiffSide(value.modified))
}

const statusErrorPrefix = "Couldn't read git status: "

// gitStatusError phrases a failure as "Couldn't read git status: <reason>.",
// keeping a server message that already says so.
export function gitStatusError(reason: string): string {
  const message = reason.startsWith(statusErrorPrefix) ? reason : `${statusErrorPrefix}${reason}`
  return /[.!?]$/.test(message) ? message : `${message}.`
}
