import { useCallback, useEffect, useMemo } from "react"

import { rangeCount, type Compare } from "./compare.ts"
import { compareFileList, type CompareDetails, type FileList } from "./compare-files.ts"
import type { Changes } from "./use-changes.ts"
import { branchKey, branchList, rangeKey, type Commits } from "./use-commits.ts"
import { useCompareStats, type CompareStats } from "./use-compare-stats.ts"

export interface CompareData {
  list: FileList
  stats: CompareStats | null
  details: CompareDetails
  // version names the list read: live compares reread stats and diffs as it
  // moves; commits never change.
  version: string
  retry(): void
}

// useCompareData gathers a compare's file list, line stats, and details,
// reading what is missing. The change list, the Files rail, and the review
// tab share it.
export function useCompareData(project: string, compare: Compare | null, changes: Changes, commits: Commits): CompareData | null {
  const { files, loadBranch, loadFiles, loadRangeFiles, rangeFiles, branches } = commits
  const history = commits.result?.commits
  const list = useMemo((): FileList | null => compare && compareFileList(compare, {
    working: changes.result,
    workingError: changes.error,
    branch: (base, head) => branchList(branches.get(branchKey(base, head))),
    commit: (hash) => files.get(hash),
    range: (from, to) => rangeFiles.get(rangeKey(from, to)),
  }), [branches, changes.error, changes.result, compare, files, rangeFiles])

  // Read a list nobody has asked for yet; commit and range lists read once.
  const missing = compare !== null && list?.kind === "loading"
  const branchBase = compare?.kind === "branch" ? compare.base : null
  const branchHead = compare?.kind === "branch" ? compare.head : ""
  const branchId = branchBase !== null ? branchKey(branchBase, branchHead) : null
  const branchKnown = branchId !== null && branches.has(branchId)
  const commitHash = compare?.kind === "commit" ? compare.hash : ""
  const rangeFrom = compare?.kind === "range" ? compare.from : ""
  const rangeTo = compare?.kind === "range" ? compare.to : ""
  useEffect(() => {
    if (!missing) return
    if (branchBase !== null && !branchKnown) loadBranch(branchBase, branchHead)
    if (commitHash) loadFiles(commitHash)
    if (rangeFrom && rangeTo) loadRangeFiles(rangeFrom, rangeTo)
  }, [branchBase, branchHead, branchKnown, commitHash, loadBranch, loadFiles, loadRangeFiles, missing, rangeFrom, rangeTo])

  const branchState = branchId !== null ? branches.get(branchId) : undefined
  const version = !compare || list?.kind !== "ready" ? null
    : compare.kind === "working" ? `read:${changes.read}`
    : compare.kind === "branch" ? `${branchState?.result?.head ?? ""}:${branchState?.result?.mergeBase ?? ""}`
    : ""
  const stats = useCompareStats(project, compare, version)

  const retry = useCallback(() => {
    if (!compare) return
    if (compare.kind === "working") changes.refresh()
    else if (compare.kind === "branch") loadBranch(compare.base, compare.head, true)
    else if (compare.kind === "commit") loadFiles(compare.hash, true)
    else loadRangeFiles(compare.from, compare.to, true)
  }, [changes, compare, loadBranch, loadFiles, loadRangeFiles])

  const details = useMemo((): CompareDetails | null => {
    if (!compare) return null
    if (compare.kind === "working") return { kind: "working", warnings: changes.result?.warnings ?? [] }
    if (compare.kind === "branch") {
      const result = branchState?.result
      return { kind: "branch", base: result?.base ?? "", head: compare.head, ahead: result?.ahead ?? 0, mergeBase: result?.mergeBase ?? "" }
    }
    if (compare.kind === "commit") return { kind: "commit", commit: history?.find((commit) => commit.hash === compare.hash) ?? null }
    return { kind: "range", count: rangeCount(history ?? [], compare) }
  }, [branchState?.result, changes.result?.warnings, compare, history])

  if (!compare || !list || !details) return null
  return { list, stats, details, version: version ?? "", retry }
}
