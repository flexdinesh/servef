import type { StackLayer, StackResponse } from "../../api/stack-contract.ts"
import { branchCompare, type Compare } from "./compare.ts"

// Pure decisions behind Source Control's panes (decision 0040): when the
// branch range applies, what each commit and stack row diffs, and how the
// flat change list names files.

// onBaseBranch is whether HEAD is the base branch itself, or a local branch
// tracking origin's copy of it; commits then list all history.
export function onBaseBranch(branch: string, base: string): boolean {
  return branch !== "" && base !== "" && (base === branch || base === `origin/${branch}`)
}

// RowDiff is what a commit row's Diff action opens, and whether it stays
// shown (on the selection) rather than only on hover or focus.
export interface RowDiff {
  compare: Compare
  pinned: boolean
}

// rowDiff places Diff on a commit row: a range's newest commit opens the
// range, its other commits offer none, and any other commit opens itself,
// pinned when it is the selection. selected lists the selection's hashes.
export function rowDiff(hash: string, selection: Compare | null, selected: readonly string[]): RowDiff | null {
  if (selection?.kind === "range" && selected.includes(hash)) {
    return hash === selection.to ? { compare: selection, pinned: true } : null
  }
  return { compare: { kind: "commit", hash }, pinned: selection?.kind === "commit" && selection.hash === hash }
}

// RefChip names a local branch at a commit.
export interface RefChip {
  name: string
  current: boolean
}

// refChips maps each stack layer's commit to its branch names, current
// first.
export function refChips(stack: StackResponse | null): ReadonlyMap<string, readonly RefChip[]> {
  const chips = new Map<string, RefChip[]>()
  for (const layer of stack?.layers ?? []) {
    const names = [layer.name, ...layer.aliases].filter(Boolean)
    if (names.length === 0) continue
    chips.set(layer.head, names.map((name) => ({ name, current: layer.current && name === layer.name })))
  }
  return chips
}

// StackRow is one Branches row with what its actions diff: the layer's own
// commits against the layer below (null for the lowest, where it equals the
// base diff), and everything since the base.
export interface StackRow {
  layer: StackLayer
  layerDiff: Compare | null
  baseDiff: Compare
}

// stackRows lists the stack top to bottom. base is the chosen base ("" for
// the default), kept in compares so they match the branch review. HEAD's
// layer compares as HEAD, so its review stays live with the checkout.
export function stackRows(stack: StackResponse | null, base: string): StackRow[] {
  if (!stack) return []
  return stack.layers.map((layer) => {
    const head = layer.current ? "" : layer.name
    const below = stack.layers.find((other) => other.name !== "" && other.name === layer.parent)
    return {
      layer,
      layerDiff: below ? branchCompare(below.name, head) : null,
      baseDiff: branchCompare(base, head),
    }
  })
}

// fileParts splits a path into the flat list's name and muted folder.
export function fileParts(path: string): { name: string; folder: string } {
  const slash = path.lastIndexOf("/")
  return slash < 0 ? { name: path, folder: "" } : { name: path.slice(slash + 1), folder: path.slice(0, slash) }
}
