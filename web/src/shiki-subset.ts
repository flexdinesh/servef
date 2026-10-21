import { createBundledHighlighter, createSingletonShorthands } from "shiki/core"
import { createJavaScriptRegexEngine } from "shiki/engine/javascript"

import { diffGrammars } from "./diff-grammars.ts"

// shiki-subset stands in for the `shiki` entry when production builds bundle
// @pierre/diffs (vite.config.ts, decision 0024). It exports what Pierre imports
// from `shiki`, with only servef's diff grammars and no bundled themes.
export { createCssVariablesTheme, getTokenStyleObject, stringifyTokenStyle } from "shiki/core"
export { createOnigurumaEngine } from "shiki/engine/oniguruma"
export { createJavaScriptRegexEngine }

export const bundledLanguages = diffGrammars

export const createHighlighter = createBundledHighlighter({
  langs: diffGrammars,
  themes: {},
  engine: () => createJavaScriptRegexEngine(),
})

export const { codeToHtml } = createSingletonShorthands(createHighlighter)
