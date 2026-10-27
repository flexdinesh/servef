import { useCallback, useEffect, useRef, useState } from "react"

import { deleteResource, failureReason, fetchJSON, isAbort, RequestError } from "../../api/http.ts"
import { projectUrl } from "../../api/project-url.ts"
import {
  isWalkthrough, isWalkthroughSummaries, walkthroughUnavailableCode, type Walkthrough, type WalkthroughSummary,
} from "../../api/walkthrough-contract.ts"

// pollInterval is how often the list rereads while the page is visible:
// agents publish without the browser knowing (decision 0047).
const pollInterval = 5_000

export interface WalkthroughsState {
  result: WalkthroughSummary[] | null
  error: string
  loading: boolean
}

function listError(reason: string): string {
  return `Couldn't read walkthroughs: ${reason}.`
}

// useWalkthroughs lists a project's walkthroughs, rereading every few
// seconds while the page is visible and whenever it returns to view.
export function useWalkthroughs(project: string) {
  const [state, setState] = useState<WalkthroughsState>({ result: null, error: "", loading: false })
  const controller = useRef<AbortController | null>(null)
  const load = useCallback(() => {
    if (!project) return
    controller.current?.abort()
    const current = new AbortController()
    controller.current = current
    setState((previous) => ({ ...previous, loading: true }))
    fetchJSON(projectUrl("/api/walkthroughs", project), { signal: current.signal }, isWalkthroughSummaries, (failure) => listError(failureReason(failure)))
      .then((result) => {
        if (controller.current === current) setState({ result, error: "", loading: false })
      }, (error: unknown) => {
        if (controller.current !== current || isAbort(error)) return
        setState((previous) => ({ ...previous, error: error instanceof Error ? error.message : listError("unknown error"), loading: false }))
      })
  }, [project])

  useEffect(() => {
    load()
    let timer = 0
    const schedule = () => {
      window.clearInterval(timer)
      if (document.visibilityState === "visible") timer = window.setInterval(load, pollInterval)
    }
    const returned = () => {
      if (document.visibilityState === "visible") load()
      schedule()
    }
    schedule()
    document.addEventListener("visibilitychange", returned)
    window.addEventListener("focus", load)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener("visibilitychange", returned)
      window.removeEventListener("focus", load)
      controller.current?.abort()
    }
  }, [load])

  // remove deletes a walkthrough, then rereads the list.
  const remove = useCallback(async (id: string) => {
    const url = `/api/walkthroughs?${new URLSearchParams({ project, id }).toString()}`
    await deleteResource(url, {}, (failure) => `Couldn't delete the walkthrough: ${failureReason(failure)}.`)
    load()
  }, [load, project])

  return { ...state, refresh: load, remove }
}

export type Walkthroughs = ReturnType<typeof useWalkthroughs>

export type WalkthroughDetail =
  | { kind: "none" }
  | { kind: "loading" }
  | { kind: "ready"; walkthrough: Walkthrough }
  | { kind: "missing" }
  | { kind: "error"; message: string }

function detailError(reason: string): string {
  return `Couldn't read the walkthrough: ${reason}.`
}

// useWalkthrough reads one walkthrough, again whenever version (its listed
// updatedAt) changes, keeping the last one shown while it rereads.
export function useWalkthrough(project: string, id: string, version: string) {
  const [detail, setDetail] = useState<{ id: string; detail: WalkthroughDetail }>({ id: "", detail: { kind: "none" } })
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    if (!project || !id) return
    const controller = new AbortController()
    setDetail((previous) => previous.id === id && previous.detail.kind === "ready" ? previous : { id, detail: { kind: "loading" } })
    const url = `/api/walkthrough?${new URLSearchParams({ project, id }).toString()}`
    fetchJSON(url, { signal: controller.signal }, isWalkthrough, (failure) => detailError(failureReason(failure)))
      .then((walkthrough) => setDetail({ id, detail: { kind: "ready", walkthrough } }), (error: unknown) => {
        if (isAbort(error)) return
        const missing = error instanceof RequestError && error.failure.kind === "rejected" && error.failure.code === walkthroughUnavailableCode
        setDetail({ id, detail: missing ? { kind: "missing" } : { kind: "error", message: error instanceof Error ? error.message : detailError("unknown error") } })
      })
    return () => controller.abort()
  }, [attempt, id, project, version])
  const retry = useCallback(() => setAttempt((current) => current + 1), [])
  return { detail: id && detail.id === id ? detail.detail : { kind: "none" } satisfies WalkthroughDetail, retry }
}
