import type { HighlightRange } from "../features/search/search-engine.ts"
import type { Theme, ThemeOption } from "../settings/theme.ts"

export interface ThemeMatch {
  option: ThemeOption
  label: readonly HighlightRange[]
}

export interface ThemeGroup {
  // title is VS Code's lowercase separator; Auto has none.
  title: string | null
  matches: readonly ThemeMatch[]
}

function labelRanges(label: string, terms: readonly string[]): HighlightRange[] | null {
  const lower = label.toLowerCase()
  const ranges: HighlightRange[] = []
  for (const term of terms) {
    const start = lower.indexOf(term)
    if (start < 0) return null
    ranges.push({ start, end: start + term.length })
  }
  return ranges.toSorted((a, b) => a.start - b.start).reduce<HighlightRange[]>((merged, range) => {
    const last = merged.at(-1)
    if (last && range.start <= last.end) last.end = Math.max(last.end, range.end)
    else merged.push({ ...range })
    return merged
  }, [])
}

// matchThemes keeps options whose label contains every whitespace-separated
// term, case-insensitively, grouped like VS Code's Color Theme pick: Auto,
// then light themes, then dark themes, in option order. Empty groups drop.
export function matchThemes(options: readonly ThemeOption[], query: string): ThemeGroup[] {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
  const groups: { scheme: ThemeOption["scheme"]; title: string | null; matches: ThemeMatch[] }[] = [
    { scheme: "system", title: null, matches: [] },
    { scheme: "light", title: "light themes", matches: [] },
    { scheme: "dark", title: "dark themes", matches: [] },
  ]
  for (const option of options) {
    const label = labelRanges(option.label, terms)
    if (!label) continue
    groups.find((group) => group.scheme === option.scheme)?.matches.push({ option, label })
  }
  return groups.filter((group) => group.matches.length > 0).map(({ title, matches }) => ({ title, matches }))
}

// pickOrder flattens groups into keyboard order.
export function pickOrder(groups: readonly ThemeGroup[]): ThemeMatch[] {
  return groups.flatMap((group) => group.matches)
}

// initialActiveIndex starts on the current theme, or the first row.
export function initialActiveIndex(matches: readonly ThemeMatch[], current: Theme): number {
  return Math.max(0, matches.findIndex((match) => match.option.value === current))
}
