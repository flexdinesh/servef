import type { FileStats } from "../../api/compare-stats-contract.ts"
import type { DiffStats } from "./diff-stats.ts"
import { statsLabel } from "./use-compare-stats.ts"

// DiffStatsText renders "+12 −3" with additions in success and deletions in
// destructive; a zero count keeps the surrounding ink.
export function DiffStatsText({ additions, deletions }: DiffStats) {
  return <>
    <span className="diff-stat" data-kind="added" data-zero={additions === 0 || undefined}>+{additions.toLocaleString()}</span>
    {" "}
    <span className="diff-stat" data-kind="deleted" data-zero={deletions === 0 || undefined}>−{deletions.toLocaleString()}</span>
  </>
}

// FileStatsText colors counted stats and keeps statsLabel's plain notes
// ("binary", "large") for the rest.
export function FileStatsText({ stats }: { stats: FileStats | undefined }) {
  return stats?.status === "counted" ? <DiffStatsText additions={stats.additions} deletions={stats.deletions} /> : statsLabel(stats)
}
