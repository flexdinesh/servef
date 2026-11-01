import path from "node:path"
import type { LiveChange, LiveHead } from "../../../web/src/api/events-contract.ts"
import type { WalkthroughAnnouncement } from "./daemon-contract.ts"
import type { TimerHandle, Timers } from "./timers.ts"

export type NoticeKind = "switch" | "commit" | "walkthrough" | "settled"

export interface Notice {
  project: string
  kind: NoticeKind
  title: string
  body: string
  // target is the servef-app://local URL a click opens.
  target: string
}

export const SETTLE_QUIET_MS = 10_000
export const NOTICE_INTERVAL_MS = 30_000

// projectName is the project directory's base name.
function projectName(project: string): string {
  return path.basename(project) || project
}

function target(route: "/" | "/view", params: Record<string, string>): string {
  return `servef-app://local${route}?${new URLSearchParams(params).toString()}`
}

// headNotice ignores the baseline (previous null) and unchanged heads.
export function headNotice(project: string, head: LiveHead): Notice | null {
  const previous = head.previous
  if (previous === null) return null
  const branch = head.branch === "" ? "detached HEAD" : head.branch
  if (head.branch !== previous.branch) {
    return { project, kind: "switch", title: `Switched to ${branch}`, body: projectName(project), target: target("/", { project }) }
  }
  if (head.commit !== "" && head.commit !== previous.commit) {
    return { project, kind: "commit", title: `New commit on ${branch}`, body: head.subject, target: target("/view", { project, commit: head.commit }) }
  }
  return null
}

export function walkthroughNotice(announcement: WalkthroughAnnouncement): Notice {
  const { project, id, title, created } = announcement
  return {
    project,
    kind: "walkthrough",
    title: `${created ? "Walkthrough ready" : "Walkthrough updated"}: ${title}`,
    body: projectName(project),
    target: target("/view", { project, wt: id }),
  }
}

export function settledNotice(project: string, files: number, overflow: boolean): Notice {
  const count = overflow ? `${files}+ files` : files === 1 ? "1 file" : `${files} files`
  return {
    project,
    kind: "settled",
    title: `Edits settled in ${projectName(project)}`,
    body: count,
    target: target("/view", { project, review: "working" }),
  }
}

export interface SettleTracker {
  change(project: string, change: LiveChange): void
  forget(project: string): void
  dispose(): void
}

interface Burst {
  paths: Set<string>
  overflow: boolean
  timer: TimerHandle
}

// createSettleTracker emits once per burst: after ≥1 change with paths, quietMs without changes.
// Changes without paths extend an armed burst but never start one.
export function createSettleTracker(quietMs: number, emit: (project: string, files: number, overflow: boolean) => void, timers: Timers): SettleTracker {
  const bursts = new Map<string, Burst>()

  const arm = (project: string) => timers.setTimeout(() => {
    const burst = bursts.get(project)
    if (burst === undefined) return
    bursts.delete(project)
    emit(project, burst.paths.size, burst.overflow)
  }, quietMs)

  return {
    change(project, change) {
      const burst = bursts.get(project)
      const edits = change.paths.length > 0 || change.overflow
      if (burst === undefined) {
        if (edits) bursts.set(project, { paths: new Set(change.paths), overflow: change.overflow, timer: arm(project) })
        return
      }
      for (const changed of change.paths) burst.paths.add(changed)
      burst.overflow ||= change.overflow
      timers.clearTimeout(burst.timer)
      burst.timer = arm(project)
    },
    forget(project) {
      const burst = bursts.get(project)
      if (burst === undefined) return
      timers.clearTimeout(burst.timer)
      bursts.delete(project)
    },
    dispose() {
      for (const burst of bursts.values()) timers.clearTimeout(burst.timer)
      bursts.clear()
    },
  }
}

export interface NoticeGate {
  // allow suppresses notices for a focused project and repeats of a project
  // and kind within intervalMs; it records allowed notices.
  allow(notice: Notice, focusedProject: string | null): boolean
}

export function createNoticeGate(intervalMs: number, timers: Timers): NoticeGate {
  const last = new Map<string, number>()
  return {
    allow(notice, focusedProject) {
      if (notice.project === focusedProject) return false
      const key = `${notice.kind}\u0000${notice.project}`
      const now = timers.now()
      const at = last.get(key)
      if (at !== undefined && now - at < intervalMs) return false
      last.set(key, now)
      return true
    },
  }
}
