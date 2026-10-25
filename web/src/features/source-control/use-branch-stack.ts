import { useCallback, useEffect, useRef, useState } from "react"

import { historyError } from "../../api/commits-contract.ts"
import { failureReason, fetchJSON } from "../../api/http.ts"
import { isStackResponse, type StackResponse } from "../../api/stack-contract.ts"
import { useRefreshOnReturn } from "./use-refresh-on-return.ts"

export interface BranchStackState {
  // result is the last stack read for the base; it stays while a refresh runs
  // or fails.
  result: StackResponse | null
  error: string
  loading: boolean
}

function stackUrl(project: string, base: string): string {
  const params = new URLSearchParams({ project })
  if (base) params.set("base", base)
  return `/api/branch-stack?${params.toString()}`
}

// useBranchStack owns the project's branch stack against base ("" for the
// default; decision 0040). It reads on project load, when Source Control
// opens, on window focus or visibility (throttled), on refresh, and when the
// base changes. It never polls.
export function useBranchStack(project: string, viewActive: boolean, base: string) {
  const [state, setState] = useState<BranchStackState>({ result: null, error: "", loading: project !== "" })
  const controller = useRef<AbortController | null>(null)
  const lastRead = useRef(0)

  const refresh = useCallback(() => {
    if (!project) return
    controller.current?.abort()
    const current = new AbortController()
    controller.current = current
    lastRead.current = Date.now()
    setState((previous) => previous.loading ? previous : { ...previous, loading: true })
    fetchJSON(stackUrl(project, base), { signal: current.signal }, isStackResponse, (failure) => historyError(failureReason(failure))).then((result) => {
      if (controller.current === current) setState({ result, error: "", loading: false })
    }, (error: unknown) => {
      if (controller.current !== current || current.signal.aborted) return
      setState((previous) => ({ ...previous, error: error instanceof Error ? error.message : historyError("unknown error"), loading: false }))
    })
  }, [base, project])

  useEffect(() => {
    refresh()
    return () => controller.current?.abort()
  }, [refresh])

  useRefreshOnReturn(refresh, { lastRead, viewActive })

  return { ...state, refresh }
}

export type BranchStack = ReturnType<typeof useBranchStack>
