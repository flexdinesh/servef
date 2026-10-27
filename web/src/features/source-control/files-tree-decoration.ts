import type { FileStats } from "../../api/compare-stats-contract.ts"
import { statsLabel } from "./use-compare-stats.ts"

// DecorationPart is a run of a Files tree row's trailing text; color is a
// CSS color, inherited by the tree's shadow root.
export interface DecorationPart {
  text: string
  color?: string
}

export interface FileDecoration {
  text: string
  title: string
  parts?: readonly DecorationPart[]
}

// stat is one count as DiffStatsText colors it: a zero keeps the row's ink.
function stat(sign: string, count: number, color: string): DecorationPart {
  return { text: `${sign}${count.toLocaleString()}`, ...(count === 0 ? {} : { color }) }
}

// RowMarks are a row's review marks: uncommitted edits in a review that
// includes them (decision 0051), and the Viewed mark (decision 0053).
export interface RowMarks {
  marked?: boolean
  viewed?: boolean
}

// fileDecoration is a changed file's row decoration: line stats with
// additions in success and deletions in destructive, as elsewhere in Source
// Control; "binary" or "large" as plain notes; or, for a renamed file without
// stats, where it came from. A marked file leads with an accent dot. The
// title names the marks, the rename, and the lines; a viewed file keeps a
// lane for viewedCSS's check.
export function fileDecoration(stats: FileStats | undefined, oldPath: string | undefined, { marked = false, viewed = false }: RowMarks = {}): FileDecoration | null {
  const lines = statsLabel(stats)
  const title = [marked ? "Has uncommitted changes" : "", oldPath ? `Renamed from ${oldPath}` : "", lines ? `${lines} lines` : "", viewed ? "Viewed" : ""]
    .filter(Boolean).join(", ")
  const text = lines || (oldPath ? `from ${oldPath}` : "")
  // Pierre lays parts out as flex items, which drop a plain space.
  const parts: DecorationPart[] = stats?.status === "counted"
    ? [stat("+", stats.additions, "var(--color-success)"), { text: "\u00a0" }, stat("−", stats.deletions, "var(--color-destructive)")]
    : text ? [{ text }] : []
  if (marked) parts.unshift({ text: "●", color: "var(--color-accent)" }, ...(parts.length > 0 ? [{ text: "\u00a0" }] : []))
  if (parts.length === 0) return viewed ? { text: "", title } : null
  if (stats?.status !== "counted" && !marked) return { text, title }
  return { text: [marked ? "●" : "", text].filter(Boolean).join(" "), title, parts }
}

// attributeValue quotes text as a CSS attribute selector's value.
function attributeValue(text: string): string {
  return `"${text.replaceAll("\\", "\\\\").replaceAll("\"", "\\\"").replaceAll("\n", "\\a ")}"`
}

// viewedIconOpacity dims a viewed row's file icon. Its name takes muted ink
// instead of an opacity: muted holds 4.5:1 in every theme, a dimmed status
// hue does not.
const viewedIconOpacity = 0.6

// checkIcon is lucide's Check, the Viewed toggle's icon, as a mask.
const checkIcon = `url("data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="black" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>')}")`

// viewedCSS checks the tree rows of viewed files, before their stats in the
// lane's muted ink, and dims them, except the selected file in view, which
// keeps full ink. The status letter keeps its hue.
export function viewedCSS(paths: readonly string[]): string {
  if (paths.length === 0) return ""
  const rows = paths.map((path) => `[data-item-path=${attributeValue(path)}]`)
  const dimmed = `:is(${rows.map((row) => `${row}:not([data-item-selected])`).join(", ")})`
  return `
:is(${rows.join(", ")}) > [data-item-section="decoration"]::before { content: ""; flex: none; align-self: center; width: var(--icon-xs); height: var(--icon-xs); margin-right: var(--space-1); background: currentColor; mask: ${checkIcon} center / contain no-repeat; }
${dimmed} > [data-item-section="icon"] { opacity: ${viewedIconOpacity}; }
${dimmed} > [data-item-section="content"] { color: var(--color-text-muted); }`
}
