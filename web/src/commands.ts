import type { HighlightRange } from "./search-engine.ts"
import { settingDefinitions, type SettingId } from "./settings.ts"

export type CommandAction =
  | { kind: "open-settings" }
  | { kind: "open-setting"; setting: SettingId }

export interface Command {
  id: string
  category: string
  title: string
  action: CommandAction
}

export interface CommandMatch {
  command: Command
  category: readonly HighlightRange[]
  title: readonly HighlightRange[]
}

// Open Settings comes first; then each setting is a command that opens
// Settings at that setting.
export const commands: readonly Command[] = [
  { id: "preferences.openSettings", category: "Preferences", title: "Open Settings", action: { kind: "open-settings" } },
  ...settingDefinitions.map((definition): Command => ({
    id: `settings.${definition.id}`,
    category: "Settings",
    title: definition.title,
    action: { kind: "open-setting", setting: definition.id },
  })),
]

function mergeRanges(ranges: HighlightRange[]): HighlightRange[] {
  const sorted = ranges.toSorted((a, b) => a.start - b.start)
  const merged: HighlightRange[] = []
  for (const range of sorted) {
    const last = merged.at(-1)
    if (last && range.start <= last.end) last.end = Math.max(last.end, range.end)
    else merged.push({ ...range })
  }
  return merged
}

// matchCommands keeps commands whose category or title contains every
// whitespace-separated term, case-insensitively. Commands with a title word
// starting with the first term rank first; otherwise the list order holds.
export function matchCommands(list: readonly Command[], query: string): CommandMatch[] {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
  const matches: CommandMatch[] = []
  for (const command of list) {
    const category = command.category.toLowerCase()
    const title = command.title.toLowerCase()
    const categoryRanges: HighlightRange[] = []
    const titleRanges: HighlightRange[] = []
    const matched = terms.every((term) => {
      const inTitle = title.indexOf(term)
      if (inTitle >= 0) {
        titleRanges.push({ start: inTitle, end: inTitle + term.length })
        return true
      }
      const inCategory = category.indexOf(term)
      if (inCategory < 0) return false
      categoryRanges.push({ start: inCategory, end: inCategory + term.length })
      return true
    })
    if (matched) matches.push({ command, category: mergeRanges(categoryRanges), title: mergeRanges(titleRanges) })
  }
  const first = terms[0]
  if (!first) return matches
  const rank = (match: CommandMatch) => match.command.title.toLowerCase().split(/\s+/).some((word) => word.startsWith(first)) ? 0 : 1
  return matches.toSorted((a, b) => rank(a) - rank(b))
}
