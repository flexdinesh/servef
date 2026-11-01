import { isLiveChange, isLiveHead, isLiveStatus } from "../../../web/src/api/events-contract.ts"
import type { LiveChange, LiveHead, LiveStatus } from "../../../web/src/api/events-contract.ts"
import type { DaemonConnection } from "./connections.ts"
import { isControlStatus, isWalkthroughAnnouncement } from "./daemon-contract.ts"
import type { ControlStatus, WalkthroughAnnouncement } from "./daemon-contract.ts"
import { isDaemonGone } from "./forward.ts"
import type { Forward } from "./forward.ts"
import { createSSEParser } from "./sse.ts"
import type { TimerHandle, Timers } from "./timers.ts"

export type Lease = "open" | "watched"

export type SubscriptionEvent =
  | { kind: "status"; project: string; lease: Lease; status: LiveStatus }
  | { kind: "head"; project: string; head: LiveHead }
  | { kind: "change"; project: string; lease: Lease; change: LiveChange }
  | { kind: "control"; status: ControlStatus }
  | { kind: "walkthrough"; announcement: WalkthroughAnnouncement }

export const MAX_WATCHED = 16
export const BACKOFF_MIN_MS = 1_000
export const BACKOFF_MAX_MS = 30_000

export interface SubscriptionDeps {
  connection(): Promise<DaemonConnection | null>
  reensure(): Promise<boolean>
  forward: Forward
  timers: Timers
  onEvent(event: SubscriptionEvent): void
}

export interface Subscriptions {
  // shown sets the projects any window shows (open leases). Projects that
  // leave keep a watched lease, least recent dropped past MAX_WATCHED.
  shown(projects: readonly string[]): void
  close(): void
}

interface Stream {
  close(): void
}

// Streams address the daemon directly: no Origin or Sec-Fetch-Site, which
// the control stream refuses.
const STREAM_HEADERS = { accept: "text/event-stream", host: "localhost" }

// createSubscriptions opens /_daemon/events at once and keeps every stream
// reconnecting with backoff (BACKOFF_MIN_MS doubling to BACKOFF_MAX_MS).
export function createSubscriptions(deps: SubscriptionDeps): Subscriptions {
  let closed = false
  const projects = new Map<string, { lease: Lease; stream: Stream }>()
  let shownSet = new Set<string>()
  // watched lists projects that left every window, least recent first.
  let watched: string[] = []

  // open runs one stream until closed: connect, parse, and on end or error
  // re-ensure when the daemon is gone, then retry after the backoff.
  const open = (path: string, dispatch: (name: string, data: unknown) => void): Stream => {
    let stopped = false
    let backoff = BACKOFF_MIN_MS
    let timer: TimerHandle | null = null
    let cancel: (() => void) | null = null

    const connect = async () => {
      timer = null
      let gone = false
      let delivered = false
      try {
        const connection = await deps.connection()
        if (stopped) return
        if (connection === null) {
          gone = true
        } else {
          const response = await deps.forward({ target: { socket: connection.socket }, method: "GET", path, headers: { ...STREAM_HEADERS }, body: null, stream: true })
          const body = response.body
          if (stopped || !response.ok || body === null) {
            await body?.cancel()
          } else {
            const reader = body.getReader()
            cancel = () => { void reader.cancel().catch(() => undefined) }
            const decoder = new TextDecoder()
            const parse = createSSEParser((name, data) => {
              if (stopped) return
              delivered = true
              let value: unknown
              try {
                value = JSON.parse(data)
              } catch {
                return
              }
              dispatch(name, value)
            })
            for (;;) {
              const { done, value } = await reader.read()
              if (done || stopped) break
              parse(decoder.decode(value, { stream: true }))
            }
          }
        }
      } catch (error) {
        if (isDaemonGone(error)) gone = true
      }
      cancel = null
      if (stopped) return
      if (delivered) backoff = BACKOFF_MIN_MS
      if (gone) await deps.reensure().catch(() => false)
      if (stopped) return
      const delay = backoff
      backoff = Math.min(backoff * 2, BACKOFF_MAX_MS)
      timer = deps.timers.setTimeout(() => { void connect() }, delay)
    }

    void connect()
    return {
      close() {
        stopped = true
        if (timer !== null) deps.timers.clearTimeout(timer)
        timer = null
        cancel?.()
        cancel = null
      },
    }
  }

  const openProject = (project: string, lease: Lease): Stream => {
    const query = new URLSearchParams({ project, lease }).toString()
    return open(`/api/events?${query}`, (name, data) => {
      if (name === "status" && isLiveStatus(data)) deps.onEvent({ kind: "status", project, lease, status: data })
      else if (name === "head" && isLiveHead(data)) deps.onEvent({ kind: "head", project, head: data })
      else if (name === "change" && isLiveChange(data)) deps.onEvent({ kind: "change", project, lease, change: data })
    })
  }

  const control = open("/_daemon/events", (name, data) => {
    if (name === "status" && isControlStatus(data)) deps.onEvent({ kind: "control", status: data })
    else if (name === "walkthrough" && isWalkthroughAnnouncement(data)) deps.onEvent({ kind: "walkthrough", announcement: data })
  })

  const reconcile = () => {
    const desired = new Map<string, Lease>()
    for (const project of watched) desired.set(project, "watched")
    for (const project of shownSet) desired.set(project, "open")
    for (const [project, current] of projects) {
      if (desired.get(project) === current.lease) continue
      current.stream.close()
      projects.delete(project)
    }
    for (const [project, lease] of desired) {
      if (!projects.has(project)) projects.set(project, { lease, stream: openProject(project, lease) })
    }
  }

  return {
    shown(next) {
      if (closed) return
      const nextSet = new Set(next)
      const left = [...shownSet].filter((project) => !nextSet.has(project))
      watched = [...watched.filter((project) => !nextSet.has(project) && !left.includes(project)), ...left]
      if (watched.length > MAX_WATCHED) watched = watched.slice(watched.length - MAX_WATCHED)
      shownSet = nextSet
      reconcile()
    },
    close() {
      if (closed) return
      closed = true
      control.close()
      for (const { stream } of projects.values()) stream.close()
      projects.clear()
    },
  }
}
