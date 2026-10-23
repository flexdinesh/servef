import type { SelectedLineRange } from "@pierre/diffs"

import type { Commit } from "../../api/commits-contract.ts"
import { isRecord } from "../../api/http.ts"
import { isCommitHash, shortHash } from "../../lib/commit-hash.ts"
import { diffLineAnchor, parseDiffLineAnchor } from "./diff-line-selection.ts"

// Pure model of what Source Control compares (decision 0035): the working
// tree against HEAD, the branch since its merge-base with a base branch, one
// commit against its first parent, or a contiguous commit range from the
// first parent of from (the oldest) to to (the newest). One review tab shows
// each compare.

export type Compare =
  | { kind: "working" }
  // base is the chosen base branch, "" for the server's default.
  | { kind: "branch"; base: string }
  | { kind: "commit"; hash: string }
  | { kind: "range"; from: string; to: string }

export const workingCompare: Compare = { kind: "working" }

// compareKey identifies a compare uniquely.
export function compareKey(compare: Compare): string {
  if (compare.kind === "branch") return `branch:${compare.base}`
  if (compare.kind === "commit") return `commit:${compare.hash}`
  if (compare.kind === "range") return `range:${compare.from}:${compare.to}`
  return "working"
}

export function sameCompare(left: Compare | null, right: Compare | null): boolean {
  return left !== null && right !== null && compareKey(left) === compareKey(right)
}

// live compares change as the working tree or branch moves; commits never do.
export function isLiveCompare(compare: Compare): boolean {
  return compare.kind === "working" || compare.kind === "branch"
}

export interface CompareContext {
  // base is the branch compare's resolved base, "" until known.
  base: string
  // subject finds a listed commit's subject, "" when unlisted.
  subject(hash: string): string
}

export interface CompareName {
  label: string
  title: string
}

const subjectLength = 40

function clip(text: string): string {
  return text.length > subjectLength ? `${text.slice(0, subjectLength - 1).trimEnd()}…` : text
}

// compareName labels a review tab: "Uncommitted", "vs main", "3577ec6 fix:
// preserve…", or "a1b2c3d..e4f5g6h".
export function compareName(compare: Compare, context: CompareContext): CompareName {
  if (compare.kind === "working") return { label: "Uncommitted", title: "Uncommitted changes" }
  if (compare.kind === "branch") {
    const base = compare.base || context.base
    return base
      ? { label: `vs ${base}`, title: `Changes on this branch since it left ${base}` }
      : { label: "Branch", title: "Changes on this branch" }
  }
  if (compare.kind === "commit") {
    const subject = context.subject(compare.hash)
    const hash = shortHash(compare.hash)
    return { label: subject ? `${hash} ${clip(subject)}` : hash, title: subject ? `${hash} ${subject}` : `Commit ${hash}` }
  }
  const span = `${shortHash(compare.from)}..${shortHash(compare.to)}`
  return { label: span, title: `Changes in commits ${shortHash(compare.from)} through ${shortHash(compare.to)}` }
}

// CompareSearch is the /view search that opens a compare's review tab:
// review=working, review=branch (with base when chosen), commit=<hash>, or
// from=<hash>&to=<hash>, without a path.
export interface CompareSearch {
  review?: "working" | "branch"
  base?: string
  commit?: string
  from?: string
  to?: string
}

export function compareSearch(compare: Compare): CompareSearch {
  if (compare.kind === "working") return { review: "working" }
  if (compare.kind === "branch") return compare.base ? { review: "branch", base: compare.base } : { review: "branch" }
  if (compare.kind === "commit") return { commit: compare.hash }
  return { from: compare.from, to: compare.to }
}

// RouteSearch is the part of a /view search that may select a review tab.
export interface RouteSearch {
  path?: unknown
  review?: unknown
  base?: unknown
  commit?: unknown
  from?: unknown
  to?: unknown
}

// routeCompare is the compare a pathless route reviews, else null.
export function routeCompare(search: RouteSearch): Compare | null {
  if (typeof search.path === "string" && search.path) return null
  if (search.review === "working") return workingCompare
  if (search.review === "branch") return { kind: "branch", base: typeof search.base === "string" ? search.base : "" }
  if (isCommitHash(search.commit)) return { kind: "commit", hash: search.commit }
  if (isCommitHash(search.from) && isCommitHash(search.to)) return { kind: "range", from: search.from, to: search.to }
  return null
}

// LegacySearch is a per-file diff route from before review tabs (decisions
// 0024, 0027, 0030): a path with diff=1, commit=, branch=1, or from=&to=.
export interface LegacySearch {
  path?: unknown
  diff?: unknown
  commit?: unknown
  branch?: unknown
  from?: unknown
  to?: unknown
}

// legacyCompare is the compare a per-file diff route now reviews, with the
// file it showed, else null.
export function legacyCompare(search: LegacySearch): { compare: Compare; path: string } | null {
  const path = typeof search.path === "string" ? search.path : ""
  if (!path) return null
  if (isCommitHash(search.commit)) return { compare: { kind: "commit", hash: search.commit }, path }
  if (isCommitHash(search.from) && isCommitHash(search.to)) return { compare: { kind: "range", from: search.from, to: search.to }, path }
  if (search.branch === 1 || search.branch === "1") return { compare: { kind: "branch", base: "" }, path }
  if (search.diff === 1 || search.diff === "1") return { compare: workingCompare, path }
  return null
}

// StoredCompare is a remembered review tab.
export type StoredCompare =
  | { review: "working" }
  | { review: "branch"; base: string }
  | { review: "commit"; commit: string }
  | { review: "range"; from: string; to: string }

