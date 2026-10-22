// Open tabs are documents, working-tree diffs, commit diffs, branch diffs, or
// commit-range diffs of a project-relative path. Pure helpers; components
// and memory share them.

export type WorkspaceTab =
  | { kind: "document" | "diff"; path: string }
  // commit is the full hash whose change to path the tab shows.
  | { kind: "commit"; path: string; commit: string }
  // branch shows path's change from the merge-base with the base branch to
  // HEAD, re-read as HEAD moves (decision 0030).
  | { kind: "branch"; path: string }
  // range shows path's net change from the first parent of from (the oldest
  // selected commit) to to (the newest).
  | { kind: "range"; path: string; from: string; to: string }

// StoredTab is a remembered tab: a plain path for a document, as earlier
// versions stored it, {diff: path}, {commit: hash, path}, {branch: path}, or
// {from, to, path}.
export type StoredTab = string | { diff: string } | { commit: string; path: string } | { branch: string }
  | { from: string; to: string; path: string }

export function documentTab(path: string): WorkspaceTab {
  return { kind: "document", path }
}

export function diffTab(path: string): WorkspaceTab {
  return { kind: "diff", path }
}

export function commitTab(commit: string, path: string): WorkspaceTab {
  return { kind: "commit", path, commit }
}

export function branchTab(path: string): WorkspaceTab {
  return { kind: "branch", path }
}

export function rangeTab(from: string, to: string, path: string): WorkspaceTab {
  return { kind: "range", path, from, to }
}

// isCommitHash accepts a full, lowercase SHA-1 commit hash.
export function isCommitHash(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{40}$/.test(value)
}

// shortHash abbreviates a commit hash as git log --oneline does.
export function shortHash(hash: string): string {
  return hash.slice(0, 7)
}

// tabKey identifies a tab uniquely across kinds.
export function tabKey(tab: WorkspaceTab): string {
  if (tab.kind === "commit") return `commit:${tab.commit}:${tab.path}`
  if (tab.kind === "range") return `range:${tab.from}:${tab.to}:${tab.path}`
  return `${tab.kind}:${tab.path}`
}

export function sameTab(left: WorkspaceTab | null, right: WorkspaceTab | null): boolean {
  return left !== null && right !== null && tabKey(left) === tabKey(right)
}

export function includesTab(tabs: readonly WorkspaceTab[], tab: WorkspaceTab): boolean {
  return tabs.some((entry) => sameTab(entry, tab))
}

export function fileName(path: string): string {
  return path.split("/").at(-1) || path
}

export interface TabName {
  label: string
  // title exposes the full path.
  title: string
}

// tabName names a tab as VS Code does: diffs show the compared side, commit
// diffs the short hash, branch diffs the base (once known), and range diffs
// their oldest and newest commits.
export function tabName(tab: WorkspaceTab, deleted: boolean, base = ""): TabName {
  if (tab.kind === "document") return { label: fileName(tab.path), title: tab.path }
  if (tab.kind === "range") {
    const span = `${shortHash(tab.from)}..${shortHash(tab.to)}`
    return { label: `${fileName(tab.path)} (${span})`, title: `${tab.path} (changes in commits ${shortHash(tab.from)} through ${shortHash(tab.to)})` }
  }
  const side = tab.kind === "commit" ? shortHash(tab.commit)
    : tab.kind === "branch" ? base ? `vs ${base}` : "Branch"
    : deleted ? "Deleted" : "Working Tree"
  return { label: `${fileName(tab.path)} (${side})`, title: `${tab.path} (${side})` }
}

export function storedTab(tab: WorkspaceTab): StoredTab {
  if (tab.kind === "commit") return { commit: tab.commit, path: tab.path }
  if (tab.kind === "branch") return { branch: tab.path }
  if (tab.kind === "range") return { from: tab.from, to: tab.to, path: tab.path }
  return tab.kind === "diff" ? { diff: tab.path } : tab.path
}

function storedPath(value: object): string {
  return "path" in value && typeof value.path === "string" ? value.path : ""
}

// parseStoredTab accepts legacy path strings, {diff: path},
// {commit: hash, path}, {branch: path}, and {from, to, path} entries.
export function parseStoredTab(value: unknown): WorkspaceTab | null {
  if (typeof value === "string") return value ? documentTab(value) : null
  if (typeof value !== "object" || value === null || Array.isArray(value)) return null
  if ("commit" in value) {
    const path = storedPath(value)
    return isCommitHash(value.commit) && path ? commitTab(value.commit, path) : null
  }
  if ("from" in value || "to" in value) {
    const path = storedPath(value)
    return "from" in value && "to" in value && isCommitHash(value.from) && isCommitHash(value.to) && path
      ? rangeTab(value.from, value.to, path)
      : null
  }
  if ("branch" in value) return typeof value.branch === "string" && value.branch ? branchTab(value.branch) : null
  if (!("diff" in value)) return null
  return typeof value.diff === "string" && value.diff ? diffTab(value.diff) : null
}

// RouteSearch is the part of a /view search that selects a diff tab.
export interface RouteSearch {
  path?: unknown
  diff?: unknown
  commit?: unknown
  branch?: unknown
  from?: unknown
  to?: unknown
}

// routeDiffTab is the diff tab a route opens: ?diff=1, ?commit=<hash>,
// ?branch=1, or ?from=<hash>&to=<hash> with a path, else none.
export function routeDiffTab(search: RouteSearch): WorkspaceTab | null {
  const path = typeof search.path === "string" ? search.path : ""
  if (!path) return null
  if (isCommitHash(search.commit)) return commitTab(search.commit, path)
  if (isCommitHash(search.from) && isCommitHash(search.to)) return rangeTab(search.from, search.to, path)
  if (search.branch === 1) return branchTab(path)
  return search.diff === 1 ? diffTab(path) : null
}
