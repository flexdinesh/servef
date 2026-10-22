// HighlightsModule is what servef uses from @pierre/highlights.
export type HighlightsModule = Pick<typeof import("@pierre/highlights"), "codeToTokens" | "isSupportedLanguage">

let loading: Promise<HighlightsModule | null> | undefined

// loadHighlights imports Highlights once per thread. Its WebAssembly needs
// SIMD and initializes at import; where that fails, code stays plain text
// and the app keeps working (decision 0029).
export function loadHighlights(): Promise<HighlightsModule | null> {
  loading ??= import("@pierre/highlights").then(
    (module): HighlightsModule => module,
    () => null,
  )
  return loading
}
