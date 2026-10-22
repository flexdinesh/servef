import type { Commit, CommitFile } from "../../commits-contract.ts"
import { branchTab, commitTab, rangeTab, type WorkspaceTab } from "../../workspace/workspace-tabs.ts"

// Pure row model for the Commits tree: a pinned compare row (decision 0030),
// commits, the expanded ones' details, and their changed files under compact
// folders (decision 0027).

// CommitFiles is one commit's, or one compare's, file list as the view
// knows it.
export type CommitFiles =
  | { kind: "loading" }
  | { kind: "ready"; response: { files: CommitFile[]; truncated: boolean } }
  | { kind: "error"; message: string }

// Compare is what the pinned row shows: the branch since its merge-base with
// base, or a selected commit range from (oldest) to to (newest).
export type Compare =
  | { kind: "branch"; base: string; ahead: number; mergeBase: string }
  | { kind: "range"; from: string; to: string; count: number }

// PinnedCompare is the pinned row's compare, open state, and file list.
export interface PinnedCompare {
  compare: Compare
  expanded: boolean
  files: CommitFiles | undefined
}

export type CommitRow =
  | { kind: "compare"; key: string; compare: Compare; expanded: boolean; files: CommitFiles | undefined }
  // compare-details describes the compared commits and the file list's state.
  | { kind: "compare-details"; key: string; compare: Compare; files: CommitFiles | undefined }
  | { kind: "commit"; key: string; commit: Commit; expanded: boolean }
  // details holds the message, author, date, hash, and the file list's state.
  | { kind: "details"; key: string; commit: Commit; files: CommitFiles | undefined }
  // folder names a chain of single-child folders, as compact folders do.
  // owner is the commit hash or compare owner whose files it holds; parent is
  // that commit's or compare's row key.
  | { kind: "folder"; key: string; owner: string; parent: string; path: string; name: string; depth: number; expanded: boolean }
  // tab is the diff tab the file opens.
  | { kind: "file"; key: string; owner: string; parent: string; tab: WorkspaceTab; file: CommitFile; name: string; depth: number }

// compareRowKey is the pinned row's key; Source Control's expanded set holds
// it while the row is open.
export const compareRowKey = "compare"

export function commitRowKey(hash: string): string {
  return `commit:${hash}`
}

// compareOwner names a compare's files apart from every commit's.
export function compareOwner(compare: Compare): string {
  return compare.kind === "branch" ? "branch" : `range:${compare.from}:${compare.to}`
}

export function fileRowKey(owner: string, path: string): string {
  return `file:${owner}:${path}`
}

// tabRowKey is the file row an open diff tab selects, "" for other tabs.
export function tabRowKey(tab: WorkspaceTab): string {
  if (tab.kind === "commit") return fileRowKey(tab.commit, tab.path)
  if (tab.kind === "branch") return fileRowKey("branch", tab.path)
  if (tab.kind === "range") return fileRowKey(`range:${tab.from}:${tab.to}`, tab.path)
  return ""
}

// folderKey names a folder of one commit or compare in the collapsed set.
export function folderKey(owner: string, path: string): string {
  return `${owner}:${path}`
}

interface Folder {
  folders: Map<string, Folder>
  files: CommitFile[]
}

function newFolder(): Folder {
  return { folders: new Map(), files: [] }
}

function folderTree(files: readonly CommitFile[]): Folder {
  const root = newFolder()
  for (const file of files) {
    const segments = file.path.split("/")
    let folder = root
    for (const segment of segments.slice(0, -1)) {
      let child = folder.folders.get(segment)
      if (!child) {
        child = newFolder()
        folder.folders.set(segment, child)
      }
      folder = child
    }
    folder.files.push(file)
  }
  return root
}

function byName(left: string, right: string): number {
  return left.localeCompare(right, undefined, { numeric: true, sensitivity: "base" }) || (left < right ? -1 : left > right ? 1 : 0)
}

function fileName(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1)
}

// FileOwner is whose files a folder holds and how its files open.
interface FileOwner {
  owner: string
  parent: string
  tab(path: string): WorkspaceTab
}

