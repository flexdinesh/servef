import { useCallback, useRef, useState } from "react"

import { historyError } from "../../api/commits-contract.ts"
import { failureReason, fetchJSON, isAbort } from "../../api/http.ts"
import { projectUrl } from "../../api/project-url.ts"
import { isRefsResponse, type RefsResponse } from "../../api/refs-contract.ts"

export interface RefsState {
  result: RefsResponse | null
  error: string
  loading: boolean
}

// useRefs reads the project's branches for the base picker when it opens,
// never before.
export function useRefs(project: string) {
  const [state, setState] = useState<RefsState>({ result: null, error: "", loading: false })
  const controller = useRef<AbortController | null>(null)
  const load = useCallback(() => {
    if (!project) return
    controller.current?.abort()
    const current = new AbortController()
    controller.current = current
    setState((previous) => ({ ...previous, loading: true, error: "" }))
    fetchJSON(projectUrl("/api/refs", project), { signal: current.signal }, isRefsResponse, (failure) => historyError(failureReason(failure)))
      .then((result) => {
        if (controller.current === current) setState({ result, error: "", loading: false })
      }, (error: unknown) => {
        if (controller.current !== current || isAbort(error)) return
        setState((previous) => ({ ...previous, error: error instanceof Error ? error.message : historyError("unknown error"), loading: false }))
      })
  }, [project])
  return { ...state, load }
}

export type Refs = ReturnType<typeof useRefs>
