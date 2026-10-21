import { useCallback, useEffect, useMemo, useRef, useState } from "react"

import { gitStatusError, isChangesResponse, isDiffErrorResponse, type ChangedFile, type ChangesResponse } from "./changes-contract.ts"
import { projectUrl } from "./project-url.ts"

// Focus and visibility refreshes run at most this often.
const focusThrottle = 2_000

export interface ChangesState {
  // result is the last list read; it stays while a refresh runs or fails.
  result: ChangesResponse | null
  error: string
  loading: boolean
}

async function fetchChanges(project: string, signal: AbortSignal): Promise<ChangesResponse> {
  let response: Response
  try {
    response = await fetch(projectUrl("/api/changes", project), { headers: { Accept: "application/json" }, signal })
  } catch (error: unknown) {
    if (error instanceof Error && error.name === "AbortError") throw error
    throw new Error(gitStatusError("the server didn't respond"))
  }
  const payload: unknown = await response.json().catch(() => null)
  if (response.ok && isChangesResponse(payload)) return payload
  throw new Error(gitStatusError(isDiffErrorResponse(payload) ? payload.error : "the server sent an invalid response"))
}

// useChanges owns the project's working-tree change list. It reads on project
// load, when the Source Control view opens, on window focus or visibility
// (throttled), and on refresh. It never polls.
export function useChanges(project: string, viewActive: boolean) {
  const [state, setState] = useState<ChangesState>({ result: null, error: "", loading: project !== "" })
  const controller = useRef<AbortController | null>(null)
  const lastRead = useRef(0)

  const refresh = useCallback(() => {
    if (!project) return
    controller.current?.abort()
    const current = new AbortController()
    controller.current = current
    lastRead.current = Date.now()
    setState((previous) => previous.loading ? previous : { ...previous, loading: true })
    fetchChanges(project, current.signal).then((result) => {
      if (controller.current === current) setState({ result, error: "", loading: false })
    }, (error: unknown) => {
      if (controller.current !== current || current.signal.aborted) return
      setState((previous) => ({ ...previous, error: error instanceof Error ? error.message : gitStatusError("unknown error"), loading: false }))
    })
  }, [project])

  useEffect(() => {
    refresh()
    return () => controller.current?.abort()
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

  const files = state.result?.files
  const byPath = useMemo(() => new Map((files ?? []).map((file): [string, ChangedFile] => [file.path, file])), [files])
  return { ...state, byPath, refresh }
}

export type Changes = ReturnType<typeof useChanges>
