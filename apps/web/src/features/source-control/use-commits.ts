import { useCallback, useEffect, useRef, useState } from "react"

import { appendedCommits, commitsUrl, refreshedCommits } from "./commit-pages.ts"
import type { CompareFile, FileList } from "./compare-files.ts"
import {
  historyError, isBranchFilesResponse, isCommitFilesResponse, isCommitsResponse, isRangeFilesResponse,
  type BranchFilesResponse, type CommitRange, type CommitsResponse,
} from "../../api/commits-contract.ts"
import { failureReason, fetchJSON, isAbort } from "../../api/http.ts"
import { useRefreshOnReturn } from "./use-refresh-on-return.ts"

export interface CommitsState {
  // result is the loaded list; it stays while a refresh of its range runs or
  // fails.
  result: CommitsResponse | null
  error: string
  loading: boolean
  loadingMore: boolean
  // moreError reports a failed Load more; the list stays.
  moreError: string
}

function fetchHistory<T>(url: string, signal: AbortSignal, valid: (value: unknown) => value is T): Promise<T> {
  return fetchJSON(url, { signal }, valid, (failure) => historyError(failureReason(failure)))
}

// BranchState is one base's net file list: null until read, kept while a
// reread runs or fails.
export interface BranchState {
  result: BranchFilesResponse | null
  error: string
  loading: boolean
}

// branchKey names a base and head pair in the branch file lists: the base
// alone for HEAD, and with "::uncommitted" for HEAD with the working tree.
// Git branch names never hold ":".
export function branchKey(base: string, head: string, uncommitted = false): string {
  if (head) return `${base}:${head}`
  return uncommitted ? `${base}::uncommitted` : base
}

// rangeKey names a commit range in the range file lists.
export function rangeKey(from: string, to: string): string {
  return `${from}:${to}`
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : historyError("unknown error")
}

function readyList(response: { files: CompareFile[]; truncated: boolean }): FileList {
  return { kind: "ready", files: response.files, truncated: response.truncated }
}

// sameBranch keeps a reread that found the same compare from replacing the
// list, so its stats and diffs stay.
function sameBranch(left: BranchFilesResponse | null, right: BranchFilesResponse): boolean {
  return left !== null && left.head === right.head && left.base === right.base && left.mergeBase === right.mergeBase
    && left.ahead === right.ahead && left.truncated === right.truncated && left.files.length === right.files.length
    && left.files.every((file, index) => {
      const other = right.files[index]
      return other !== undefined && other.path === file.path && other.status === file.status && other.oldPath === file.oldPath
    })
}

