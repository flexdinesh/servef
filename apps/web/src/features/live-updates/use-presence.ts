import { useEffect } from "react"

import { fetchProjects } from "../../api/projects-data.ts"

const retryDelay = 5_000

export interface PresenceStream {
  // closed reports that the stream gave up and will not reconnect by itself.
  closed(): boolean
  onError(listener: () => void): void
  close(): void
}

export interface PresenceDeps {
  // wanted asks the server whether its lifetime follows its pages.
  wanted(signal: AbortSignal): Promise<boolean>
  open(): PresenceStream
  schedule(run: () => void, delay: number): () => void
}

// followPresence holds the presence stream for as long as it is not stopped,
// which tells a server with no other reason to run that a page is open
// (decision 0098). A browser gives up on a stream answered with an error, as
// the desktop app answers while a server is away, so reopening is on us: a
// server that came back and saw no page would close under an open window.
export function followPresence(deps: PresenceDeps): () => void {
  const controller = new AbortController()
  let stopped = false
  let stream: PresenceStream | null = null
  let cancelRetry: (() => void) | null = null
  const later = (run: () => void) => {
    cancelRetry = deps.schedule(() => {
      cancelRetry = null
      run()
    }, retryDelay)
  }
  const hold = () => {
    if (stopped) return
    const current = deps.open()
    stream = current
    current.onError(() => {
      if (stopped || !current.closed()) return
      current.close()
      later(hold)
    })
  }
  const ask = () => {
    if (stopped) return
    deps.wanted(controller.signal).then((wanted) => {
      if (wanted) hold()
    }, () => {
      later(ask)
    })
  }
  ask()
  return () => {
    stopped = true
    controller.abort()
    cancelRetry?.()
    stream?.close()
  }
}

// usePresence keeps the presence stream open while this page is, including
// while it is hidden: a hidden window is still open. Mount it once.
export function usePresence(): void {
  useEffect(() => followPresence({
    wanted: async (signal) => (await fetchProjects(signal)).presence,
    open: () => {
      const source = new EventSource("/api/presence")
      return {
        closed: () => source.readyState === EventSource.CLOSED,
        onError: (listener) => source.addEventListener("error", listener),
        close: () => source.close(),
      }
    },
    schedule: (run, delay) => {
      const timer = setTimeout(run, delay)
      return () => clearTimeout(timer)
    },
  }), [])
}
