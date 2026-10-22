// Diff toolbar options, persisted per browser under servef-diff-options.

export const DIFF_OPTIONS_STORAGE_KEY = "servef-diff-options"

export type DiffStyle = "split" | "unified"
// InlineChanges is the within-line emphasis of changed lines.
export type InlineChanges = "word" | "char" | "none"

export interface DiffOptions {
  style: DiffStyle
  wrap: boolean
  ignoreWhitespace: boolean
  inline: InlineChanges
  expandUnchanged: boolean
}

export const defaultDiffOptions: DiffOptions = {
  style: "split",
  wrap: false,
  ignoreWhitespace: false,
  inline: "word",
  expandUnchanged: false,
}

export const inlineChoices: ReadonlyArray<{ value: InlineChanges; label: string }> = [
  { value: "word", label: "Word" },
  { value: "char", label: "Character" },
  { value: "none", label: "Off" },
]

function isInlineChanges(value: unknown): value is InlineChanges {
  return value === "word" || value === "char" || value === "none"
}

// parseDiffOptions keeps every valid stored option and defaults the rest.
export function parseDiffOptions(value: unknown): DiffOptions {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return defaultDiffOptions
  const stored: Partial<Record<keyof DiffOptions, unknown>> = value
  return {
    style: stored.style === "split" || stored.style === "unified" ? stored.style : defaultDiffOptions.style,
    wrap: typeof stored.wrap === "boolean" ? stored.wrap : defaultDiffOptions.wrap,
    ignoreWhitespace: typeof stored.ignoreWhitespace === "boolean" ? stored.ignoreWhitespace : defaultDiffOptions.ignoreWhitespace,
    inline: isInlineChanges(stored.inline) ? stored.inline : defaultDiffOptions.inline,
    expandUnchanged: typeof stored.expandUnchanged === "boolean" ? stored.expandUnchanged : defaultDiffOptions.expandUnchanged,
  }
}

// effectiveDiffStyle forces Unified where Split has no room.
export function effectiveDiffStyle(options: DiffOptions, narrow: boolean): DiffStyle {
  return narrow ? "unified" : options.style
}

export function readDiffOptions(): DiffOptions {
  try {
    const raw = window.localStorage.getItem(DIFF_OPTIONS_STORAGE_KEY)
    return raw ? parseDiffOptions(JSON.parse(raw)) : defaultDiffOptions
  } catch {
    return defaultDiffOptions
  }
}

export function storeDiffOptions(options: DiffOptions): void {
  try {
    window.localStorage.setItem(DIFF_OPTIONS_STORAGE_KEY, JSON.stringify(options))
  } catch {
    // Options still apply for the session when storage is unavailable.
  }
}