// useCommits owns the project's commit list, commits' and ranges' file
// lists, and the branch net file lists per base and head. The list and the branch
// read on project load, when Source Control or a live review opens, on window
// focus or visibility (throttled), on refresh, and when the range or base
// changes. It never polls. Commits never change, so commit and range file
// lists are read once. base is the chosen base branch, "" for the default.
export function useCommits(project: string, viewActive: boolean, range: CommitRange, base: string) {
  const [state, setState] = useState<CommitsState>({ result: null, error: "", loading: project !== "", loadingMore: false, moreError: "" })
  const [files, setFiles] = useState<ReadonlyMap<string, FileList>>(() => new Map())
  const controller = useRef<AbortController | null>(null)
  const moreController = useRef<AbortController | null>(null)
  const filesRequests = useRef(new Map<string, AbortController>())
  const lastRead = useRef(0)
  const [branches, setBranches] = useState<ReadonlyMap<string, BranchState>>(() => new Map())
  const branchRequests = useRef(new Map<string, AbortController>())
  // knownBranches are the base and head pairs read so far, by key; a refresh
  // rereads them all.
  const knownBranches = useRef(new Map<string, { base: string; head: string; uncommitted: boolean }>())
  const [rangeFiles, setRangeFiles] = useState<ReadonlyMap<string, FileList>>(() => new Map())
  const rangeRequests = useRef(new Map<string, AbortController>())

  // loadBranch reads head's files against a base; "" is the default base and
  // HEAD. uncommitted reads HEAD's with the working tree. Without reread it
  // joins a read already running.
  const loadBranch = useCallback((branchBase: string, head = "", reread = false, uncommitted = false) => {
    if (!project) return
    const working = uncommitted && !head
    const key = branchKey(branchBase, head, working)
    if (!reread && branchRequests.current.has(key)) return
    knownBranches.current.set(key, { base: branchBase, head, uncommitted: working })
    branchRequests.current.get(key)?.abort()
    const current = new AbortController()
    branchRequests.current.set(key, current)
    const update = (change: (previous: BranchState) => BranchState) => setBranches((previous) => {
      const next = new Map(previous)
      next.set(key, change(previous.get(key) ?? { result: null, error: "", loading: false }))
      return next
    })
    update((previous) => ({ ...previous, loading: true }))
    const params = new URLSearchParams({ project })
    if (branchBase) params.set("base", branchBase)
    if (head) params.set("head", head)
    if (working) params.set("uncommitted", "1")
    fetchHistory(`/api/branch-files?${params.toString()}`, current.signal, isBranchFilesResponse).then((result) => {
      if (branchRequests.current.get(key) !== current) return
      branchRequests.current.delete(key)
      update((previous) => ({ result: sameBranch(previous.result, result) ? previous.result : result, error: "", loading: false }))
    }, (error: unknown) => {
      if (branchRequests.current.get(key) !== current || current.signal.aborted) return
      branchRequests.current.delete(key)
      update((previous) => ({ ...previous, error: errorMessage(error), loading: false }))
    })
  }, [project])

  const refresh = useCallback(() => {
    if (!project) return
    controller.current?.abort()
    moreController.current?.abort()
    const current = new AbortController()
    controller.current = current
    lastRead.current = Date.now()
    setState((previous) => ({ ...previous, loading: true, loadingMore: false, moreError: "" }))
    fetchHistory(commitsUrl(project, range, "", 0, base), current.signal, isCommitsResponse).then((first) => {
      if (controller.current !== current) return
      setState((previous) => ({ ...previous, result: refreshedCommits(previous.result, first), error: "", loading: false }))
    }, (error: unknown) => {
      if (controller.current !== current || current.signal.aborted) return
      setState((previous) => ({ ...previous, error: errorMessage(error), loading: false }))
    })
    loadBranch(base, "", true)
    for (const [key, known] of knownBranches.current) {
      if (key !== branchKey(base, "")) loadBranch(known.base, known.head, true, known.uncommitted)
    }
  }, [base, loadBranch, project, range])

  useEffect(() => {
    refresh()
    return () => {
      controller.current?.abort()
      moreController.current?.abort()
    }
  }, [refresh])

  useRefreshOnReturn(refresh, { lastRead, viewActive })

  // A list read for the other range or base is not this one's history, even
  // while the switch reads or after it fails.
  const loaded = state.result?.range === range ? state.result : null
  const loadMore = useCallback(() => {
    if (!project || !loaded?.more || !loaded.head) return
    moreController.current?.abort()
    const current = new AbortController()
    moreController.current = current
    setState((previous) => ({ ...previous, loadingMore: true, moreError: "" }))
    fetchHistory(commitsUrl(project, loaded.range, loaded.head, loaded.commits.length, base), current.signal, isCommitsResponse).then((page) => {
      if (moreController.current !== current) return
      setState((previous) => ({
        ...previous,
        result: previous.result ? appendedCommits(previous.result, page) : previous.result,
        loadingMore: false,
      }))
    }, (error: unknown) => {
      if (moreController.current !== current || current.signal.aborted) return
      setState((previous) => ({ ...previous, loadingMore: false, moreError: errorMessage(error) }))
    })
  }, [base, loaded, project])

  // loadFiles reads a commit's files unless they are loaded or loading.
  const loadFiles = useCallback((hash: string, retry = false) => {
    if (!project) return
    const known = files.get(hash)
    if (known && (known.kind !== "error" || !retry)) return
    const current = new AbortController()
    filesRequests.current.set(hash, current)
    setFiles((previous) => new Map(previous).set(hash, { kind: "loading" }))
    const params = new URLSearchParams({ project, commit: hash })
    fetchHistory(`/api/commit-files?${params.toString()}`, current.signal, isCommitFilesResponse).then((response) => {
      filesRequests.current.delete(hash)
      setFiles((previous) => new Map(previous).set(hash, readyList(response)))
    }, (error: unknown) => {
      filesRequests.current.delete(hash)
      if (isAbort(error)) {
        setFiles((previous) => {
          const next = new Map(previous)
          next.delete(hash)
          return next
        })
        return
      }
      setFiles((previous) => new Map(previous).set(hash, { kind: "error", message: errorMessage(error) }))
    })
  }, [files, project])

  // loadRangeFiles reads a commit range's files unless they are loaded or
  // loading; from is the oldest selected commit, to the newest.
  const loadRangeFiles = useCallback((from: string, to: string, retry = false) => {
    if (!project) return
    const key = rangeKey(from, to)
    const known = rangeFiles.get(key)
    if (known && (known.kind !== "error" || !retry)) return
    const current = new AbortController()
    rangeRequests.current.set(key, current)
    setRangeFiles((previous) => new Map(previous).set(key, { kind: "loading" }))
    const params = new URLSearchParams({ project, from, to })
    fetchHistory(`/api/range-files?${params.toString()}`, current.signal, isRangeFilesResponse).then((response) => {
      rangeRequests.current.delete(key)
      setRangeFiles((previous) => new Map(previous).set(key, readyList(response)))
    }, (error: unknown) => {
      rangeRequests.current.delete(key)
      if (isAbort(error)) {
        setRangeFiles((previous) => {
          const next = new Map(previous)
          next.delete(key)
          return next
        })
        return
      }
      setRangeFiles((previous) => new Map(previous).set(key, { kind: "error", message: errorMessage(error) }))
    })
  }, [project, rangeFiles])

  useEffect(() => {
    const requests = filesRequests.current
    const ranges = rangeRequests.current
    const branchReads = branchRequests.current
    return () => {
      for (const request of requests.values()) request.abort()
      for (const request of ranges.values()) request.abort()
      for (const request of branchReads.values()) request.abort()
    }
  }, [])

  return { ...state, result: loaded, branches, files, loadBranch, loadFiles, loadMore, loadRangeFiles, rangeFiles, refresh }
}

export type Commits = ReturnType<typeof useCommits>

// branchList is a base's file list as a FileList.
export function branchList(state: BranchState | undefined): FileList | undefined {
  if (!state) return undefined
  if (state.result) return readyList(state.result)
  return state.error ? { kind: "error", message: state.error } : { kind: "loading" }
}
