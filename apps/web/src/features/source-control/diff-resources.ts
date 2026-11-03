import { cleanUpResolvedLanguages, cleanUpResolvedThemes, disposeHighlighter } from "@pierre/diffs"

// releaseDiffResources frees what diffs hold once the last review tab
// closes: Pierre's resolved grammars, themes, and main-thread highlighter.
// Highlighting workers already end with the last shown diff.
export function releaseDiffResources(): void {
  cleanUpResolvedLanguages()
  cleanUpResolvedThemes()
  void disposeHighlighter()
}
