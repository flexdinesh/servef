import { useCallback, useEffect, useMemo, useRef, useState } from "react"

import { gitStatusError, isChangesResponse, type ChangedFile, type ChangesResponse } from "../../api/changes-contract.ts"
import { failureReason, fetchJSON } from "../../api/http.ts"
import { projectUrl } from "../../api/project-url.ts"
import { useRefreshOnReturn } from "./use-refresh-on-return.ts"

export interface ChangesState {
  // result is the last list read; it stays while a refresh runs or fails.
  result: ChangesResponse | null
  error: string
  loading: boolean
  // read counts successful reads; stats and diffs of the working tree reread
  // when it moves.
  read: number
}

function fetchChanges(project: string, signal: AbortSignal): Promise<ChangesResponse> {
  return fetchJSON(projectUrl("/api/changes", project), { signal }, isChangesResponse, (failure) => gitStatusError(failureReason(failure)))
}

// useChanges owns the project's working-tree change list. It reads on project
// load, when the Source Control view opens, on window focus or visibility
// (throttled), and on refresh. It never polls.
export function useChanges(project: string, viewActive: boolean) {
  const [state, setState] = useState<ChangesState>({ result: null, error: "", loading: project !== "", read: 0 })
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
      if (controller.current === current) setState((previous) => ({ result, error: "", loading: false, read: previous.read + 1 }))
    }, (error: unknown) => {
      if (controller.current !== current || current.signal.aborted) return
      setState((previous) => ({ ...previous, error: error instanceof Error ? error.message : gitStatusError("unknown error"), loading: false }))
    })
  }, [project])

  useEffect(() => {
    refresh()
    return () => controller.current?.abort()
  }, [refresh])

  useRefreshOnReturn(refresh, { lastRead, viewActive })

  const files = state.result?.files
  const byPath = useMemo(() => new Map((files ?? []).map((file): [string, ChangedFile] => [file.path, file])), [files])
  return { ...state, byPath, refresh }
}

export type Changes = ReturnType<typeof useChanges>
