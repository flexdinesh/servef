import { useEffect, useLayoutEffect, useRef, useState } from "react"

import { isLiveChange, isLiveStatus, type LiveChange, type LiveStatus } from "../../api/events-contract.ts"
import { projectUrl } from "../../api/project-url.ts"
import { everything } from "./live-updates.ts"

// Returns to the page reread at most this often.
const returnThrottle = 2_000

const reconnecting: LiveStatus = { mode: "off", reason: "Reconnecting to servef…" }
const unavailable: LiveStatus = { mode: "off", reason: "servef did not start live updates for this project." }

function eventData(event: Event): unknown {
  if (!(event instanceof MessageEvent) || typeof event.data !== "string") return undefined
  try {
    const value: unknown = JSON.parse(event.data)
    return value
  } catch {
    return undefined
  }
}

// useLiveUpdates follows a project's changes while the page is visible and
// calls onChange with each batch (decision 0050). Hidden, it closes its
// stream; on return, and after a reconnect, it reports everything changed,
// since changes meanwhile went unseen. Without a stream, window focus
// rereads instead. Mount it once per project.
export function useLiveUpdates(project: string, onChange: (change: LiveChange) => void): LiveStatus | null {
  const [status, setStatus] = useState<LiveStatus | null>(null)
  const latest = useRef(onChange)
  useLayoutEffect(() => { latest.current = onChange })

  useEffect(() => {
    if (!project) return
    let source: EventSource | null = null
    // following is true while a stream delivers changes.
    let following = false
    let missed = false
    let lastReturn = 0
    const deliver = (change: LiveChange) => latest.current(change)
    const returned = () => {
      if (Date.now() - lastReturn < returnThrottle) return
      lastReturn = Date.now()
      deliver(everything)
    }
    const open = () => {
      const current = new EventSource(projectUrl("/api/events", project))
      source = current
      current.addEventListener("status", (event) => {
        const value = eventData(event)
        if (!isLiveStatus(value)) return
        following = value.mode !== "off"
        setStatus(value)
        if (missed) {
          missed = false
          deliver(everything)
        }
      })
      current.addEventListener("change", (event) => {
        const value = eventData(event)
        if (isLiveChange(value)) deliver(value)
      })
      current.addEventListener("error", () => {
        following = false
        missed = true
        setStatus(current.readyState === EventSource.CLOSED ? unavailable : reconnecting)
      })
    }
    const close = () => {
      source?.close()
      source = null
      following = false
    }
    const visibility = () => {
      if (document.visibilityState !== "visible") {
        close()
      } else if (!source) {
        returned()
        open()
      }
    }
    const focus = () => {
      if (!following) returned()
    }
    if (document.visibilityState === "visible") open()
    document.addEventListener("visibilitychange", visibility)
    window.addEventListener("focus", focus)
    return () => {
      close()
      document.removeEventListener("visibilitychange", visibility)
      window.removeEventListener("focus", focus)
    }
  }, [project])

  return status
}