export function storedCompare(compare: Compare): StoredCompare {
  if (compare.kind === "working") return { review: "working" }
  if (compare.kind === "branch") return { review: "branch", base: compare.base }
  if (compare.kind === "commit") return { review: "commit", commit: compare.hash }
  return { review: "range", from: compare.from, to: compare.to }
}

// parseStoredCompare accepts review entries and the per-file diff entries
// earlier versions stored: {diff: path}, {commit, path}, {branch: path}, and
// {from, to, path}.
export function parseStoredCompare(value: unknown): Compare | null {
  if (!isRecord(value)) return null
  if ("review" in value) {
    if (value.review === "working") return workingCompare
    if (value.review === "branch") return { kind: "branch", base: typeof value.base === "string" ? value.base : "" }
    if (value.review === "commit") return isCommitHash(value.commit) ? { kind: "commit", hash: value.commit } : null
    if (value.review === "range") {
      return isCommitHash(value.from) && isCommitHash(value.to) ? { kind: "range", from: value.from, to: value.to } : null
    }
    return null
  }
  if ("commit" in value) return isCommitHash(value.commit) ? { kind: "commit", hash: value.commit } : null
  if ("from" in value || "to" in value) {
    return isCommitHash(value.from) && isCommitHash(value.to) ? { kind: "range", from: value.from, to: value.to } : null
  }
  if ("branch" in value) return typeof value.branch === "string" && value.branch ? { kind: "branch", base: "" } : null
  if ("diff" in value) return typeof value.diff === "string" && value.diff ? workingCompare : null
  return null
}

// ReviewFragment is a review tab's URL fragment: the file in view and its
// selected lines, as #f=<path>&R12-R20.
export interface ReviewFragment {
  path: string
  lines: SelectedLineRange | null
}

export function parseReviewFragment(hash: string): ReviewFragment | null {
  const text = hash.startsWith("#") ? hash.slice(1) : hash
  if (!text.startsWith("f=")) return null
  const separator = text.indexOf("&")
  const encoded = separator < 0 ? text.slice(2) : text.slice(2, separator)
  let path: string
  try {
    path = decodeURIComponent(encoded)
  } catch {
    return null
  }
  if (!path) return null
  return { path, lines: separator < 0 ? null : parseDiffLineAnchor(text.slice(separator + 1)) }
}

// reviewFragment formats a fragment without the leading #. Slashes stay
// readable.
export function reviewFragment(path: string, lines: SelectedLineRange | null = null): string {
  const encoded = encodeURIComponent(path).replaceAll("%2F", "/")
  return lines ? `f=${encoded}&${diffLineAnchor(lines)}` : `f=${encoded}`
}

// selectionCompare is what selecting the listed commits from anchor through
// focus compares: the commit itself, or the contiguous run between them, in
// either direction. It is null unless both are listed.
export function selectionCompare(commits: readonly Commit[], anchor: string, focus: string): Compare | null {
  const start = commits.findIndex((commit) => commit.hash === anchor)
  const end = commits.findIndex((commit) => commit.hash === focus)
  if (start < 0 || end < 0) return null
  const newest = commits[Math.min(start, end)]
  const oldest = commits[Math.max(start, end)]
  if (!newest || !oldest) return null
  return start === end ? { kind: "commit", hash: newest.hash } : { kind: "range", from: oldest.hash, to: newest.hash }
}

// selectedHashes lists the commits a compare covers, newest first: one for
// a commit, the run for a range, none for other compares or when either end
// is no longer listed.
export function selectedHashes(commits: readonly Commit[], compare: Compare | null): string[] {
  if (compare?.kind === "commit") return commits.some((commit) => commit.hash === compare.hash) ? [compare.hash] : []
  if (compare?.kind !== "range") return []
  const newest = commits.findIndex((commit) => commit.hash === compare.to)
  const oldest = commits.findIndex((commit) => commit.hash === compare.from)
  if (newest < 0 || oldest <= newest) return []
  return commits.slice(newest, oldest + 1).map((commit) => commit.hash)
}

// rangeCount counts a listed range's commits, 0 when unlisted.
export function rangeCount(commits: readonly Commit[], compare: Compare): number {
  return compare.kind === "range" ? selectedHashes(commits, compare).length : 0
}

const minute = 60_000
const hour = 60 * minute
const day = 24 * hour

const ageUnits: ReadonlyArray<{ size: number; short: string; unit: Intl.RelativeTimeFormatUnit }> = [
  { size: 365 * day, short: "y", unit: "year" },
  { size: 30 * day, short: "mo", unit: "month" },
  { size: 7 * day, short: "w", unit: "week" },
  { size: day, short: "d", unit: "day" },
  { size: hour, short: "h", unit: "hour" },
  { size: minute, short: "m", unit: "minute" },
]

// commitAge reads "3h" for the row and "3 hours ago" for its name. Future
// dates (clock skew) read as now.
export function commitAge(time: string, now: number): { short: string; long: string } {
  const elapsed = Math.max(0, now - Date.parse(time))
  const unit = ageUnits.find((entry) => elapsed >= entry.size)
  if (!unit) return { short: "now", long: "just now" }
  const count = Math.floor(elapsed / unit.size)
  return { short: `${count}${unit.short}`, long: new Intl.RelativeTimeFormat("en", { numeric: "always" }).format(-count, unit.unit) }
}

const absoluteFormat = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" })

// commitDate reads as the browser's locale, e.g. "Oct 21, 2026, 2:02 PM".
export function commitDate(time: string): string {
  return absoluteFormat.format(new Date(time))
}
