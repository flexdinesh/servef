import type { LiveChange, LiveStatus } from "../../api/events-contract.ts"

// LiveTargets are what one change rereads (decision 0050).
export interface LiveTargets {
  // page rereads the routed page: the tree and the open document.
  page: boolean
  // changes rereads the working-tree change list.
  changes: boolean
  // history rereads commits and the branch stack.
  history: boolean
  // search marks the search corpus stale.
  search: boolean
}

// liveTargets decides what a change rereads. document is the open
// working-tree document's path, "" for none, a snapshot, a review, or a
// walkthrough. Overflow rereads everything.
export function liveTargets(change: LiveChange, document: string): LiveTargets {
  const all = change.overflow
  const files = change.paths.length > 0
  return {
    page: all || change.tree || (document !== "" && change.paths.includes(document)),
    changes: all || change.git || files,
    history: all || change.git,
    search: all || change.tree || files,
  }
}

// everything is what returning to a tab rereads: changes it may have missed.
export const everything: LiveChange = { paths: [], tree: false, git: false, overflow: true }

export interface LiveIndicator {
  label: string
  // description is the indicator's tooltip and accessible name.
  description: string
  mode: LiveStatus["mode"]
}

// liveIndicator names a status for the status bar.
export function liveIndicator(status: LiveStatus): LiveIndicator {
  if (status.mode === "watching") {
    return { label: "Live", description: "Live updates: watching the project for changes.", mode: status.mode }
  }
  const reason = status.reason ? ` ${status.reason}` : ""
  if (status.mode === "polling") {
    return { label: "Polling", description: `Live updates: checking the project for changes every few seconds.${reason}`, mode: status.mode }
  }
  return { label: "Off", description: `Live updates are off; the project rereads when you return to it.${reason}`, mode: status.mode }
}
