import { useCallback, useEffect, useRef, useState } from "react"

import type { CommitFiles } from "./commit-rows.ts"
import { appendedCommits, commitsUrl, refreshedCommits } from "./commit-pages.ts"
import {
  historyError, isBranchFilesResponse, isCommitFilesResponse, isCommitsResponse, isRangeFilesResponse,
  type BranchFilesResponse, type CommitRange, type CommitsResponse,
} from "./commits-contract.ts"
import { failureReason, fetchJSON, isAbort } from "./http.ts"

// Focus and visibility refreshes run at most this often.
const focusThrottle = 2_000

export interface CommitsState {
  // result is the loaded list; it stays while a refresh runs or fails.
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

// BranchState is the branch's net file list: null until read, kept while a
// reread runs or fails.
export interface BranchState {
  result: BranchFilesResponse | null
  error: string
  loading: boolean
}

// branchCurrent reports whether a branch read still matches the listed
// history: the same head, base, and ahead count mean the same merge-base.
function branchCurrent(branch: BranchFilesResponse | null, history: CommitsResponse): boolean {
  return branch !== null && branch.head === history.head && branch.base === history.base && branch.ahead === history.ahead
}

// rangeKey names a commit range in the range file lists.
export function rangeKey(from: string, to: string): string {
  return `${from}:${to}`
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : historyError("unknown error")
}

// useCommits owns the project's commit list, the file lists of commits the
// view expands, the branch's net file list, and selected ranges' file lists.
// The list reads on project load, when the Source Control view opens, on
// window focus or visibility (throttled), on refresh, and when the range
// changes. It never polls. Commits never change, so commit and range file
// lists are read once; the branch list rereads when the branch range's head,
// base, or ahead count changes.
export function useCommits(project: string, viewActive: boolean, range: CommitRange) {
  const [state, setState] = useState<CommitsState>({ result: null, error: "", loading: project !== "", loadingMore: false, moreError: "" })
  const [files, setFiles] = useState<ReadonlyMap<string, CommitFiles>>(() => new Map())
  const controller = useRef<AbortController | null>(null)
  const moreController = useRef<AbortController | null>(null)
  const filesRequests = useRef(new Map<string, AbortController>())
  const lastRead = useRef(0)
  const [branch, setBranch] = useState<BranchState>({ result: null, error: "", loading: false })
  const branchController = useRef<AbortController | null>(null)
  const branchResult = useRef<BranchFilesResponse | null>(null)
  const [rangeFiles, setRangeFiles] = useState<ReadonlyMap<string, CommitFiles>>(() => new Map())
  const rangeRequests = useRef(new Map<string, AbortController>())

  // readBranch reads the branch's files once the branch range lists commits
  // ahead of a base, unless the last read still matches.
  const readBranch = useCallback((history: CommitsResponse, force: boolean) => {
    if (!project || history.range !== "branch" || history.ahead === 0 || !history.base) return
    if (!force && branchCurrent(branchResult.current, history)) return
    branchController.current?.abort()
    const current = new AbortController()
    branchController.current = current
    setBranch((previous) => ({ ...previous, loading: true }))
    const params = new URLSearchParams({ project })
    fetchHistory(`/api/branch-files?${params.toString()}`, current.signal, isBranchFilesResponse).then((result) => {
      if (branchController.current !== current) return
      branchResult.current = result
      setBranch({ result, error: "", loading: false })
    }, (error: unknown) => {
      if (branchController.current !== current || current.signal.aborted) return
      setBranch((previous) => ({ ...previous, error: errorMessage(error), loading: false }))
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
    fetchHistory(commitsUrl(project, range, "", 0), current.signal, isCommitsResponse).then((first) => {
      if (controller.current !== current) return
      setState((previous) => ({ ...previous, result: refreshedCommits(previous.result, first), error: "", loading: false }))
      readBranch(first, false)
    }, (error: unknown) => {
      if (controller.current !== current || current.signal.aborted) return
      setState((previous) => ({ ...previous, error: errorMessage(error), loading: false }))
    })
  }, [project, range, readBranch])

  useEffect(() => {
    refresh()
    return () => {
      controller.current?.abort()
      moreController.current?.abort()
      branchController.current?.abort()
    }
  }, [refresh])

  // Opening the view rereads; staying open does not.
  const wasActive = useRef(viewActive)
  useEffect(() => {
    if (viewActive && !wasActive.current) refresh()
    wasActive.current = viewActive
  }, [refresh, viewActive])

  useEffect(() => {
    const onReturn = () => {
      if (document.visibilityState !== "visible" || Date.now() - lastRead.current < focusThrottle) return
      refresh()
    }
    window.addEventListener("focus", onReturn)
    document.addEventListener("visibilitychange", onReturn)
    return () => {
      window.removeEventListener("focus", onReturn)
      document.removeEventListener("visibilitychange", onReturn)
    }
  }, [refresh])

  const loaded = state.result
  const loadMore = useCallback(() => {
    if (!project || !loaded?.more || !loaded.head) return
    moreController.current?.abort()
    const current = new AbortController()
    moreController.current = current
    setState((previous) => ({ ...previous, loadingMore: true, moreError: "" }))
    fetchHistory(commitsUrl(project, loaded.range, loaded.head, loaded.commits.length), current.signal, isCommitsResponse).then((page) => {
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
  }, [loaded, project])

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
      setFiles((previous) => new Map(previous).set(hash, { kind: "ready", response }))
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
      setRangeFiles((previous) => new Map(previous).set(key, { kind: "ready", response }))
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

  // retryBranch rereads the branch's files after a failure.
  const retryBranch = useCallback(() => {
    if (loaded) readBranch(loaded, true)
  }, [loaded, readBranch])

  useEffect(() => {
    const requests = filesRequests.current
    const ranges = rangeRequests.current
    return () => {
      for (const request of requests.values()) request.abort()
      for (const request of ranges.values()) request.abort()
    }
  }, [])

  return { ...state, branch, files, loadFiles, loadMore, loadRangeFiles, rangeFiles, refresh, retryBranch }
}

export type Commits = ReturnType<typeof useCommits>
