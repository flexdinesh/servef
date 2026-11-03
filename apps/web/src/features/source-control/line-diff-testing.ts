import { parseDiffFromFile } from "@pierre/diffs"

import type { DiffBlock, DiffGap, DiffHunk, LineDiff } from "@servef/contracts"

// lineDiffOf is the line diff the server sends for two texts (decision
// 0083), read back from Pierre's own parse, which lays out the same hunks.
// Tests only: the app never diffs in the browser.
export function lineDiffOf(before: string | null, after: string | null): LineDiff {
  const side = (contents: string | null) => contents === null ? null : { name: "notes.txt", contents }
  const fileDiff = parseDiffFromFile(side(before), side(after))
  const hunks = fileDiff.hunks.map((hunk): DiffHunk => {
    // Pierre's headers name the line before an empty side.
    const oldStart = hunk.deletionStart + (hunk.deletionCount === 0 ? 1 : 0)
    const newStart = hunk.additionStart + (hunk.additionCount === 0 ? 1 : 0)
    let oldLine = oldStart
    let newLine = newStart
    const blocks = hunk.hunkContent.map((content): DiffBlock => {
      const oldCount = content.type === "context" ? content.lines : content.deletions
      const newCount = content.type === "context" ? content.lines : content.additions
      const block: DiffBlock = { kind: content.type, oldStart: oldLine, oldCount, newStart: newLine, newCount }
      oldLine += oldCount
      newLine += newCount
      return block
    })
    return {
      oldStart, oldCount: hunk.deletionCount, newStart, newCount: hunk.additionCount,
      additions: hunk.additionLines, deletions: hunk.deletionLines, blocks,
    }
  })
  const gaps: DiffGap[] = []
  let oldLine = 1
  let newLine = 1
  const gap = (index: number, lines: number) => {
    if (lines > 0) gaps.push({ hunk: index, oldStart: oldLine, newStart: newLine, lines })
  }
  for (const [index, hunk] of hunks.entries()) {
    gap(index, hunk.newStart - newLine)
    oldLine = hunk.oldStart + hunk.oldCount
    newLine = hunk.newStart + hunk.newCount
  }
  if (hunks.length > 0) {
    gap(hunks.length, Math.min(fileDiff.deletionLines.length + 1 - oldLine, fileDiff.additionLines.length + 1 - newLine))
  }
  return {
    ignoreWhitespace: false,
    additions: hunks.reduce((sum, hunk) => sum + hunk.additions, 0),
    deletions: hunks.reduce((sum, hunk) => sum + hunk.deletions, 0),
    hunks,
    gaps,
  }
}
