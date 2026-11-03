import type { FileStats } from "@servef/contracts"

export interface DiffStats {
  additions: number
  deletions: number
}

// countedLines is a file's line counts, or null unless they were counted.
export function countedLines(stats: FileStats | undefined): DiffStats | null {
  if (stats?.status !== "counted" || stats.additions === undefined || stats.deletions === undefined) return null
  return { additions: stats.additions, deletions: stats.deletions }
}

// diffStatsLabel reads like VS Code's footer: "+12 −3" (U+2212 minus).
export function diffStatsLabel(stats: DiffStats): string {
  return `+${stats.additions.toLocaleString()} −${stats.deletions.toLocaleString()}`
}
