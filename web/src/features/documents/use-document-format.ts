import { useCallback, useEffect, useRef, useState } from "react"

import { fetchFormat, type FormatState } from "./document-format.ts"

interface FormatEntry {
  source: string
  state: FormatState
}

// useDocumentFormats caches on-demand formatting per path for the project's
// session. Entries are tied to the source they formatted, so a changed file
// refetches.
export function useDocumentFormats(project: string, onFailure: (path: string) => void) {
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
    fetchFormat(project, path, controller.signal).then((state) => {
      if (latest.current.get(path)?.source !== source) return
      store(path, { source, state })
      if (state.kind === "error") failure.current(path)
    }, () => {
      // Aborted on unmount.
    }).finally(() => controllers.current.delete(controller))
  }, [project, store])

  const get = useCallback((path: string, source: string): FormatState | undefined => {
    const entry = entries.get(path)
    return entry?.source === source ? entry.state : undefined
  }, [entries])

  return { get, request }
}
