import type { SelectedLineRange, SelectionSide } from "@pierre/diffs"

import type { LineDiff } from "@servef/contracts"
import type { DiffStyle } from "./diff-options.ts"

// Hunks, blocks, and gaps come from the server's line diff (decision 0083),
// numbered from 1 on each side; these functions place selections and steps
// among them as the diff shows its rows.

// hunkSelection selects a hunk's rows as shown, first to last, context
// included. Unchanged rows and additions address the new side; an end on
// deletions only addresses the old side, so the range can cross sides.
export function hunkSelection(lineDiff: LineDiff, hunkIndex: number, style: DiffStyle): SelectedLineRange | null {
  const hunk = lineDiff.hunks[hunkIndex]
  const first = hunk?.blocks[0]
  const last = hunk?.blocks.at(-1)
  if (!hunk || !first || !last) return null
  const oldEnd = hunk.oldStart + hunk.oldCount - 1
  const newEnd = hunk.newStart + hunk.newCount - 1
  // Unified lists a change's deletions first; Split pairs them with additions.
  const startsOld = first.kind === "change" && (style === "unified" ? first.oldCount > 0 : first.newCount === 0)
  const endsOld = last.kind === "change" && (style === "unified" ? last.newCount === 0 : last.oldCount > last.newCount)
  const side: SelectionSide = startsOld ? "deletions" : "additions"
  const endSide: SelectionSide = endsOld ? "deletions" : "additions"
  return {
    start: startsOld ? hunk.oldStart : hunk.newStart, side,
    end: endsOld ? oldEnd : newEnd, endSide,
  }
}

// A HunkCursor places a position among hunks: inside one, or before one
// (index equals the hunk count past the last).
export type HunkCursor = { kind: "at" | "before"; index: number }

// hunkCursor places a line of either side among the hunks.
export function hunkCursor(lineDiff: LineDiff, side: SelectionSide, line: number): HunkCursor {
  for (const [index, hunk] of lineDiff.hunks.entries()) {
    const start = side === "deletions" ? hunk.oldStart : hunk.newStart
    const count = side === "deletions" ? hunk.oldCount : hunk.newCount
    if (line < start) return { kind: "before", index }
    if (line < start + count) return { kind: "at", index }
  }
  return { kind: "before", index: lineDiff.hunks.length }
}

// adjacentHunk steps to the next or previous hunk, wrapping within the file.
export function adjacentHunk(count: number, cursor: HunkCursor, direction: 1 | -1): number | null {
  if (count <= 0) return null
  const next = direction === 1
    ? cursor.kind === "at" ? cursor.index + 1 : cursor.index
    : cursor.index - 1
  return ((next % count) + count) % count
}

// A CollapsedLine is a line inside an unchanged gap: the gap's hunk index
// (the hunk count past the last hunk, as Pierre expands it), its length, and
// the line's number on the new side.
export interface CollapsedLine {
  hunkIndex: number
  lines: number
  newLine: number
}

// gapForLine finds the unchanged gap that holds a line of either side.
export function gapForLine(lineDiff: LineDiff, side: SelectionSide, line: number): CollapsedLine | null {
  for (const gap of lineDiff.gaps) {
    const start = side === "deletions" ? gap.oldStart : gap.newStart
    if (line >= start && line < start + gap.lines) return { hunkIndex: gap.hunk, lines: gap.lines, newLine: gap.newStart + line - start }
  }
  return null
}

// hunkInFile steps to the next or previous hunk without wrapping, null past
// the file's ends; a review then moves on to another file.
export function hunkInFile(count: number, cursor: HunkCursor, direction: 1 | -1): number | null {
  const next = direction === 1
    ? cursor.kind === "at" ? cursor.index + 1 : cursor.index
    : cursor.index - 1
  return next >= 0 && next < count ? next : null
}

// fileOrder lists the files a review step visits after from, in direction,
// wrapping past the ends and ending on from itself (decision 0035).
export function fileOrder(count: number, from: number, direction: 1 | -1): number[] {
  if (count <= 0) return []
  const start = from >= 0 && from < count ? from : direction === 1 ? count - 1 : 0
  return Array.from({ length: count }, (_, step) => (((start + direction * (step + 1)) % count) + count) % count)
}
