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

// fileDecoration is a changed file's row decoration: line stats with
// additions in success and deletions in destructive, as elsewhere in Source
// Control; "binary" or "large" as plain notes; or, for a renamed file without
// stats, where it came from. A file with uncommitted edits in a review that
// includes them leads with an accent dot (decision 0051). The title names
// them, the rename, and the lines.
export function fileDecoration(stats: FileStats | undefined, oldPath: string | undefined, marked = false): FileDecoration | null {
  const lines = statsLabel(stats)
  const title = [marked ? "Has uncommitted changes" : "", oldPath ? `Renamed from ${oldPath}` : "", lines ? `${lines} lines` : ""]
    .filter(Boolean).join(", ")
  const text = lines || (oldPath ? `from ${oldPath}` : "")
  // Pierre lays parts out as flex items, which drop a plain space.
  const parts: DecorationPart[] = stats?.status === "counted"
    ? [stat("+", stats.additions, "var(--color-success)"), { text: "\u00a0" }, stat("−", stats.deletions, "var(--color-destructive)")]
    : text ? [{ text }] : []
  if (marked) parts.unshift({ text: "●", color: "var(--color-accent)" }, ...(parts.length > 0 ? [{ text: "\u00a0" }] : []))
  if (parts.length === 0) return null
  if (stats?.status !== "counted" && !marked) return { text, title }
  return { text: [marked ? "●" : "", text].filter(Boolean).join(" "), title, parts }
}
