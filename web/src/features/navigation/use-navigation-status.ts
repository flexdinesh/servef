import { useCallback, useEffect, useState } from "react"

import { fetchJSON } from "../../api/http.ts"
import { isNavigationStatus, type NavigationStatus } from "../../api/navigation-contract.ts"
import { LANGUAGE_SERVER_HEADER, navigationStatusQuery, pollsServer } from "./navigation-status.ts"

const POLL_MS = 3_000

export interface NavigationStatusState {
  // status is null until loaded or when availability could not be checked.
  status: NavigationStatus | null
  // check looks again for installed servers and retries failed ones.
  check(): void
  // refresh reloads the status, e.g. after a navigation started a server.
  refresh(): void
}

// useNavigationStatus follows who answers navigation for the open file.
// It never starts servers; it polls only while one is starting or running.
export function useNavigationStatus(project: string, path: string, textFile: boolean, languageServers: boolean): NavigationStatusState {
  const [status, setStatus] = useState<NavigationStatus | null>(null)
  const [token, setToken] = useState({ value: 0, refresh: false })

  useEffect(() => { setStatus(null) }, [project, path, textFile, languageServers])

  useEffect(() => {
    if (!textFile || !path) {
      setStatus(null)
      return
    }
    const controller = new AbortController()
    const headers: Record<string, string> = languageServers ? { [LANGUAGE_SERVER_HEADER]: "on" } : {}
    // Any failure shows that availability could not be checked.
    fetchJSON(`/api/navigation-status?${navigationStatusQuery(project, path, token.refresh)}`, { headers, signal: controller.signal },
      isNavigationStatus, () => "Couldn't check language server availability.")
      .then((body) => {
        if (!controller.signal.aborted) setStatus(body)
      }, () => {
        if (!controller.signal.aborted) setStatus(null)
      })
    return () => controller.abort()
  }, [languageServers, textFile, path, project, token])

  const polling = pollsServer(status)
  useEffect(() => {
    if (!polling) return
    const timer = window.setInterval(() => {
      if (!document.hidden) setToken((current) => ({ value: current.value + 1, refresh: false }))
    }, POLL_MS)
    return () => window.clearInterval(timer)
  }, [polling])

  const check = useCallback(() => setToken((current) => ({ value: current.value + 1, refresh: true })), [])
  const refresh = useCallback(() => setToken((current) => ({ value: current.value + 1, refresh: false })), [])
  return { status, check, refresh }
}
