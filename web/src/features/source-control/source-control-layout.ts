import { isCommitRange, type CommitRange } from "../../api/commits-contract.ts"
import { isRecord } from "../../api/http.ts"

// Source Control's layout, commit range, and chosen base branches,
// remembered in this browser under servef-source-control (decisions 0027,
// 0035, 0040). Pure except for readLayout/writeLayout, which never throw.

export const LAYOUT_STORAGE_KEY = "servef-source-control"

// Panes stack in this order, always (decision 0040).
export const paneIds = ["working", "commits", "branches"] as const
export type PaneId = (typeof paneIds)[number]

// A pane's weight is its share of the height the open panes split; a sash
// never shrinks one below minimumWeight of the pair it splits.
export const minimumWeight = 0.15

// The review's Files rail width bounds, in rem so it follows the Font Size
// setting.
export const minimumRailWidth = 12
export const maximumRailWidth = 32
export const defaultRailWidth = 16

const maxBases = 100

export type ChangesView = "list" | "tree"

export interface SourceControlLayout {
  range: CommitRange
  // weights split the sidebar's height between open panes.
  weights: Readonly<Record<PaneId, number>>
  // collapsed holds the panes the user opened or closed; others follow
  // whether they have content.
  collapsed: Readonly<Partial<Record<PaneId, boolean>>>
  changesView: ChangesView
  railWidth: number
  railHidden: boolean
  // bases maps a project to its chosen base branch; absent uses the default.
  bases: Readonly<Record<string, string>>
}

export const defaultLayout: SourceControlLayout = {
  range: "branch",
  weights: { working: 0.35, commits: 0.45, branches: 0.2 },
  collapsed: {},
  changesView: "list",
  railWidth: defaultRailWidth,
  railHidden: false,
  bases: {},
}

export function clampRailWidth(width: number): number {
  return Math.min(maximumRailWidth, Math.max(minimumRailWidth, width))
}

function parseBases(value: unknown): Record<string, string> {
  if (!isRecord(value)) return {}
  const bases: Record<string, string> = {}
  for (const [project, base] of Object.entries(value)) {
    if (project !== "__proto__" && typeof base === "string" && base) bases[project] = base
  }
  return bases
}

function parseWeights(value: unknown): Record<PaneId, number> {
  const weights = { ...defaultLayout.weights }
  if (!isRecord(value)) return weights
  for (const id of paneIds) {
    const weight = value[id]
    if (typeof weight === "number" && Number.isFinite(weight) && weight > 0) weights[id] = weight
  }
  return weights
}

function parseCollapsed(value: unknown): Partial<Record<PaneId, boolean>> {
  const collapsed: Partial<Record<PaneId, boolean>> = {}
  if (!isRecord(value)) return collapsed
  for (const id of paneIds) {
    const entry = value[id]
    if (typeof entry === "boolean") collapsed[id] = entry
  }
  return collapsed
}

// parseLayout keeps every well-formed field of untrusted stored data. The
// list and Files split stored before decision 0040 is dropped.
export function parseLayout(value: unknown): SourceControlLayout {
  if (!isRecord(value)) return defaultLayout
  const railWidth = value.railWidth
  return {
    range: isCommitRange(value.range) ? value.range : defaultLayout.range,
    weights: parseWeights(value.weights),
    collapsed: parseCollapsed(value.collapsed),
    changesView: value.changesView === "tree" ? "tree" : "list",
    railWidth: typeof railWidth === "number" && Number.isFinite(railWidth) ? clampRailWidth(railWidth) : defaultRailWidth,
    railHidden: value.railHidden === true,
    bases: parseBases(value.bases),
  }
}

// paneOpen is whether a pane shows its body: the user's choice once made,
// else whether it has content. Unknown content (null) counts as content, so
// panes do not flash closed while they read.
export function paneOpen(collapsed: boolean | undefined, empty: boolean | null): boolean {
  return collapsed === undefined ? empty !== true : !collapsed
}

// resizePair moves the sash between two open panes by delta, a share of the
// height the open panes split, keeping the pair's total and each side at
// least minimumWeight of it.
export function resizePair(weights: Readonly<Record<PaneId, number>>, open: readonly PaneId[], above: PaneId, below: PaneId, delta: number): Record<PaneId, number> {
  const total = open.reduce((sum, id) => sum + weights[id], 0)
  const pair = weights[above] + weights[below]
  const floor = pair * minimumWeight
  const next = Math.min(pair - floor, Math.max(floor, weights[above] + delta * total))
  return { ...weights, [above]: next, [below]: pair - next }
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
