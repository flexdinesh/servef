import { useEffect, useState } from "react"

import { isCompareStatsResponse, type CompareStatsResponse, type FileStats } from "../../api/compare-stats-contract.ts"
import { historyError } from "../../api/commits-contract.ts"
import { failureReason, fetchJSON } from "../../api/http.ts"
import { boundedCache } from "../../lib/bounded-cache.ts"
import { compareKey, type Compare } from "./compare.ts"
import { diffStatsLabel } from "./diff-stats.ts"

// Line stats are on-demand work (decision 0035): the Files pane and the
// review tab read them after the file list, from /api/compare-stats. Reads
// are shared by key, so the pane and the review of one compare read once.

export interface CompareStats {
  byPath: ReadonlyMap<string, FileStats>
  budgetExceeded: boolean
}

function statsUrl(project: string, compare: Compare): string {
  const params = new URLSearchParams({ project, compare: compare.kind })
  if (compare.kind === "branch" && compare.base) params.set("base", compare.base)
  if (compare.kind === "branch" && compare.head) params.set("head", compare.head)
  if (compare.kind === "commit") params.set("commit", compare.hash)
  if (compare.kind === "range") {
    params.set("from", compare.from)
    params.set("to", compare.to)
  }
  return `/api/compare-stats?${params.toString()}`
}

function indexed(response: CompareStatsResponse): CompareStats {
  return { byPath: new Map(response.files.map((file): [string, FileStats] => [file.path, file])), budgetExceeded: response.budgetExceeded }
}

// cache shares recent reads.
const cache = boundedCache<Promise<CompareStats>>(8)

function readStats(key: string, url: string): Promise<CompareStats> {
  const known = cache.get(key)
  if (known) return known
  const read = fetchJSON(url, {}, isCompareStatsResponse, (failure) => historyError(failureReason(failure))).then(indexed)
  // A failed read is not kept, so the next request retries.
  read.catch(() => { if (cache.get(key) === read) cache.delete(key) })
  cache.set(key, read)
  return read
}

// useCompareStats reads a compare's line stats once its file list is ready.
// version names the list read, so live compares reread as they move; null
// waits.
export function useCompareStats(project: string, compare: Compare | null, version: string | null): CompareStats | null {
  const key = project && compare && version !== null ? `${project}\n${compareKey(compare)}\n${version}` : ""
  const owner = project && compare ? `${project}\n${compareKey(compare)}` : ""
  const [entry, setEntry] = useState<{ owner: string; stats: CompareStats } | null>(null)
  useEffect(() => {
    if (!key || !compare) return
    let current = true
    readStats(key, statsUrl(project, compare)).then((stats) => {
      if (current) setEntry({ owner, stats })
    }, () => {
      // Stats are optional; rows show status letters without them.
    })
    return () => { current = false }
  }, [compare, key, owner, project])
  // A reread keeps the last stats of the same compare until it lands.
  return entry && entry.owner === owner ? entry.stats : null
}

// statsLabel reads "+12 −8", "binary", or "" when unknown.
export function statsLabel(stats: FileStats | undefined): string {
  if (!stats) return ""
  if (stats.status === "binary") return "binary"
  if (stats.status === "tooLarge") return "large"
  if (stats.status !== "counted") return ""
  return diffStatsLabel(stats)
}

// totals sums counted files' lines.
export function totals(stats: CompareStats | null): { additions: number; deletions: number } | null {
  if (!stats) return null
  let additions = 0
  let deletions = 0
  for (const file of stats.byPath.values()) {
    if (file.status !== "counted") continue
    additions += file.additions
    deletions += file.deletions
  }
  return { additions, deletions }
}
