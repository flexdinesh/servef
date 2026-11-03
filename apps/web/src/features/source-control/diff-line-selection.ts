import type { FileDiffMetadata, SelectedLineRange, SelectionSide } from "@pierre/diffs"

import type { DiffStyle } from "./diff-options.ts"
import type { LineRange } from "../../lib/line-selection.ts"

// DiffLineTarget is what a diff selection addresses: lines of the old file
// (HEAD or the commit's parent) or of the new file.
export interface DiffLineTarget {
  side: "old" | "new"
  range: LineRange
}

const diffAnchorPattern = /^#?([LR])([1-9]\d*)(?:-([LR])?([1-9]\d*))?$/

function anchorSide(letter: string | undefined): SelectionSide {
  return letter === "L" ? "deletions" : "additions"
}

function sideLetter(side: SelectionSide | undefined): string {
  return side === "deletions" ? "L" : "R"
}

// parseDiffLineAnchor reads GitHub diff fragments: L is the old side, R the
// new one. #R12, #R12-R20 (#R12-20 too), #L5, and #L5-R8 across sides.
export function parseDiffLineAnchor(hash: string): SelectedLineRange | null {
  const match = diffAnchorPattern.exec(hash)
  if (!match) return null
  const side = anchorSide(match[1])
  const endSide = match[3] === undefined ? side : anchorSide(match[3])
  const first = Number(match[2])
  const second = match[4] === undefined ? first : Number(match[4])
  if (!Number.isSafeInteger(first) || !Number.isSafeInteger(second)) return null
  if (side !== endSide) return { start: first, side, end: second, endSide }
  return { start: Math.min(first, second), side, end: Math.max(first, second), endSide }
}

// diffLineAnchor formats a selection as a fragment without the leading #.
// One side reads top down; a range across sides keeps its drag order.
export function diffLineAnchor(range: SelectedLineRange): string {
  const side = range.side ?? "additions"
  const endSide = range.endSide ?? side
  if (side !== endSide) return `${sideLetter(side)}${range.start}-${sideLetter(endSide)}${range.end}`
  const start = Math.min(range.start, range.end)
  const end = Math.max(range.start, range.end)
  return start === end ? `${sideLetter(side)}${start}` : `${sideLetter(side)}${start}-${sideLetter(side)}${end}`
}

// A Block is a run of rows: unchanged lines (both sides equal) or one change
// of deletions then additions.
interface Block {
  change: boolean
  oldStart: number
  newStart: number
  deletions: number
  additions: number
}

function blocks(fileDiff: FileDiffMetadata): Block[] {
  const result: Block[] = []
  let oldLine = 1
  let newLine = 1
  const context = (lines: number) => {
    if (lines <= 0) return
    result.push({ change: false, oldStart: oldLine, newStart: newLine, deletions: lines, additions: lines })
    oldLine += lines
    newLine += lines
  }
  for (const hunk of fileDiff.hunks) {
    // Hunk headers name the line before an empty side.
    context(hunk.additionStart + (hunk.additionCount === 0 ? 1 : 0) - newLine)
    for (const content of hunk.hunkContent) {
      if (content.type === "context") {
        context(content.lines)
        continue
      }
      result.push({ change: true, oldStart: oldLine, newStart: newLine, deletions: content.deletions, additions: content.additions })
      oldLine += content.deletions
      newLine += content.additions
    }
  }
  context(fileDiff.additionLines.length - newLine + 1)
  return result
}

interface RowPoint {
  side: SelectionSide
  line: number
  block: Block
  blockIndex: number
  row: number
}

// rowPoint finds a side's line among the rendered rows: Unified lists a
// change's deletions before its additions; Split pairs them.
function rowPoint(rows: readonly Block[], side: SelectionSide, line: number, style: DiffStyle): RowPoint | null {
  for (const [blockIndex, block] of rows.entries()) {
    const start = side === "deletions" ? block.oldStart : block.newStart
    const count = side === "deletions" ? block.deletions : block.additions
    if (line < start || line >= start + count) continue
    const offset = line - start
    const row = block.change && side === "additions" && style === "unified" ? block.deletions + offset : offset
    return { side, line, block, blockIndex, row }
  }
  return null
}

function before(a: RowPoint, b: RowPoint): boolean {
  return a.blockIndex < b.blockIndex || (a.blockIndex === b.blockIndex && a.row <= b.row)
}

// newLinesBefore counts a block's new-side lines in rows before row.
function newLinesBefore(block: Block, row: number, style: DiffStyle): number {
  if (!block.change) return row
  return style === "unified" ? Math.max(0, row - block.deletions) : Math.min(row, block.additions)
}

// diffLineTarget resolves a selection to the lines it addresses. One side
// keeps its own numbers; a range across sides covers the new lines between
// its ends, in the rows as shown.
export function diffLineTarget(fileDiff: FileDiffMetadata, selection: SelectedLineRange, style: DiffStyle): DiffLineTarget | null {
  const rows = blocks(fileDiff)
  const startSide = selection.side ?? "additions"
  const first = rowPoint(rows, startSide, selection.start, style)
  const second = rowPoint(rows, selection.endSide ?? startSide, selection.end, style)
  if (!first || !second) return null
  if (first.side === second.side) {
    const range = { start: Math.min(first.line, second.line), end: Math.max(first.line, second.line) }
    return { side: first.side === "deletions" ? "old" : "new", range }
  }
  const [top, bottom] = before(first, second) ? [first, second] : [second, first]
  // An old-side end moves inward to the nearest new line.
  const start = top.side === "additions" ? top.line : top.block.newStart + newLinesBefore(top.block, top.row, style)
  const end = bottom.side === "additions" ? bottom.line : bottom.block.newStart + newLinesBefore(bottom.block, bottom.row + 1, style) - 1
  return { side: "new", range: { start, end } }
}
