import { useCallback, useEffect, useRef, useState } from "react"

import { isDiffErrorResponse, isDiffResponse, type DiffResponse } from "./changes-contract.ts"

// Focus refetches run at most this often.
const focusThrottle = 2_000

export type DiffState =
  | { kind: "loading" }
  | { kind: "ready"; diff: DiffResponse }
  // unchanged: the tracked file matches HEAD again.
  | { kind: "unchanged"; message: string }
  | { kind: "error"; message: string }

function diffUrl(project: string, path: string, oldPath: string): string {
  const params = new URLSearchParams({ project, path })
  if (oldPath) params.set("oldPath", oldPath)
  return `/api/diff?${params.toString()}`
}

async function fetchDiff(project: string, path: string, oldPath: string, signal: AbortSignal): Promise<DiffState> {
  let response: Response
  try {
    response = await fetch(diffUrl(project, path, oldPath), { headers: { Accept: "application/json" }, signal })
  } catch (error: unknown) {
    if (error instanceof Error && error.name === "AbortError") throw error
    return { kind: "error", message: "Couldn't load this diff: the server didn't respond." }
  }
  const payload: unknown = await response.json().catch(() => null)
  if (response.ok && isDiffResponse(payload)) return { kind: "ready", diff: payload }
  if (isDiffErrorResponse(payload)) {
    return payload.code === "unchanged" ? { kind: "unchanged", message: payload.error } : { kind: "error", message: payload.error }
  }
  return { kind: "error", message: "Couldn't load this diff: the server sent an invalid response." }
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
// the shown diff until its answer arrives.
export function useWorkingTreeDiff(project: string, path: string, oldPath: string) {
  const key = path ? `${path}\n${oldPath}` : ""
  const [entry, setEntry] = useState<{ key: string; state: DiffState } | null>(null)
  const [loading, setLoading] = useState(false)
  const [request, setRequest] = useState(0)
  const lastRead = useRef(0)
  // Leaving diffs drops the last response; activation refetches anyway.
  if (!key && entry) setEntry(null)

  useEffect(() => {
    if (!project || !path) return
    const controller = new AbortController()
    lastRead.current = Date.now()
    setLoading(true)
    fetchDiff(project, path, oldPath, controller.signal).then((state) => {
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
  }, [key, oldPath, path, project, request])

  useEffect(() => {
    if (!path) return
    const onReturn = () => {
      if (document.visibilityState !== "visible" || Date.now() - lastRead.current < focusThrottle) return
      setRequest((count) => count + 1)
    }
    window.addEventListener("focus", onReturn)
    document.addEventListener("visibilitychange", onReturn)
    return () => {
      window.removeEventListener("focus", onReturn)
      document.removeEventListener("visibilitychange", onReturn)
    }
  }, [path])

  const retry = useCallback(() => setRequest((count) => count + 1), [])
  const state: DiffState = entry?.key === key && key ? entry.state : { kind: "loading" }
  return { state, loading: Boolean(key) && loading, retry }
}

export type WorkingTreeDiff = ReturnType<typeof useWorkingTreeDiff>
