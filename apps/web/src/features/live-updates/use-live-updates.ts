import { useEffect, useLayoutEffect, useRef, useState } from "react"

import { isLiveChange, isLiveStatus, type LiveChange, type LiveStatus } from "../../api/events-contract.ts"
import { projectUrl } from "../../api/project-url.ts"
import { everything } from "./live-updates.ts"

// Returns to the page reread at most this often.
const returnThrottle = 2_000
// A stream the browser gave up on is reopened after this long.
const retryDelay = 5_000

const reconnecting: LiveStatus = { mode: "off", reason: "Reconnecting to Source Zero…" }

export interface LiveStream {
  // closed reports that the stream gave up and will not reconnect by itself.
  closed(): boolean
  close(): void
}

export interface LiveStreamHandlers {
  status(value: LiveStatus): void
  change(value: LiveChange): void
  error(): void
}

export interface LiveDeps {
  open(handlers: LiveStreamHandlers): LiveStream
  visible(): boolean
  now(): number
  schedule(run: () => void, delay: number): () => void
  status(value: LiveStatus): void
  change(value: LiveChange): void
}

export interface LiveFollower {
  // visibility and focus report the page's events of those names.
  visibility(): void
  focus(): void
  stop(): void
}

// followLiveUpdates follows a project's changes while the page is visible
// (decision 0050). Hidden, it closes its stream; on return, and after a
// reconnect, it reports everything changed, since changes meanwhile went
// unseen. Without a stream, window focus rereads instead. A browser gives up on
// a stream answered with an error, as the desktop app answers while a server is
// away, so reopening is on us (decision 0098).
export function followLiveUpdates(deps: LiveDeps): LiveFollower {
  let source: LiveStream | null = null
  let cancelRetry: (() => void) | null = null
  // following is true while a stream delivers changes.
  let following = false
  let missed = false
  let lastReturn = 0
  const returned = () => {
    if (deps.now() - lastReturn < returnThrottle) return
    lastReturn = deps.now()
    deps.change(everything)
  }
  const open = () => {
    const current = deps.open({
      status(value) {
        following = value.mode !== "off"
        deps.status(value)
        if (missed) {
          missed = false
          deps.change(everything)
        }
      },
      change: deps.change,
      error() {
        following = false
        missed = true
        deps.status(reconnecting)
        if (!current.closed()) return
        current.close()
        source = null
        cancelRetry = deps.schedule(() => {
          cancelRetry = null
          open()
        }, retryDelay)
      },
    })
    source = current
  }
  const close = () => {
    cancelRetry?.()
    cancelRetry = null
    source?.close()
    source = null
    following = false
  }
  if (deps.visible()) open()
  return {
    visibility() {
      if (!deps.visible()) {
        close()
      } else if (!source && !cancelRetry) {
        returned()
        open()
      }
    },
    focus() {
      if (!following) returned()
    },
    stop: close,
  }
}

function eventData(event: Event): unknown {
  if (!(event instanceof MessageEvent) || typeof event.data !== "string") return undefined
  try {
    const value: unknown = JSON.parse(event.data)
    return value
  } catch {
    return undefined
  }
}

// useLiveUpdates runs followLiveUpdates for project and calls onChange with
// each batch. Mount it once per project.
export function useLiveUpdates(project: string, onChange: (change: LiveChange) => void): LiveStatus | null {
  const [status, setStatus] = useState<LiveStatus | null>(null)
  const latest = useRef(onChange)
  useLayoutEffect(() => { latest.current = onChange })

  useEffect(() => {
    if (!project) return
    const follower = followLiveUpdates({
      open(handlers) {
        const source = new EventSource(projectUrl("/api/events", project))
        source.addEventListener("status", (event) => {
          const value = eventData(event)
          if (isLiveStatus(value)) handlers.status(value)
        })
        source.addEventListener("change", (event) => {
          const value = eventData(event)
          if (isLiveChange(value)) handlers.change(value)
        })
        source.addEventListener("error", () => handlers.error())
        return {
          closed: () => source.readyState === EventSource.CLOSED,
          close: () => source.close(),
        }
      },
      visible: () => document.visibilityState === "visible",
      now: () => Date.now(),
      schedule(run, delay) {
        const timer = setTimeout(run, delay)
        return () => clearTimeout(timer)
      },
      status: setStatus,
      change: (change) => latest.current(change),
    })
    const visibility = () => follower.visibility()
    const focus = () => follower.focus()
    document.addEventListener("visibilitychange", visibility)
    window.addEventListener("focus", focus)
    return () => {
      follower.stop()
      document.removeEventListener("visibilitychange", visibility)
      window.removeEventListener("focus", focus)
    }
  }, [project])

  return status
}
