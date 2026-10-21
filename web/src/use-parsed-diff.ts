import { cleanUpResolvedLanguages, cleanUpResolvedThemes, disposeHighlighter, type FileDiffMetadata } from "@pierre/diffs"
import { useEffect, useState } from "react"

import type { DiffContent } from "./diff-content.ts"
import { createDiffParser } from "./diff-parser.ts"

// One worker parses for every diff tab; it starts with the first diff.
const parser = createDiffParser(() => new Worker(new URL("./diff-worker.ts", import.meta.url), { type: "module" }))

// releaseDiffResources frees what diffs hold once the last diff tab closes:
// the parse worker and Pierre's resolved grammars, themes, and main-thread
// highlighter. Highlighting workers already end with the last shown diff.
export function releaseDiffResources(): void {
  parser.release()
  cleanUpResolvedLanguages()
  cleanUpResolvedThemes()
  void disposeHighlighter()
}

type TextContent = Extract<DiffContent, { kind: "text" }>

interface Parsed {
  key: string
  content: TextContent
  ignoreWhitespace: boolean
  fileDiff: FileDiffMetadata | null
}

export interface ParsedDiff {
  // fileDiff is the parsed text diff; while a parse runs it is the tab's
  // previous diff, if any.
  fileDiff: FileDiffMetadata | null
  parsing: boolean
  failed: boolean
}

const noDiff: ParsedDiff = { fileDiff: null, parsing: false, failed: false }

// useParsedDiff parses the open diff tab's text sides off the main thread.
// key names the tab, so a reparse of the same tab keeps its diff on screen.
export function useParsedDiff(key: string, content: DiffContent | null, ignoreWhitespace: boolean): ParsedDiff {
  const text = content?.kind === "text" && (content.oldFile || content.newFile) ? content : null
  const [parsed, setParsed] = useState<Parsed | null>(null)
  // A closed or non-text diff releases the last parse.
  if (!text && parsed) setParsed(null)

  useEffect(() => {
    if (!text) return
    const controller = new AbortController()
    parser.parse(text.oldFile, text.newFile, ignoreWhitespace, controller.signal).then(
      (fileDiff) => { if (!controller.signal.aborted) setParsed({ key, content: text, ignoreWhitespace, fileDiff }) },
      () => { if (!controller.signal.aborted) setParsed({ key, content: text, ignoreWhitespace, fileDiff: null }) },
    )
    return () => controller.abort()
  }, [ignoreWhitespace, key, text])

  if (!text) return noDiff
  if (parsed?.content === text && parsed.ignoreWhitespace === ignoreWhitespace) {
    return { fileDiff: parsed.fileDiff, parsing: false, failed: !parsed.fileDiff }
  }
  return { fileDiff: parsed?.key === key ? parsed.fileDiff : null, parsing: true, failed: false }
}
