import type { ChangedFile } from "../../api/changes-contract.ts"
import type { Commit } from "../../api/commits-contract.ts"
import type { Compare } from "./compare.ts"

// Pure file-list states shared by every compare's change list, Files rail,
// and review tab.

// CompareDetails is what a review says about its compare.
export type CompareDetails =
  | { kind: "working"; warnings: readonly string[] }
  | { kind: "branch"; base: string; head: string; ahead: number; mergeBase: string }
  | { kind: "commit"; commit: Commit | null }
  | { kind: "range"; count: number }

// CompareFile is a changed file of any compare; only the working tree has
// untracked files.
export type CompareFile = ChangedFile

// FileList is one compare's file list as Source Control knows it.
export type FileList =
  | { kind: "loading" }
  | { kind: "ready"; files: readonly CompareFile[]; truncated: boolean }
  | { kind: "error"; message: string }

// FileListSources holds what each compare's list is read from.
export interface FileListSources {
  working: { files: readonly CompareFile[]; truncated: boolean } | null
  workingError: string
  // branch returns head's list against base, if read.
  branch(base: string, head: string): FileList | undefined
  commit(hash: string): FileList | undefined
  range(from: string, to: string): FileList | undefined
}

// compareFileList is a compare's list: loading until read, then ready or
// failed.
export function compareFileList(compare: Compare, sources: FileListSources): FileList {
  if (compare.kind === "working") {
    if (sources.working) return { kind: "ready", files: sources.working.files, truncated: sources.working.truncated }
    return sources.workingError ? { kind: "error", message: sources.workingError } : { kind: "loading" }
  }
  const list = compare.kind === "branch" ? sources.branch(compare.base, compare.head)
    : compare.kind === "commit" ? sources.commit(compare.hash)
    : sources.range(compare.from, compare.to)
  return list ?? { kind: "loading" }
}

// ancestorDirectories lists the folders holding paths, with a trailing slash
// as Pierre trees take them.
export function ancestorDirectories(paths: readonly string[]): string[] {
  const directories = new Set<string>()
  for (const path of paths) {
    const segments = path.split("/")
    for (let index = 1; index < segments.length; index++) directories.add(`${segments.slice(0, index).join("/")}/`)
  }
  return [...directories]
}

// displayOrder sorts paths as the Files tree shows them: folders before
// files at each level, each by name, so the review reads in tree order.
export function displayOrder(files: readonly CompareFile[]): CompareFile[] {
  const compare = (left: string, right: string) => left.localeCompare(right, undefined, { numeric: true, sensitivity: "base" })
    || (left < right ? -1 : left > right ? 1 : 0)
  return [...files].sort((left, right) => {
    const a = left.path.split("/")
    const b = right.path.split("/")
    for (let index = 0; index < Math.min(a.length, b.length); index++) {
      const leftSegment = a[index] ?? ""
      const rightSegment = b[index] ?? ""
      const leftFolder = index < a.length - 1
      const rightFolder = index < b.length - 1
      if (leftSegment === rightSegment && leftFolder === rightFolder) continue
      if (leftFolder !== rightFolder) return leftFolder ? -1 : 1
      return compare(leftSegment, rightSegment)
    }
    return a.length - b.length
  })
}
