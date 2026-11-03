// shiki-unavailable stands in for every `shiki` entry when production builds
// bundle @pierre/diffs (vite.config.ts, decision 0029). servef highlights
// only with Highlights, so Pierre's Shiki backend never loads; these exports
// exist only so the bundle links.
function unavailable(): never {
  throw new Error("servef does not bundle Shiki")
}

export const createCssVariablesTheme = unavailable
export const createHighlighterCore = unavailable
export const createJavaScriptRegexEngine = unavailable
export const createOnigurumaEngine = unavailable
export const normalizeTheme = unavailable
export const EncodedTokenMetadata = { getForeground: unavailable, getTokenType: unavailable }
export const INITIAL = null
export const bundledLanguages = {}
export default unavailable
