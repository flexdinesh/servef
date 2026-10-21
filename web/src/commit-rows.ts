import type { Commit, CommitFile, CommitFilesResponse } from "./commits-contract.ts"

// Pure row model for the Commits tree: commits, the expanded ones' details,
// and their changed files under compact folders (decision 0027).

// CommitFiles is one commit's file list as the view knows it.
export type CommitFiles =
  | { kind: "loading" }
  | { kind: "ready"; response: CommitFilesResponse }
  | { kind: "error"; message: string }

export type CommitRow =
  | { kind: "commit"; key: string; commit: Commit; expanded: boolean }
  // details holds the message, author, date, hash, and the file list's state.
  | { kind: "details"; key: string; commit: Commit; files: CommitFiles | undefined }
  // folder names a chain of single-child folders, as compact folders do.
  | { kind: "folder"; key: string; commit: Commit; path: string; name: string; depth: number; expanded: boolean }
  | { kind: "file"; key: string; commit: Commit; file: CommitFile; name: string; depth: number }

export function commitRowKey(hash: string): string {
  return `commit:${hash}`
}

export function fileRowKey(hash: string, path: string): string {
  return `file:${hash}:${path}`
}

// folderKey names a folder of one commit in the collapsed set.
export function folderKey(hash: string, path: string): string {
  return `${hash}:${path}`
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

function folderRows(commit: Commit, folder: Folder, prefix: string, depth: number, collapsed: ReadonlySet<string>, rows: CommitRow[]) {
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
    const expanded = !collapsed.has(folderKey(commit.hash, path))
    rows.push({ kind: "folder", key: `folder:${commit.hash}:${path}`, commit, path, name: label, depth, expanded })
    if (expanded) folderRows(commit, child, `${path}/`, depth + 1, collapsed, rows)
  }
  for (const file of [...folder.files].sort((left, right) => byName(fileName(left.path), fileName(right.path)))) {
    rows.push({ kind: "file", key: fileRowKey(commit.hash, file.path), commit, file, name: fileName(file.path), depth })
  }
}

// commitRows flattens the tree in display order. Folders open by default;
// collapsed holds folderKey entries the user closed.
export function commitRows(
  commits: readonly Commit[],
  expanded: ReadonlySet<string>,
  files: ReadonlyMap<string, CommitFiles>,
  collapsed: ReadonlySet<string>,
): CommitRow[] {
  const rows: CommitRow[] = []
  for (const commit of commits) {
    const open = expanded.has(commit.hash)
    rows.push({ kind: "commit", key: commitRowKey(commit.hash), commit, expanded: open })
    if (!open) continue
    const state = files.get(commit.hash)
    rows.push({ kind: "details", key: `details:${commit.hash}`, commit, files: state })
    if (state?.kind === "ready") folderRows(commit, folderTree(state.response.files), "", 2, collapsed, rows)
  }
  return rows
}

// parentRowKey is the row ArrowLeft moves to: a file's or folder's folder,
// else its commit.
export function parentRowKey(rows: readonly CommitRow[], index: number): string {
  const row = rows[index]
  if (!row || row.kind === "commit") return ""
  if (row.kind === "details") return commitRowKey(row.commit.hash)
  for (let at = index - 1; at >= 0; at--) {
    const candidate = rows[at]
    if (!candidate || candidate.kind === "commit") return commitRowKey(row.commit.hash)
    if (candidate.kind === "folder" && candidate.depth < row.depth) return candidate.key
  }
  return commitRowKey(row.commit.hash)
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
