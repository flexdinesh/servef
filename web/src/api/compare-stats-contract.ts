import { isRecord } from "./http.ts"

// Transport contract for /api/compare-stats: per-file line counts for one
// compare (compare=working|branch|commit|range, with base, commit, or from
// and to), in the order of the compare's file list.

// CountedFileStats carries line counts as git diff --numstat does.
export interface CountedFileStats {
  path: string
  status: "counted"
  additions: number
  deletions: number
  // blob is the git blob hash of the newer side (the older for a deletion),
  // "" when that side is in the worktree and was not read.
  blob: string
}

// UncountedFileStats is a file whose lines were not counted: a binary side,
// a side over 4 MiB, or the request's read budget spent.
export interface UncountedFileStats {
  path: string
  status: "binary" | "tooLarge" | "budget"
  blob: string
}

export type FileStats = CountedFileStats | UncountedFileStats

export type FileStatsStatus = FileStats["status"]

export interface CompareStatsResponse {
  files: FileStats[]
  // budgetExceeded reports that the read budget left files uncounted.
  budgetExceeded: boolean
}

function isCount(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 0
}

function isBlob(value: unknown): value is string {
  return typeof value === "string" && (value === "" || /^[0-9a-f]{40}$/.test(value))
}

function isFileStats(value: unknown): value is FileStats {
  if (!isRecord(value) || typeof value.path !== "string" || value.path === "" || !isBlob(value.blob)) return false
  switch (value.status) {
    case "counted":
      return isCount(value.additions) && isCount(value.deletions)
    case "binary":
    case "tooLarge":
    case "budget":
      return value.additions === undefined && value.deletions === undefined
    default:
      return false
  }
}

export function isCompareStatsResponse(value: unknown): value is CompareStatsResponse {
  if (!isRecord(value) || !Array.isArray(value.files) || typeof value.budgetExceeded !== "boolean") return false
  const files: unknown[] = value.files
  // Only a spent budget leaves files with status "budget".
  return files.every(isFileStats) && (value.budgetExceeded || files.every((file) => file.status !== "budget"))
}
