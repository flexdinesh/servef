import type { FileDiffMetadata } from "@pierre/diffs"

export interface DiffStats {
  additions: number
  deletions: number
}

// diffStats counts added and deleted lines across a diff's hunks.
export function diffStats(diff: Pick<FileDiffMetadata, "hunks">): DiffStats {
  let additions = 0
  let deletions = 0
  for (const hunk of diff.hunks) {
    additions += hunk.additionLines
    deletions += hunk.deletionLines
  }
  return { additions, deletions }
}

// diffStatsLabel reads like VS Code's footer: "+12 −3" (U+2212 minus).
export function diffStatsLabel(stats: DiffStats): string {
  return `+${stats.additions.toLocaleString()} −${stats.deletions.toLocaleString()}`
}
