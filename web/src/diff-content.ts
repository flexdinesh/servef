import { parseDiffFromFile, type FileContents, type FileDiffMetadata } from "@pierre/diffs"

import type { DiffResponse, DiffSide } from "./changes-contract.ts"
import { diffLanguage } from "./diff-language.ts"
import { formatBytes } from "./format-bytes.ts"

// DiffContent is what a working-tree diff can show: text sides for Pierre, or
// the reason it cannot (decision 0004 limits apply per side).
export type DiffContent =
  | { kind: "text"; oldFile: FileContents | null; newFile: FileContents | null }
  | { kind: "binary"; before: number | null; after: number | null }
  | { kind: "tooLarge"; size: number }

function textFile(name: string, side: DiffSide | null, lang: FileContents["lang"]): FileContents | null {
  return side ? { name, contents: side.content ?? "", lang } : null
}

export function diffContent(diff: DiffResponse): DiffContent {
  const { original, modified } = diff
  if (original?.binary || modified?.binary) {
    return { kind: "binary", before: original?.size ?? null, after: modified?.size ?? null }
  }
  if (original?.tooLarge || modified?.tooLarge) {
    return { kind: "tooLarge", size: Math.max(original?.size ?? 0, modified?.size ?? 0) }
  }
  // The server's file type picks the grammar; Pierre never guesses one.
  const lang = diffLanguage(diff.documentType.id)
  return {
    kind: "text",
    oldFile: textFile(diff.oldPath || diff.path, original, lang),
    newFile: textFile(diff.path, modified, lang),
  }
}

// binaryChangeLabel reads "Binary file changed · 12 KB → 14 KB".
export function binaryChangeLabel(before: number | null, after: number | null): string {
  if (before !== null && after !== null) return `Binary file changed · ${formatBytes(before)} → ${formatBytes(after)}`
  if (after !== null) return `Binary file added · ${formatBytes(after)}`
  return `Binary file deleted · ${formatBytes(before ?? 0)}`
}

// parseTextDiff runs jsdiff over both text sides; whitespace is ignored at the
// start and end of lines when asked. It returns null for non-text content.
export function parseTextDiff(content: DiffContent, ignoreWhitespace: boolean): FileDiffMetadata | null {
  if (content.kind !== "text" || (!content.oldFile && !content.newFile)) return null
  return parseDiffFromFile(content.oldFile, content.newFile, { ignoreWhitespace })
}
