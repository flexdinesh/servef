import { useCallback, useEffect, useRef, useState } from "react"

import { definitionOutcome, emptyMessage, navigationQuery, type NavigationKind } from "./code-navigation.ts"
import { fetchJSON, type RequestFailure } from "./http.ts"
import { isNavigationResult, type NavigationLocation, type NavigationResult } from "./navigation-contract.ts"
import { LANGUAGE_SERVER_HEADER } from "./navigation-status.ts"
import type { SourceTarget } from "./source-position.ts"

export type NavigationPanel =
  | { status: "closed" }
  | { status: "loading"; kind: NavigationKind; symbol: string }
  | { status: "results"; kind: NavigationKind; result: NavigationResult }
  | { status: "message"; kind: NavigationKind; message: string; tone: "info" | "error" }

export interface CodeNavigation {
  panel: NavigationPanel
  // request runs a navigation from a target in a file.
  request(kind: NavigationKind, path: string, target: SourceTarget): void
  // explain shows why a navigation could not run.
  explain(kind: NavigationKind, message: string): void
  close(): void
}

// navigationError keeps the server's reason and never shows raw transport text.
function navigationError(failure: RequestFailure): string {
  switch (failure.kind) {
    case "unreachable":
      return "Code navigation failed: the server didn't respond."
    case "rejected":
      return failure.error
    case "malformed":
    case "invalid":
      return failure.ok ? "The server sent an unexpected navigation response." : "Code navigation failed."
  }
}

function fetchNavigation(kind: NavigationKind, project: string, path: string, target: SourceTarget, languageServers: boolean, signal: AbortSignal): Promise<NavigationResult> {
  const endpoint = kind === "definition" ? "/api/definition" : "/api/references"
  const headers: Record<string, string> = languageServers ? { [LANGUAGE_SERVER_HEADER]: "on" } : {}
  return fetchJSON(`${endpoint}?${navigationQuery(project, path, target.position)}`, { headers, signal }, isNavigationResult, navigationError)
}

// useCodeNavigation runs go to definition and find references. One
// request is live at a time; a newer one cancels it. languageServers opts
// requests into language servers; settled runs after each request, which
// may have started one.
export function useCodeNavigation(project: string, languageServers: boolean, openLocation: (location: NavigationLocation) => void, settled: () => void): CodeNavigation {
  const [panel, setPanel] = useState<NavigationPanel>({ status: "closed" })
  const controller = useRef<AbortController | null>(null)
  useEffect(() => () => controller.current?.abort(), [])

  const close = useCallback(() => {
    controller.current?.abort()
    setPanel({ status: "closed" })
  }, [])

  const explain = useCallback((kind: NavigationKind, message: string) => {
    controller.current?.abort()
    setPanel({ status: "message", kind, message, tone: "info" })
  }, [])

  const request = useCallback((kind: NavigationKind, path: string, target: SourceTarget) => {
    controller.current?.abort()
    const current = new AbortController()
    controller.current = current
    const run = async (step: NavigationKind) => {
      setPanel({ status: "loading", kind: step, symbol: target.text })
      const result = await fetchNavigation(step, project, path, target, languageServers, current.signal)
      if (current.signal.aborted) return
      settled()
      if (step === "references") {
        setPanel(result.locations.length
          ? { status: "results", kind: step, result }
          : { status: "message", kind: step, message: emptyMessage(step, result), tone: "info" })
        return
      }
      if (result.locations.length > 0 && (result.filtered > 0 || result.truncated)) {
        setPanel({ status: "results", kind: step, result })
        return
      }
      const outcome = definitionOutcome(result, path, target.position)
      switch (outcome.kind) {
        case "none":
          setPanel({ status: "message", kind: step, message: emptyMessage(step, result), tone: "info" })
          return
        case "open":
          setPanel({ status: "closed" })
          openLocation(outcome.location)
          return
        case "choose":
          setPanel({ status: "results", kind: step, result })
          return
        case "references":
          await run("references")
      }
    }
    run(kind).catch((error: unknown) => {
      if (current.signal.aborted) return
      settled()
      setPanel({ status: "message", kind, message: error instanceof Error ? error.message : "Code navigation failed.", tone: "error" })
    })
  }, [languageServers, openLocation, project, settled])

  return { panel, request, explain, close }
}
