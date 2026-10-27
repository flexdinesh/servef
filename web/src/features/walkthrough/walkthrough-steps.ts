import type { SelectedLineRange, SelectionSide } from "@pierre/diffs"

import type { WalkthroughStep } from "../../api/walkthrough-contract.ts"
import { isCommitHash, shortHash } from "../../lib/commit-hash.ts"
import { lineAnchor, type LineRange } from "../../lib/line-selection.ts"
import { branchCompare, compareName, compareSearch, reviewFragment, workingCompare, type Compare, type CompareContext, type CompareSearch } from "../source-control/compare.ts"

// Pure model of walkthrough steps (decision 0047): what each step opens, the
// route that opens it, how it is labelled, and whether its code has drifted
// since the step was written. Steps open ordinary document and review tabs;
// the route adds wt=<id>&step=<n> (1-based) so the narration follows.

export type StepTarget =
  | { kind: "note" }
  | { kind: "file"; path: string; at: string; lines: LineRange | null; expect: string }
  | { kind: "diff"; compare: Compare; path: string; side: SelectionSide; lines: LineRange | null; expect: string }

function lineRange(lines: readonly [number, number] | undefined): LineRange | null {
  return lines ? { start: lines[0], end: lines[1] } : null
}

// walkthroughCompare maps a published compare to Source Control's. Commit
// hashes are stored in full; anything else is not a compare.
export function walkthroughCompare(compare: NonNullable<WalkthroughStep["diff"]>["compare"]): Compare | null {
  switch (compare.kind) {
    case "working":
      return workingCompare
    case "branch":
      return branchCompare(compare.base ?? "", compare.head ?? "")
    case "commit":
      return isCommitHash(compare.commit) ? { kind: "commit", hash: compare.commit } : null
    case "range":
      return isCommitHash(compare.from) && isCommitHash(compare.to) ? { kind: "range", from: compare.from, to: compare.to } : null
  }
}

// stepTarget is what a step opens; a step whose target cannot open reads as
// a note.
export function stepTarget(step: WalkthroughStep): StepTarget {
  if (step.file) {
    const at = step.file.at && isCommitHash(step.file.at) ? step.file.at : ""
    return { kind: "file", path: step.file.path, at, lines: lineRange(step.file.lines), expect: step.file.expect ?? "" }
  }
  if (step.diff) {
    const compare = walkthroughCompare(step.diff.compare)
    if (!compare) return { kind: "note" }
    const side: SelectionSide = step.diff.side === "old" ? "deletions" : "additions"
    return { kind: "diff", compare, path: step.diff.path ?? "", side, lines: step.diff.path ? lineRange(step.diff.lines) : null, expect: step.diff.expect ?? "" }
  }
  return { kind: "note" }
}

export type StepSearch =
  | { project: string; wt: string; step?: number }
  | { project: string; path: string; at?: string; wt: string; step: number }
  | (CompareSearch & { project: string; wt: string; step: number })

export interface StepRoute {
  search: StepSearch
  hash: string
}

// coverRoute opens a walkthrough's own tab at its cover.
export function coverRoute(project: string, id: string): StepRoute {
  return { search: { project, wt: id }, hash: "" }
}

// diffSelection is a diff target's lines as a one-sided selection.
export function diffSelection(target: { side: SelectionSide; lines: LineRange | null }): SelectedLineRange | null {
  return target.lines ? { start: target.lines.start, end: target.lines.end, side: target.side, endSide: target.side } : null
}

// stepRoute opens step index (0-based): a file in Source at its lines, a
// review at its file and lines, or a note in the walkthrough's tab.
export function stepRoute(project: string, id: string, index: number, step: WalkthroughStep): StepRoute {
  const number = index + 1
  const target = stepTarget(step)
  switch (target.kind) {
    case "file":
      return {
        search: target.at ? { project, path: target.path, at: target.at, wt: id, step: number } : { project, path: target.path, wt: id, step: number },
        hash: target.lines ? lineAnchor(target.lines) : "",
      }
    case "diff":
      return {
        search: { project, ...compareSearch(target.compare), wt: id, step: number },
        hash: target.path ? reviewFragment(target.path, diffSelection(target)) : "",
      }
    case "note":
      return { search: { project, wt: id, step: number }, hash: "" }
  }
}

function linesLabel(lines: LineRange | null): string {
  if (!lines) return ""
  return lines.start === lines.end ? `:${lines.start}` : `:${lines.start}–${lines.end}`
}

// stepLabel names what a step opens: "a.go:12–20", "a.go:3 @ 3577ec6",
// "vs main · a.go:4–9", "Uncommitted", or "Note".
export function stepLabel(step: WalkthroughStep, context: CompareContext): string {
  const target = stepTarget(step)
  switch (target.kind) {
    case "file":
      return `${target.path}${linesLabel(target.lines)}${target.at ? ` @ ${shortHash(target.at)}` : ""}`
    case "diff": {
      const compare = compareName(target.compare, context).label
      return target.path ? `${compare} · ${target.path}${linesLabel(target.lines)}${target.side === "deletions" && target.lines ? " (old)" : ""}` : compare
    }
    case "note":
      return "Note"
  }
}

export type Drift = "unchanged" | "changed" | "unknown"

// lineDrift compares a step's expected prefix with its first line now: a
// line that no longer starts with it has changed. Without an expected
// prefix nothing is known.
export function lineDrift(expect: string, line: string | undefined): Drift {
  if (!expect) return "unknown"
  if (line === undefined) return "changed"
  return line.trim().startsWith(expect) ? "unchanged" : "changed"
}

// StepPosition is a step index, or null for the cover.
export type StepPosition = number | null

// stepAfter moves one step forward or back from position: the cover leads
// to the first step and back from it; the ends stay put (undefined).
export function stepAfter(position: StepPosition, count: number, direction: 1 | -1): StepPosition | undefined {
  if (count === 0) return undefined
  if (position === null) return direction === 1 ? 0 : undefined
  const next = position + direction
  if (next < 0) return null
  return next < count ? next : undefined
}

// parseStep reads a route's 1-based step as an index within count, or null
// (the cover) when absent or out of range.
export function parseStep(value: unknown, count: number): StepPosition {
  const number = typeof value === "number" ? value : typeof value === "string" ? Number(value) : Number.NaN
  return Number.isInteger(number) && number >= 1 && number <= count ? number - 1 : null
}

// routeStep validates a route's step parameter: a positive integer.
export function routeStep(value: unknown): number | undefined {
  const number = typeof value === "number" ? value : typeof value === "string" && /^\d+$/.test(value) ? Number(value) : Number.NaN
  return Number.isInteger(number) && number >= 1 ? number : undefined
}

// routeWalkthrough validates a route's walkthrough ID.
export function routeWalkthrough(value: unknown): string | undefined {
  return typeof value === "string" && /^[a-z0-9][a-z0-9-]{0,63}$/.test(value) ? value : undefined
}
