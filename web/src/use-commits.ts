import { useCallback, useEffect, useRef, useState } from "react"

import { isDiffErrorResponse } from "./changes-contract.ts"
import type { CommitFiles } from "./commit-rows.ts"
import { appendedCommits, commitsUrl, refreshedCommits } from "./commit-pages.ts"
import {
  historyError, isCommitFilesResponse, isCommitsResponse, type CommitRange, type CommitsResponse,
} from "./commits-contract.ts"

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

async function fetchJSON<T>(url: string, signal: AbortSignal, valid: (value: unknown) => value is T): Promise<T> {
  let response: Response
  try {
    response = await fetch(url, { headers: { Accept: "application/json" }, signal })
  } catch (error: unknown) {
    if (error instanceof Error && error.name === "AbortError") throw error
    throw new Error(historyError("the server didn't respond"))
  }
  const payload: unknown = await response.json().catch(() => null)
  if (response.ok && valid(payload)) return payload
  throw new Error(historyError(isDiffErrorResponse(payload) ? payload.error : "the server sent an invalid response"))
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : historyError("unknown error")
}

function isAbort(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError"
}

// useCommits owns the project's commit list and the file lists of commits
// the view expands. The list reads on project load, when the Source Control
// view opens, on window focus or visibility (throttled), on refresh, and when
// the range changes. It never polls. Commits never change, so file lists are
// read once per commit.
export function useCommits(project: string, viewActive: boolean, range: CommitRange) {
  const [state, setState] = useState<CommitsState>({ result: null, error: "", loading: project !== "", loadingMore: false, moreError: "" })
  const [files, setFiles] = useState<ReadonlyMap<string, CommitFiles>>(() => new Map())
  const controller = useRef<AbortController | null>(null)
  const moreController = useRef<AbortController | null>(null)
  const filesRequests = useRef(new Map<string, AbortController>())
  const lastRead = useRef(0)

  const refresh = useCallback(() => {
    if (!project) return
    controller.current?.abort()
    moreController.current?.abort()
    const current = new AbortController()
    controller.current = current
    lastRead.current = Date.now()
    setState((previous) => ({ ...previous, loading: true, loadingMore: false, moreError: "" }))
    fetchJSON(commitsUrl(project, range, "", 0), current.signal, isCommitsResponse).then((first) => {
      if (controller.current !== current) return
      setState((previous) => ({ ...previous, result: refreshedCommits(previous.result, first), error: "", loading: false }))
    }, (error: unknown) => {
      if (controller.current !== current || current.signal.aborted) return
      setState((previous) => ({ ...previous, error: errorMessage(error), loading: false }))
    })
  }, [project, range])

  useEffect(() => {
    refresh()
    return () => {
      controller.current?.abort()
      moreController.current?.abort()
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
    fetchJSON(commitsUrl(project, loaded.range, loaded.head, loaded.commits.length), current.signal, isCommitsResponse).then((page) => {
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
    fetchJSON(`/api/commit-files?${params.toString()}`, current.signal, isCommitFilesResponse).then((response) => {
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

  useEffect(() => {
    const requests = filesRequests.current
    return () => {
      for (const request of requests.values()) request.abort()
    }
  }, [])

  return { ...state, files, loadFiles, loadMore, refresh }
}

export type Commits = ReturnType<typeof useCommits>
