import { isCommitRange, type CommitRange } from "../../api/commits-contract.ts"
import { isRecord } from "../../api/http.ts"

// Source Control's layout, commit range, and chosen base branches,
// remembered in this browser under servef-source-control (decisions 0027,
// 0035). Pure except for readLayout/writeLayout, which never throw.

export const LAYOUT_STORAGE_KEY = "servef-source-control"

// listShare bounds keep the list and the Files pane usable while dragging.
export const minimumShare = 0.15
export const maximumShare = 0.85

const maxBases = 100

export interface SourceControlLayout {
  range: CommitRange
  // listShare is the list's share of the height it splits with the Files
  // pane on desktop.
  listShare: number
  // bases maps a project to its chosen base branch; absent uses the default.
  bases: Readonly<Record<string, string>>
}

export const defaultLayout: SourceControlLayout = {
  range: "branch",
  listShare: 0.5,
  bases: {},
}

export function clampShare(share: number): number {
  return Math.min(maximumShare, Math.max(minimumShare, share))
}

function parseBases(value: unknown): Record<string, string> {
  if (!isRecord(value)) return {}
  const bases: Record<string, string> = {}
  for (const [project, base] of Object.entries(value)) {
    if (project !== "__proto__" && typeof base === "string" && base) bases[project] = base
  }
  return bases
}

// parseLayout keeps every well-formed field of untrusted stored data. The
// split stored before decision 0035 (changesShare) carries over.
export function parseLayout(value: unknown): SourceControlLayout {
  if (!isRecord(value)) return defaultLayout
  const share = typeof value.listShare === "number" ? value.listShare : value.changesShare
  return {
    range: isCommitRange(value.range) ? value.range : defaultLayout.range,
    listShare: typeof share === "number" && Number.isFinite(share) ? clampShare(share) : defaultLayout.listShare,
    bases: parseBases(value.bases),
  }
}

// withBase remembers project's base; "" forgets it. Re-inserting keeps the
// most recent last, so the oldest drop first.
export function withBase(layout: SourceControlLayout, project: string, base: string): SourceControlLayout {
  const { [project]: _previous, ...rest } = layout.bases
  const kept = Object.entries(rest).slice(-(maxBases - 1))
  return { ...layout, bases: Object.fromEntries(base ? [...kept, [project, base]] : kept) }
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
