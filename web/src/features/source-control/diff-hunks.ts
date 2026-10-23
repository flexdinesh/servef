import type { FileDiffMetadata, Hunk, SelectedLineRange, SelectionSide } from "@pierre/diffs"

import type { DiffStyle } from "./diff-options.ts"

// A hunk's first line on each side; headers name the line before an empty side.
function hunkStarts(hunk: Hunk): { old: number; new: number } {
  return {
    old: hunk.deletionStart + (hunk.deletionCount === 0 ? 1 : 0),
    new: hunk.additionStart + (hunk.additionCount === 0 ? 1 : 0),
  }
}

// hunkSelection selects a hunk's rows as shown, first to last, context
// included. Unchanged rows and additions address the new side; an end on
// deletions only addresses the old side, so the range can cross sides.
export function hunkSelection(fileDiff: FileDiffMetadata, hunkIndex: number, style: DiffStyle): SelectedLineRange | null {
  const hunk = fileDiff.hunks[hunkIndex]
  const first = hunk?.hunkContent[0]
  const last = hunk?.hunkContent.at(-1)
  if (!hunk || !first || !last) return null
  const starts = hunkStarts(hunk)
  const oldEnd = starts.old + hunk.deletionCount - 1
  const newEnd = starts.new + hunk.additionCount - 1
  // Unified lists a change's deletions first; Split pairs them with additions.
  const startsOld = first.type === "change" && (style === "unified" ? first.deletions > 0 : first.additions === 0)
  const endsOld = last.type === "change" && (style === "unified" ? last.additions === 0 : last.deletions > last.additions)
  const side: SelectionSide = startsOld ? "deletions" : "additions"
  const endSide: SelectionSide = endsOld ? "deletions" : "additions"
  return {
    start: startsOld ? starts.old : starts.new, side,
    end: endsOld ? oldEnd : newEnd, endSide,
  }
}

// A HunkCursor places a position among hunks: inside one, or before one
// (index equals the hunk count past the last).
export type HunkCursor = { kind: "at" | "before"; index: number }

// hunkCursor places a line of either side among the hunks.
export function hunkCursor(fileDiff: FileDiffMetadata, side: SelectionSide, line: number): HunkCursor {
  for (const [index, hunk] of fileDiff.hunks.entries()) {
    const starts = hunkStarts(hunk)
    const start = side === "deletions" ? starts.old : starts.new
    const count = side === "deletions" ? hunk.deletionCount : hunk.additionCount
    if (line < start) return { kind: "before", index }
    if (line < start + count) return { kind: "at", index }
  }
  return { kind: "before", index: fileDiff.hunks.length }
}

// adjacentHunk steps to the next or previous hunk, wrapping within the file.
export function adjacentHunk(count: number, cursor: HunkCursor, direction: 1 | -1): number | null {
  if (count <= 0) return null
  const next = direction === 1
    ? cursor.kind === "at" ? cursor.index + 1 : cursor.index
    : cursor.index - 1
  return ((next % count) + count) % count
}

// A DiffGap is the run of unchanged lines collapsed before hunkIndex; the
// gap after the last hunk has index hunks.length, as Pierre expands it.
export interface DiffGap {
  hunkIndex: number
  lines: number
  // newLine is the addressed line's number on the new side.
  newLine: number
}

// gapForLine finds the unchanged gap between hunks that holds a line.
export function gapForLine(fileDiff: FileDiffMetadata, side: SelectionSide, line: number): DiffGap | null {
  let oldLine = 1
  let newLine = 1
  const inGap = (hunkIndex: number, lines: number): DiffGap | null => {
    const start = side === "deletions" ? oldLine : newLine
    if (lines <= 0 || line < start || line >= start + lines) return null
    return { hunkIndex, lines, newLine: newLine + line - start }
  }
  for (const [index, hunk] of fileDiff.hunks.entries()) {
    const starts = hunkStarts(hunk)
    const gap = inGap(index, starts.new - newLine)
    if (gap) return gap
    oldLine = starts.old + hunk.deletionCount
    newLine = starts.new + hunk.additionCount
  }
  return inGap(fileDiff.hunks.length, fileDiff.additionLines.length - newLine + 1)
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
