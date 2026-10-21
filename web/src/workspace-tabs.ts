// Open tabs are documents, working-tree diffs, or commit diffs of a
// project-relative path. Pure helpers; components and memory share them.

export type WorkspaceTab =
  | { kind: "document" | "diff"; path: string }
  // commit is the full hash whose change to path the tab shows.
  | { kind: "commit"; path: string; commit: string }

// StoredTab is a remembered tab: a plain path for a document, as earlier
// versions stored it, {diff: path}, or {commit: hash, path}.
export type StoredTab = string | { diff: string } | { commit: string; path: string }

export function documentTab(path: string): WorkspaceTab {
  return { kind: "document", path }
}

export function diffTab(path: string): WorkspaceTab {
  return { kind: "diff", path }
}

export function commitTab(commit: string, path: string): WorkspaceTab {
  return { kind: "commit", path, commit }
}

// isCommitHash accepts a full, lowercase SHA-1 commit hash.
export function isCommitHash(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{40}$/.test(value)
}

// shortHash abbreviates a commit hash as git log --oneline does.
export function shortHash(hash: string): string {
  return hash.slice(0, 7)
}

function tabCommit(tab: WorkspaceTab): string {
  return tab.kind === "commit" ? tab.commit : ""
}

export function sameTab(left: WorkspaceTab | null, right: WorkspaceTab | null): boolean {
  return left !== null && right !== null && left.kind === right.kind && left.path === right.path
    && tabCommit(left) === tabCommit(right)
}

// tabKey identifies a tab uniquely across kinds.
export function tabKey(tab: WorkspaceTab): string {
  return tab.kind === "commit" ? `commit:${tab.commit}:${tab.path}` : `${tab.kind}:${tab.path}`
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
// diffs the short hash.
export function tabName(tab: WorkspaceTab, deleted: boolean): TabName {
  if (tab.kind === "document") return { label: fileName(tab.path), title: tab.path }
  const side = tab.kind === "commit" ? shortHash(tab.commit) : deleted ? "Deleted" : "Working Tree"
  return { label: `${fileName(tab.path)} (${side})`, title: `${tab.path} (${side})` }
}

export function storedTab(tab: WorkspaceTab): StoredTab {
  if (tab.kind === "commit") return { commit: tab.commit, path: tab.path }
  return tab.kind === "diff" ? { diff: tab.path } : tab.path
}

// parseStoredTab accepts legacy path strings, {diff: path}, and
// {commit: hash, path} entries.
export function parseStoredTab(value: unknown): WorkspaceTab | null {
  if (typeof value === "string") return value ? documentTab(value) : null
  if (typeof value !== "object" || value === null || Array.isArray(value)) return null
  if ("commit" in value) {
    return isCommitHash(value.commit) && "path" in value && typeof value.path === "string" && value.path
      ? commitTab(value.commit, value.path)
      : null
  }
  if (!("diff" in value)) return null
  return typeof value.diff === "string" && value.diff ? diffTab(value.diff) : null
}
