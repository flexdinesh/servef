import { cleanUpResolvedLanguages, cleanUpResolvedThemes, disposeHighlighter, type FileContents, type FileDiffMetadata } from "@pierre/diffs"

import { createDiffParser } from "./diff-parser.ts"

// One worker parses for every review tab; it starts with the first diff.
const parser = createDiffParser(() => new Worker(new URL("./diff-worker.ts", import.meta.url), { type: "module" }))

// parseDiff parses text sides off the main thread.
export function parseDiff(oldFile: FileContents | null, newFile: FileContents | null, ignoreWhitespace: boolean, signal: AbortSignal): Promise<FileDiffMetadata> {
  return parser.parse(oldFile, newFile, ignoreWhitespace, signal)
}

// releaseDiffResources frees what diffs hold once the last review tab
// closes: the parse worker and Pierre's resolved grammars, themes, and
// main-thread highlighter. Highlighting workers already end with the last
// shown diff.
export function releaseDiffResources(): void {
  parser.release()
  cleanUpResolvedLanguages()
  cleanUpResolvedThemes()
  void disposeHighlighter()
}
