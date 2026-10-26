import { isRecord } from "../../api/http.ts"

// Per-browser walkthrough memory (decision 0047), under servef-walkthroughs:
// each project's newest walkthrough time the user has seen, for the activity
// badge, and the last step opened in each walkthrough, to resume it. Pure
// functions return new values; at most 50 projects and 100 walkthroughs each
// are kept, newest first.

const storageKey = "servef-walkthroughs"
const maxProjects = 50
const maxWalkthroughs = 100

export interface WalkthroughMemory {
  // seen is the newest updatedAt seen per project.
  seen: Record<string, string>
  // steps is the last step index per project and walkthrough.
  steps: Record<string, Record<string, number>>
}

export const emptyWalkthroughMemory: WalkthroughMemory = { seen: {}, steps: {} }

function entries(value: unknown): [string, unknown][] {
  return isRecord(value) ? Object.entries(value) : []
}

export function parseWalkthroughMemory(value: unknown): WalkthroughMemory {
  if (!isRecord(value)) return emptyWalkthroughMemory
  const seen: Record<string, string> = {}
  for (const [project, time] of entries(value.seen).slice(0, maxProjects)) {
    if (typeof time === "string") seen[project] = time
  }
  const steps: Record<string, Record<string, number>> = {}
  for (const [project, ids] of entries(value.steps).slice(0, maxProjects)) {
    const kept: Record<string, number> = {}
    for (const [id, step] of entries(ids).slice(0, maxWalkthroughs)) {
      if (typeof step === "number" && Number.isInteger(step) && step >= 0) kept[id] = step
    }
    steps[project] = kept
  }
  return { seen, steps }
}

// withFirst puts key first in record, dropping entries past max.
function withFirst<T>(record: Record<string, T>, key: string, value: T, max: number): Record<string, T> {
  const { [key]: _previous, ...rest } = record
  return Object.fromEntries([[key, value], ...Object.entries(rest).slice(0, max - 1)])
}

export function rememberSeen(memory: WalkthroughMemory, project: string, updatedAt: string): WalkthroughMemory {
  if (memory.seen[project] === updatedAt) return memory
  return { ...memory, seen: withFirst(memory.seen, project, updatedAt, maxProjects) }
}

export function rememberStep(memory: WalkthroughMemory, project: string, id: string, step: number): WalkthroughMemory {
  const ids = memory.steps[project] ?? {}
  if (ids[id] === step) return memory
  return { ...memory, steps: withFirst(memory.steps, project, withFirst(ids, id, step, maxWalkthroughs), maxProjects) }
}

// unseenCount counts walkthroughs updated after the newest one seen; before
// any is seen, all are.
export function unseenCount(updated: readonly string[], seen: string | undefined): number {
  if (seen === undefined) return updated.length
  const since = Date.parse(seen)
  return updated.filter((time) => Date.parse(time) > since).length
}

export function readWalkthroughMemory(): WalkthroughMemory {
  try {
    return parseWalkthroughMemory(JSON.parse(window.localStorage.getItem(storageKey) ?? "null"))
  } catch {
    return emptyWalkthroughMemory
  }
}

export function writeWalkthroughMemory(memory: WalkthroughMemory) {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(memory))
  } catch {
    // Walkthroughs still work when storage is unavailable.
  }
}