function folderRows(owner: FileOwner, folder: Folder, prefix: string, depth: number, collapsed: ReadonlySet<string>, rows: CommitRow[]) {
  for (const name of [...folder.folders.keys()].sort(byName)) {
    let child = folder.folders.get(name)
    let label = name
    let path = `${prefix}${name}`
    // A folder whose only entry is a folder joins it on one row.
    while (child && child.files.length === 0 && child.folders.size === 1) {
      const [nextName, next] = [...child.folders][0] ?? []
      if (nextName === undefined || !next) break
      label = `${label}/${nextName}`
      path = `${path}/${nextName}`
      child = next
    }
    if (!child) continue
    const expanded = !collapsed.has(folderKey(owner.owner, path))
    rows.push({ kind: "folder", key: `folder:${owner.owner}:${path}`, owner: owner.owner, parent: owner.parent, path, name: label, depth, expanded })
    if (expanded) folderRows(owner, child, `${path}/`, depth + 1, collapsed, rows)
  }
  for (const file of [...folder.files].sort((left, right) => byName(fileName(left.path), fileName(right.path)))) {
    rows.push({
      kind: "file", key: fileRowKey(owner.owner, file.path), owner: owner.owner, parent: owner.parent,
      tab: owner.tab(file.path), file, name: fileName(file.path), depth,
    })
  }
}

function compareFileOwner(compare: Compare): FileOwner {
  return {
    owner: compareOwner(compare),
    parent: compareRowKey,
    tab: (path) => compare.kind === "branch" ? branchTab(path) : rangeTab(compare.from, compare.to, path),
  }
}

// commitRows flattens the tree in display order: the pinned compare row
// first, when there is one, then the commits. Folders open by default;
// collapsed holds folderKey entries the user closed.
export function commitRows(
  commits: readonly Commit[],
  expanded: ReadonlySet<string>,
  files: ReadonlyMap<string, CommitFiles>,
  collapsed: ReadonlySet<string>,
  pinned: PinnedCompare | null = null,
): CommitRow[] {
  const rows: CommitRow[] = []
  if (pinned) {
    const { compare, expanded: open, files: state } = pinned
    rows.push({ kind: "compare", key: compareRowKey, compare, expanded: open, files: state })
    if (open) {
      rows.push({ kind: "compare-details", key: "compare-details", compare, files: state })
      if (state?.kind === "ready") folderRows(compareFileOwner(compare), folderTree(state.response.files), "", 2, collapsed, rows)
    }
  }
  for (const commit of commits) {
    const open = expanded.has(commit.hash)
    rows.push({ kind: "commit", key: commitRowKey(commit.hash), commit, expanded: open })
    if (!open) continue
    const state = files.get(commit.hash)
    rows.push({ kind: "details", key: `details:${commit.hash}`, commit, files: state })
    const owner: FileOwner = { owner: commit.hash, parent: commitRowKey(commit.hash), tab: (path) => commitTab(commit.hash, path) }
    if (state?.kind === "ready") folderRows(owner, folderTree(state.response.files), "", 2, collapsed, rows)
  }
  return rows
}

// CommitSelection is a contiguous run of listed commits: from is the oldest
// (lowest row), to the newest (highest row).
export interface CommitSelection {
  from: string
  to: string
}

// rangeSelection spans the listed commits between anchor and focus, in
// either direction. It is null unless both are listed and differ: one commit
// is the commit itself.
export function rangeSelection(commits: readonly Commit[], anchor: string, focus: string): CommitSelection | null {
  const start = commits.findIndex((commit) => commit.hash === anchor)
  const end = commits.findIndex((commit) => commit.hash === focus)
  if (start < 0 || end < 0 || start === end) return null
  const newest = commits[Math.min(start, end)]
  const oldest = commits[Math.max(start, end)]
  return newest && oldest ? { from: oldest.hash, to: newest.hash } : null
}

// selectedCommits lists a selection's hashes, newest first, or null when
// either end is no longer listed.
export function selectedCommits(commits: readonly Commit[], selection: CommitSelection | null): string[] | null {
  if (!selection) return null
  const newest = commits.findIndex((commit) => commit.hash === selection.to)
  const oldest = commits.findIndex((commit) => commit.hash === selection.from)
  if (newest < 0 || oldest <= newest) return null
  return commits.slice(newest, oldest + 1).map((commit) => commit.hash)
}

// parentRowKey is the row ArrowLeft moves to: a file's or folder's folder,
// else its commit or the compare row.
export function parentRowKey(rows: readonly CommitRow[], index: number): string {
  const row = rows[index]
  if (!row || row.kind === "commit" || row.kind === "compare") return ""
  if (row.kind === "details") return commitRowKey(row.commit.hash)
  if (row.kind === "compare-details") return compareRowKey
  for (let at = index - 1; at >= 0; at--) {
    const candidate = rows[at]
    if (!candidate || candidate.kind === "commit" || candidate.kind === "compare") return row.parent
    if (candidate.kind === "folder" && candidate.depth < row.depth) return candidate.key
  }
  return row.parent
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
