import { isCommitRange, type CommitRange } from "../../commits-contract.ts"
import { isRecord } from "../../http.ts"

// Source Control's section layout and commit range, remembered in this
// browser under servef-source-control (decision 0027). Pure except for
// readLayout/writeLayout, which never throw.

export const LAYOUT_STORAGE_KEY = "servef-source-control"

// changesShare bounds keep both sections usable while dragging.
export const minimumShare = 0.15
export const maximumShare = 0.85

export interface SourceControlLayout {
  range: CommitRange
  changesCollapsed: boolean
  commitsCollapsed: boolean
  // changesShare is the Changes section's share of the height both open
  // sections split on desktop.
  changesShare: number
}

export const defaultLayout: SourceControlLayout = {
  range: "branch",
  changesCollapsed: false,
  commitsCollapsed: false,
  changesShare: 0.5,
}

export function clampShare(share: number): number {
  return Math.min(maximumShare, Math.max(minimumShare, share))
}

// parseLayout keeps every well-formed field of untrusted stored data.
export function parseLayout(value: unknown): SourceControlLayout {
  if (!isRecord(value)) return defaultLayout
  return {
    range: isCommitRange(value.range) ? value.range : defaultLayout.range,
    changesCollapsed: typeof value.changesCollapsed === "boolean" ? value.changesCollapsed : defaultLayout.changesCollapsed,
    commitsCollapsed: typeof value.commitsCollapsed === "boolean" ? value.commitsCollapsed : defaultLayout.commitsCollapsed,
    changesShare: typeof value.changesShare === "number" && Number.isFinite(value.changesShare)
      ? clampShare(value.changesShare)
      : defaultLayout.changesShare,
  }
}

export function readLayout(): SourceControlLayout {
  try {
    const raw = window.localStorage.getItem(LAYOUT_STORAGE_KEY)
    return raw ? parseLayout(JSON.parse(raw)) : defaultLayout
  } catch {
    return defaultLayout
  }
}

export function writeLayout(layout: SourceControlLayout) {
  try {
    window.localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify(layout))
  } catch {
    // Storage is a convenience; the layout still applies for this session.
  }
}
