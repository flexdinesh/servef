import { isRecord } from "./http.ts"

// Transport contract for /api/events (decision 0050): a server-sent event
// stream of one project's live updates. "status" says how the project is
// followed, first and whenever that changes; "change" is one debounced batch.
// A checkout's stream also sends "head" after the first status and after a
// git change that moves HEAD (decision 0066); the browser ignores it.

export type LiveMode = "watching" | "polling" | "off"

export interface LiveStatus {
  mode: LiveMode
  // reason explains polling and off; "" while watching.
  reason: string
}

export interface LiveChange {
  // paths are the changed project-relative slash paths, sorted.
  paths: string[]
  // tree reports that an entry was created, removed, or renamed.
  tree: boolean
  // git reports that HEAD, the index, or a ref changed.
  git: boolean
  // overflow reports that more changed than paths lists.
  overflow: boolean
}

export interface LiveHeadRef {
  // branch is "" when HEAD is detached.
  branch: string
  // commit is HEAD's full hash; "" on an unborn branch.
  commit: string
}

export interface LiveHead extends LiveHeadRef {
  // subject is the first line of the commit's message.
  subject: string
  // previous is the stream's last head; null for the first.
  previous: LiveHeadRef | null
}

export function isLiveStatus(value: unknown): value is LiveStatus {
  return isRecord(value)
    && (value.mode === "watching" || value.mode === "polling" || value.mode === "off")
    && typeof value.reason === "string"
}

export function isLiveChange(value: unknown): value is LiveChange {
  return isRecord(value)
    && Array.isArray(value.paths)
    && value.paths.every((path) => typeof path === "string" && path !== "")
    && typeof value.tree === "boolean"
    && typeof value.git === "boolean"
    && typeof value.overflow === "boolean"
}

function isLiveHeadRef(value: unknown): value is LiveHeadRef {
  return isRecord(value)
    && typeof value.branch === "string"
    && typeof value.commit === "string"
}

export function isLiveHead(value: unknown): value is LiveHead {
  return isRecord(value)
    && isLiveHeadRef(value)
    && typeof value.subject === "string"
    && (value.previous === null || isLiveHeadRef(value.previous))
}
