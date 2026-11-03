import { useCallback, useEffect, useMemo, useRef } from "react"

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
    branch: (base, head, uncommitted) => branchList(branches.get(branchKey(base, head, uncommitted))),
    commit: (hash) => files.get(hash),
    range: (from, to) => rangeFiles.get(rangeKey(from, to)),
  }), [branches, changes.error, changes.result, compare, files, rangeFiles])

  // Read a list nobody has asked for yet; commit and range lists read once.
  const missing = compare !== null && list?.kind === "loading"
  const branchBase = compare?.kind === "branch" ? compare.base : null
  const branchHead = compare?.kind === "branch" ? compare.head : ""
  const branchWorking = compare?.kind === "branch" && compare.uncommitted
  const branchId = branchBase !== null ? branchKey(branchBase, branchHead, branchWorking) : null
  const branchKnown = branchId !== null && branches.has(branchId)
  const commitHash = compare?.kind === "commit" ? compare.hash : ""
  const rangeFrom = compare?.kind === "range" ? compare.from : ""
  const rangeTo = compare?.kind === "range" ? compare.to : ""
  // workingListed names the branch-with-working-tree list read last and the
  // status read it followed.
  const workingListed = useRef({ id: "", read: -1 })
  const workingRead = changes.read
  useEffect(() => {
    if (!missing) return
    if (branchBase !== null && !branchKnown) {
      loadBranch(branchBase, branchHead, false, branchWorking)
      if (branchWorking && branchId !== null) workingListed.current = { id: branchId, read: workingRead }
    }
    if (commitHash) loadFiles(commitHash)
    if (rangeFrom && rangeTo) loadRangeFiles(rangeFrom, rangeTo)
  }, [branchBase, branchHead, branchId, branchKnown, branchWorking, commitHash, loadBranch, loadFiles, loadRangeFiles, missing, rangeFrom, rangeTo, workingRead])

  // A branch with the working tree rereads its list with every status read,
  // so files the working tree adds or restores join or leave it.
  useEffect(() => {
    if (branchBase === null || !branchWorking || !branchKnown || branchId === null) return
    const listed = workingListed.current
    if (listed.id === branchId && listed.read === workingRead) return
    workingListed.current = { id: branchId, read: workingRead }
    loadBranch(branchBase, branchHead, true, true)
  }, [branchBase, branchHead, branchId, branchKnown, branchWorking, loadBranch, workingRead])

  // A branch with the working tree also moves with every status read.
  const branchState = branchId !== null ? branches.get(branchId) : undefined
  const version = !compare || list?.kind !== "ready" ? null
    : compare.kind === "working" ? `read:${changes.read}`
    : compare.kind === "branch" ? `${branchState?.result?.head ?? ""}:${branchState?.result?.mergeBase ?? ""}${compare.uncommitted ? `:read:${changes.read}` : ""}`
    : ""
  const stats = useCompareStats(project, compare, version)

  const retry = useCallback(() => {
    if (!compare) return
    if (compare.kind === "working") changes.refresh()
    else if (compare.kind === "branch") loadBranch(compare.base, compare.head, true, compare.uncommitted)
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
