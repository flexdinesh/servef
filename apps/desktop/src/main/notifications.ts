import path from "node:path"
import type { LiveChange, LiveHead } from "../../../web/src/api/events-contract.ts"
import { connectionOrigin } from "./connections.ts"
import type { WalkthroughAnnouncement } from "./daemon-contract.ts"
import type { TimerHandle, Timers } from "./timers.ts"

export type NoticeKind = "switch" | "commit" | "walkthrough" | "settled"

export interface Notice {
  // connection is the connection id the notice came from (local or ssh-<slug>).
  connection: string
  project: string
  kind: NoticeKind
  title: string
  body: string
  // target is the servef-app://<connection> URL a click opens.
  target: string
}

// NoticeSource names the connection a notice came from; label is "" for
// local and the ssh destination otherwise.
export interface NoticeSource {
  connection: string
  label: string
}

export const SETTLE_QUIET_MS = 10_000
export const NOTICE_INTERVAL_MS = 30_000

// place is the project directory's base name, plus " on <label>" for remote
// connections.
function place(source: NoticeSource, project: string): string {
  const name = path.basename(project) || project
  return source.label === "" ? name : `${name} on ${source.label}`
}

function target(source: NoticeSource, route: "/" | "/view", params: Record<string, string>): string {
  return `${connectionOrigin(source.connection)}${route}?${new URLSearchParams(params).toString()}`
}

// headNotice ignores the baseline (previous null) and unchanged heads.
export function headNotice(source: NoticeSource, project: string, head: LiveHead): Notice | null {
  const previous = head.previous
  if (previous === null) return null
  const branch = head.branch === "" ? "detached HEAD" : head.branch
  const connection = source.connection
  if (head.branch !== previous.branch) {
    return { connection, project, kind: "switch", title: `Switched to ${branch}`, body: place(source, project), target: target(source, "/", { project }) }
  }
  if (head.commit !== "" && head.commit !== previous.commit) {
    return { connection, project, kind: "commit", title: `New commit on ${branch}`, body: head.subject, target: target(source, "/view", { project, commit: head.commit }) }
  }
  return null
}

export function walkthroughNotice(source: NoticeSource, announcement: WalkthroughAnnouncement): Notice {
  const { project, id, title, created } = announcement
  return {
    connection: source.connection,
    project,
    kind: "walkthrough",
    title: `${created ? "Walkthrough ready" : "Walkthrough updated"}: ${title}`,
    body: place(source, project),
    target: target(source, "/view", { project, wt: id }),
  }
}

export function settledNotice(source: NoticeSource, project: string, files: number, overflow: boolean): Notice {
  const count = overflow ? `${files}+ files` : files === 1 ? "1 file" : `${files} files`
  return {
    connection: source.connection,
    project,
    kind: "settled",
    title: `Edits settled in ${place(source, project)}`,
    body: count,
    target: target(source, "/view", { project, review: "working" }),
  }
}

export interface SettleTracker {
  change(connection: string, project: string, change: LiveChange): void
  forget(connection: string, project: string): void
  dispose(): void
}

interface Burst {
  connection: string
  project: string
  paths: Set<string>
  overflow: boolean
  timer: TimerHandle
}

function key(...parts: string[]): string {
  return JSON.stringify(parts)
}

// createSettleTracker emits once per burst of a (connection, project): after
// ≥1 change with paths, quietMs without changes. Changes without paths extend
// an armed burst but never start one.
export function createSettleTracker(quietMs: number, emit: (connection: string, project: string, files: number, overflow: boolean) => void, timers: Timers): SettleTracker {
  const bursts = new Map<string, Burst>()

  const arm = (id: string) => timers.setTimeout(() => {
    const burst = bursts.get(id)
    if (burst === undefined) return
    bursts.delete(id)
    emit(burst.connection, burst.project, burst.paths.size, burst.overflow)
  }, quietMs)

  return {
    change(connection, project, change) {
      const id = key(connection, project)
      const burst = bursts.get(id)
      const edits = change.paths.length > 0 || change.overflow
      if (burst === undefined) {
        if (edits) bursts.set(id, { connection, project, paths: new Set(change.paths), overflow: change.overflow, timer: arm(id) })
        return
      }
      for (const changed of change.paths) burst.paths.add(changed)
      burst.overflow ||= change.overflow
      timers.clearTimeout(burst.timer)
      burst.timer = arm(id)
    },
    forget(connection, project) {
      const id = key(connection, project)
      const burst = bursts.get(id)
      if (burst === undefined) return
      timers.clearTimeout(burst.timer)
      bursts.delete(id)
    },
    dispose() {
      for (const burst of bursts.values()) timers.clearTimeout(burst.timer)
      bursts.clear()
    },
  }
}

// FocusedWindow is the focused window's connection id and project.
export interface FocusedWindow {
  connection: string
  project: string | null
}

export interface NoticeGate {
  // allow suppresses notices for the focused window's connection and project
  // and repeats of a (connection, project, kind) within intervalMs; it
  // records allowed notices.
  allow(notice: Notice, focused: FocusedWindow | null): boolean
}

export function createNoticeGate(intervalMs: number, timers: Timers): NoticeGate {
  const last = new Map<string, number>()
  return {
    allow(notice, focused) {
      if (focused !== null && focused.connection === notice.connection && focused.project === notice.project) return false
      const id = key(notice.connection, notice.project, notice.kind)
      const now = timers.now()
      const at = last.get(id)
      if (at !== undefined && now - at < intervalMs) return false
      last.set(id, now)
      return true
    },
  }
}
