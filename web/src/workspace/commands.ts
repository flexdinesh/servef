import type { HighlightRange } from "../features/search/search-engine.ts"
import { settingDefinitions, type SettingId } from "../settings/settings.ts"

const mac = typeof navigator !== "undefined" && navigator.platform.includes("Mac")

export type CommandAction =
  | { kind: "open-settings" }
  | { kind: "select-color-theme" }
  | { kind: "open-setting"; setting: SettingId }
  | { kind: "go-to-definition" }
  | { kind: "find-references" }
  | { kind: "walkthrough-step"; direction: 1 | -1 }
  | { kind: "walkthrough-exit" }

export interface Command {
  id: string
  category: string
  title: string
  action: CommandAction
  // shortcut labels a command's key binding when it is not Settings'.
  shortcut?: string
}

export interface CommandMatch {
  command: Command
  category: readonly HighlightRange[]
  title: readonly HighlightRange[]
}

// Open Settings and the Color Theme pick come first; then each setting is a
// command that opens Settings at that setting; then code navigation, which
// acts on the name last clicked in Source; then walkthrough stepping, which
// acts on the walkthrough last opened.
export const commands: readonly Command[] = [
  { id: "preferences.openSettings", category: "Preferences", title: "Open Settings", action: { kind: "open-settings" } },
  { id: "preferences.colorTheme", category: "Preferences", title: "Color Theme", action: { kind: "select-color-theme" } },
  ...settingDefinitions.map((definition): Command => ({
    id: `settings.${definition.id}`,
    category: "Settings",
    title: definition.title,
    action: { kind: "open-setting", setting: definition.id },
  })),
  { id: "navigation.goToDefinition", category: "Navigation", title: "Go to Definition", action: { kind: "go-to-definition" }, shortcut: "F12" },
  { id: "navigation.findReferences", category: "Navigation", title: "Find All References", action: { kind: "find-references" }, shortcut: "Shift+F12" },
  { id: "walkthrough.next", category: "Walkthrough", title: "Next Step", action: { kind: "walkthrough-step", direction: 1 }, shortcut: mac ? "⌘→" : "Ctrl+Right" },
  { id: "walkthrough.previous", category: "Walkthrough", title: "Previous Step", action: { kind: "walkthrough-step", direction: -1 }, shortcut: mac ? "⌘←" : "Ctrl+Left" },
  { id: "walkthrough.exit", category: "Walkthrough", title: "Close Walkthrough", action: { kind: "walkthrough-exit" } },
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
