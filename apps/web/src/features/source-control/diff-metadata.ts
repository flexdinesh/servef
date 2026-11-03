import type { ChangeContent, ChangeTypes, ContextContent, FileContents, FileDiffMetadata, Hunk } from "@pierre/diffs"

import type { DiffBlock, DiffHunk, LineDiff } from "@servef/contracts"

// fileDiffMetadata lays the server's line diff (decision 0083) out as the
// FileDiffMetadata Pierre renders, over both whole sides, as Pierre's own
// parseDiffFromFile would; the browser never diffs. Pierre's hunk headers
// name the line before an empty side, and its line indexes count lines
// before a row from 0.
export function fileDiffMetadata(oldFile: FileContents | null, newFile: FileContents | null, lineDiff: LineDiff): FileDiffMetadata {
  const deletionLines = splitLines(oldFile?.contents ?? "")
  const additionLines = splitLines(newFile?.contents ?? "")
  const hunks: Hunk[] = []
  let splitLineCount = 0
  let unifiedLineCount = 0
  for (const [index, hunk] of lineDiff.hunks.entries()) {
    const collapsedBefore = gapLines(lineDiff, index)
    const shown = pierreHunk(hunk, deletionLines.length, additionLines.length, oldFile, newFile)
    shown.collapsedBefore = collapsedBefore
    shown.splitLineStart = splitLineCount + collapsedBefore
    shown.unifiedLineStart = unifiedLineCount + collapsedBefore
    splitLineCount += collapsedBefore + shown.splitLineCount
    unifiedLineCount += collapsedBefore + shown.unifiedLineCount
    hunks.push(shown)
  }
  if (hunks.length > 0 && deletionLines.length > 0 && additionLines.length > 0) {
    const collapsedAfter = gapLines(lineDiff, hunks.length)
    splitLineCount += collapsedAfter
    unifiedLineCount += collapsedAfter
  }
  const metadata: FileDiffMetadata = {
    name: newFile?.name ?? oldFile?.name ?? "",
    type: changeType(oldFile, newFile, hunks.length),
    hunks,
    splitLineCount,
    unifiedLineCount,
    isPartial: false,
    deletionLines,
    additionLines,
    deletionCharacterCount: oldFile?.contents.length ?? 0,
    additionCharacterCount: newFile?.contents.length ?? 0,
  }
  if (metadata.type === "rename-pure" || metadata.type === "rename-changed") metadata.prevName = oldFile?.name
  const lang = newFile ? newFile.lang : oldFile?.lang
  if (lang !== undefined) metadata.lang = lang
  return metadata
}

// gapLines counts the unchanged lines collapsed before hunk index.
function gapLines(lineDiff: LineDiff, index: number): number {
  return lineDiff.gaps.find((gap) => gap.hunk === index)?.lines ?? 0
}

// headerStart is a hunk side's start as its header names it.
function headerStart(start: number, count: number): number {
  return count === 0 ? start - 1 : start
}

function pierreHunk(hunk: DiffHunk, oldLines: number, newLines: number, oldFile: FileContents | null, newFile: FileContents | null): Hunk {
  const deletionStart = headerStart(hunk.oldStart, hunk.oldCount)
  const additionStart = headerStart(hunk.newStart, hunk.newCount)
  // An empty side never advances: each of its rows indexes the header's line.
  const deletionIndex = (block: DiffBlock) => hunk.oldCount === 0 ? deletionStart - 1 : block.oldStart - 1
  const additionIndex = (block: DiffBlock) => hunk.newCount === 0 ? additionStart - 1 : block.newStart - 1
  const hunkContent = hunk.blocks.map((block): ContextContent | ChangeContent => block.kind === "context"
    ? { type: "context", lines: block.oldCount, deletionLineIndex: deletionIndex(block), additionLineIndex: additionIndex(block) }
    : { type: "change", deletions: block.oldCount, additions: block.newCount, deletionLineIndex: deletionIndex(block), additionLineIndex: additionIndex(block) })
  let splitLineCount = 0
  let unifiedLineCount = 0
  for (const content of hunkContent) {
    splitLineCount += content.type === "context" ? content.lines : Math.max(content.additions, content.deletions)
    unifiedLineCount += content.type === "context" ? content.lines : content.additions + content.deletions
  }
  // A side's last line without a newline is marked; a mark after context
  // covers both sides.
  const oldEnd = endsWithoutNewline(oldFile, hunk.oldStart, hunk.oldCount, oldLines)
  const newEnd = endsWithoutNewline(newFile, hunk.newStart, hunk.newCount, newLines)
  const endsInContext = hunk.blocks.at(-1)?.kind === "context"
  return {
    collapsedBefore: 0,
    additionStart,
    additionCount: hunk.newCount,
    additionLines: hunk.additions,
    additionLineIndex: additionStart - 1,
    deletionStart,
    deletionCount: hunk.oldCount,
    deletionLines: hunk.deletions,
    deletionLineIndex: deletionStart - 1,
    hunkContent,
    hunkSpecs: `@@ -${deletionStart},${hunk.oldCount} +${additionStart},${hunk.newCount} @@\n`,
    splitLineStart: 0,
    splitLineCount,
    unifiedLineStart: 0,
    unifiedLineCount,
    noEOFCRDeletions: oldEnd || (endsInContext && newEnd),
    noEOFCRAdditions: newEnd || (endsInContext && oldEnd),
  }
}

// endsWithoutNewline reports a hunk side that reaches its file's last line,
// which lacks a newline.
function endsWithoutNewline(file: FileContents | null, start: number, count: number, lines: number): boolean {
  return file !== null && count > 0 && start + count - 1 === lines && !file.contents.endsWith("\n")
}

// changeType names the change as Pierre does from file names and contents.
function changeType(oldFile: FileContents | null, newFile: FileContents | null, hunks: number): ChangeTypes {
  if (!oldFile) return "new"
  if (!newFile) return "deleted"
  if (oldFile.name !== newFile.name) return hunks > 0 ? "rename-changed" : "rename-pure"
  if (oldFile.contents === "" && newFile.contents !== "") return "new"
  if (oldFile.contents !== "" && newFile.contents === "") return "deleted"
  return "change"
}

// splitLines splits after each "\n", keeping it; empty contents have no lines.
function splitLines(contents: string): string[] {
  const lines: string[] = []
  let start = 0
  while (start < contents.length) {
    const end = contents.indexOf("\n", start)
    const next = end === -1 ? contents.length : end + 1
    lines.push(contents.slice(start, next))
    start = next
  }
  return lines
}
