import { useCallback, useEffect, useRef, useState } from "react"

import { isDiffResponse, type DiffResponse } from "./changes-contract.ts"
import { failureReason, fetchJSON, RequestError, type RequestFailure } from "./http.ts"
import { useRefreshOnReturn } from "./use-refresh-on-return.ts"
import { tabKey, type WorkspaceTab } from "./workspace-tabs.ts"

export type DiffState =
  | { kind: "loading" }
  | { kind: "ready"; diff: DiffResponse }
  // unchanged: the tracked file matches HEAD again, or a branch no longer
  // changes it.
  | { kind: "unchanged"; message: string }
  | { kind: "error"; message: string }

// diffError keeps the server's own sentence; other failures say what failed.
function diffError(failure: RequestFailure): string {
  return failure.kind === "rejected" ? failure.error : `Couldn't load this diff: ${failureReason(failure)}.`
}

function diffUrl(project: string, tab: WorkspaceTab, oldPath: string): string {
  const params = new URLSearchParams({ project, path: tab.path })
  if (oldPath) params.set("oldPath", oldPath)
  if (tab.kind === "commit") params.set("commit", tab.commit)
  if (tab.kind === "branch") params.set("branch", "1")
  if (tab.kind === "range") {
    params.set("from", tab.from)
    params.set("to", tab.to)
  }
  return `/api/diff?${params.toString()}`
}

async function fetchDiff(project: string, tab: WorkspaceTab, oldPath: string, signal: AbortSignal): Promise<DiffState> {
  try {
    return { kind: "ready", diff: await fetchJSON(diffUrl(project, tab, oldPath), { signal }, isDiffResponse, diffError) }
  } catch (error: unknown) {
    if (!(error instanceof RequestError)) throw error
    // Code "unchanged": the tracked file matches HEAD, or the branch no longer changes it.
    return error.failure.kind === "rejected" && error.failure.code === "unchanged"
      ? { kind: "unchanged", message: error.message }
      : { kind: "error", message: error.message }
  }
}

function sameSide(left: DiffResponse["original"], right: DiffResponse["original"]): boolean {
  if (left === null || right === null) return left === right
  return left.size === right.size && left.binary === right.binary && left.tooLarge === right.tooLarge && left.content === right.content
}

// sameDiff keeps a refetch that found nothing new from re-rendering the diff.
function sameDiff(left: DiffResponse, right: DiffResponse): boolean {
  return left.path === right.path && left.oldPath === right.oldPath && left.status === right.status
    && left.documentType.id === right.documentType.id
    && sameSide(left.original, right.original) && sameSide(left.modified, right.modified)
}

// useWorkingTreeDiff reads the open diff tab's sides. It refetches when the
// tab is activated, on window focus (throttled), and on retry; a refetch keeps
// the shown diff until its answer arrives. Commit and range diffs never
// change, so focus refetches only working-tree and branch diffs.
export function useWorkingTreeDiff(project: string, tab: WorkspaceTab | null, oldPath: string) {
  const diff = tab && tab.kind !== "document" ? tab : null
  const key = diff ? `${tabKey(diff)}\n${oldPath}` : ""
  const live = diff?.kind === "diff" || diff?.kind === "branch"
  const [entry, setEntry] = useState<{ key: string; state: DiffState } | null>(null)
  const [loading, setLoading] = useState(false)
  const [request, setRequest] = useState(0)
  const lastRead = useRef(0)
  // Leaving diffs drops the last response; activation refetches anyway.
  if (!key && entry) setEntry(null)

  // Callers memoize tab, so the read reruns when the tab or retry changes.
  useEffect(() => {
    if (!project || !diff) return
    const controller = new AbortController()
    lastRead.current = Date.now()
    setLoading(true)
    fetchDiff(project, diff, oldPath, controller.signal).then((state) => {
      setEntry((previous) => previous?.key === key && previous.state.kind === "ready" && state.kind === "ready" && sameDiff(previous.state.diff, state.diff)
        ? previous
        : { key, state })
      setLoading(false)
    }, () => {
      // Aborted by a newer request or unmount.
    })
    return () => {
      controller.abort()
      setLoading(false)
    }
  }, [diff, key, oldPath, project, request])

  const retry = useCallback(() => setRequest((count) => count + 1), [])
  useRefreshOnReturn(retry, { lastRead, enabled: live })
  const state: DiffState = entry?.key === key && key ? entry.state : { kind: "loading" }
  return { state, loading: Boolean(key) && loading, retry }
}

export type WorkingTreeDiff = ReturnType<typeof useWorkingTreeDiff>
