import { useCallback, useEffect, useRef, useState } from "react"

import { definitionOutcome, emptyMessage, navigationQuery, type NavigationKind } from "./code-navigation.ts"
import { isNavigationResult, type NavigationLocation, type NavigationResult } from "./navigation-contract.ts"
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

function errorMessage(value: unknown): string {
  return typeof value === "object" && value !== null && "error" in value && typeof value.error === "string"
    ? value.error
    : "Code navigation failed."
}

async function fetchNavigation(kind: NavigationKind, project: string, path: string, target: SourceTarget, signal: AbortSignal): Promise<NavigationResult> {
  const endpoint = kind === "definition" ? "/api/definition" : "/api/references"
  const response = await fetch(`${endpoint}?${navigationQuery(project, path, target.position)}`, { signal })
  const body: unknown = await response.json()
  if (!response.ok) throw new Error(errorMessage(body))
  if (!isNavigationResult(body)) throw new Error("The server sent an unexpected navigation response.")
  return body
}

// useCodeNavigation runs go to definition and find references. One
// request is live at a time; a newer one cancels it.
export function useCodeNavigation(project: string, openLocation: (location: NavigationLocation) => void): CodeNavigation {
  const [panel, setPanel] = useState<NavigationPanel>({ status: "closed" })
  const controller = useRef<AbortController | null>(null)
  useEffect(() => () => controller.current?.abort(), [])

  const close = useCallback(() => {
    controller.current?.abort()
    setPanel({ status: "closed" })
  }, [])

  const explain = useCallback((kind: NavigationKind, message: string) => {
    setPanel({ status: "message", kind, message, tone: "info" })
  }, [])

  const request = useCallback((kind: NavigationKind, path: string, target: SourceTarget) => {
    controller.current?.abort()
    const current = new AbortController()
    controller.current = current
    const run = async (step: NavigationKind) => {
      if (step === "references") setPanel({ status: "loading", kind: step, symbol: target.text })
      const result = await fetchNavigation(step, project, path, target, current.signal)
      if (current.signal.aborted) return
      if (step === "references") {
        setPanel(result.locations.length
          ? { status: "results", kind: step, result }
          : { status: "message", kind: step, message: emptyMessage(step, result), tone: "info" })
        return
      }
      const outcome = definitionOutcome(result, path, target.position, target.word)
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
      setPanel({ status: "message", kind, message: error instanceof Error ? error.message : "Code navigation failed.", tone: "error" })
    })
  }, [openLocation, project])

  return { panel, request, explain, close }
}
