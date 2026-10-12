import { useCallback, useEffect, useRef, useState } from "react"

import { fetchFormat, type FormatState } from "./document-format.ts"

interface FormatEntry {
  source: string
  state: FormatState
}

// useDocumentFormats caches on-demand formatting per path for the session.
// Entries are tied to the source they formatted, so a changed file refetches.
export function useDocumentFormats(onFailure: (path: string) => void) {
  const [entries, setEntries] = useState<ReadonlyMap<string, FormatEntry>>(() => new Map())
  const latest = useRef(new Map<string, FormatEntry>())
  const controllers = useRef(new Set<AbortController>())
  const failure = useRef(onFailure)
  useEffect(() => { failure.current = onFailure }, [onFailure])
  useEffect(() => {
    const pending = controllers.current
    return () => {
      for (const controller of pending) controller.abort()
    }
  }, [])

  const store = useCallback((path: string, entry: FormatEntry) => {
    latest.current.set(path, entry)
    setEntries(new Map(latest.current))
  }, [])

  const request = useCallback((path: string, source: string) => {
    const entry = latest.current.get(path)
    if (entry && entry.source === source && entry.state.kind !== "error") return
    const controller = new AbortController()
    controllers.current.add(controller)
    store(path, { source, state: { kind: "loading" } })
    fetchFormat(path, controller.signal).then((state) => {
      if (latest.current.get(path)?.source !== source) return
      store(path, { source, state })
      if (state.kind === "error") failure.current(path)
    }, () => {
      // Aborted on unmount.
    }).finally(() => controllers.current.delete(controller))
  }, [store])

  const get = useCallback((path: string, source: string): FormatState | undefined => {
    const entry = entries.get(path)
    return entry?.source === source ? entry.state : undefined
  }, [entries])

  return { get, request }
}

// useDelayedFlag reports an active state only after it has lasted `delay` ms.
export function useDelayedFlag(active: boolean, delay: number): boolean {
  const [elapsed, setElapsed] = useState(false)
  useEffect(() => {
    if (!active) return
    const timer = window.setTimeout(() => setElapsed(true), delay)
    return () => {
      window.clearTimeout(timer)
      setElapsed(false)
    }
  }, [active, delay])
  return active && elapsed
}
