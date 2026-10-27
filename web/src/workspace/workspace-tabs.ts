import {
  compareKey, compareName, compareSearch, parseStoredCompare, storedCompare,
  type Compare, type CompareContext, type StoredCompare,
} from "../features/source-control/compare.ts"
import { isCommitHash, shortHash } from "../lib/commit-hash.ts"
import { isRecord } from "../api/http.ts"

// Open tabs are documents of a project-relative path, read from the working
// tree or, with at, from a commit (a snapshot), or review tabs of one
// compare: uncommitted changes, the branch, a commit, or a commit range
// (decision 0035). Pure helpers; components and memory share them.

export type WorkspaceTab =
  | { kind: "document"; path: string; at?: string }
  | { kind: "review"; compare: Compare }

// StoredSnapshot is a remembered commit document.
export interface StoredSnapshot {
  path: string
  at: string
}

// StoredTab is a remembered tab: a plain path for a document, as earlier
// versions stored it, a stored snapshot, or a stored compare.
export type StoredTab = string | StoredSnapshot | StoredCompare

// documentTab is a working-tree document, or with at (a full commit hash)
// that commit's version of it.
export function documentTab(path: string, at = ""): WorkspaceTab {
  return at ? { kind: "document", path, at } : { kind: "document", path }
}

// routeAt is a route's snapshot commit: a full hash, and only with a path.
export function routeAt(path: unknown, at: unknown): string | undefined {
  return typeof path === "string" && isCommitHash(at) ? at : undefined
}

// documentKey identifies a document's modes, formats, and scroll: its path,
// or a snapshot's commit and path.
export function documentKey(path: string, at = ""): string {
  return at ? `${at}:${path}` : path
}

export function reviewTab(compare: Compare): WorkspaceTab {
  return { kind: "review", compare }
}

// tabKey identifies a tab uniquely across kinds.
export function tabKey(tab: WorkspaceTab): string {
  return tab.kind === "review" ? `review:${compareKey(tab.compare)}` : `document:${documentKey(tab.path, tab.at)}`
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
  // title exposes the full path or what the review compares.
  title: string
}

// tabName names a tab as VS Code does: documents by file name, reviews by
// what they compare.
export function tabName(tab: WorkspaceTab, context: CompareContext): TabName {
  if (tab.kind === "document") {
    return tab.at
      ? { label: `${fileName(tab.path)} @ ${shortHash(tab.at)}`, title: `${tab.path} at ${tab.at}` }
      : { label: fileName(tab.path), title: tab.path }
  }
  return compareName(tab.compare, context)
}

export function storedTab(tab: WorkspaceTab): StoredTab {
  if (tab.kind === "review") return storedCompare(tab.compare)
  return tab.at ? { path: tab.path, at: tab.at } : tab.path
}

// parseStoredTab accepts document paths, stored compares, and the per-file
// diff entries earlier versions stored, which reopen as their review.
export function parseStoredTab(value: unknown): WorkspaceTab | null {
  if (typeof value === "string") return value ? documentTab(value) : null
  // A snapshot has at; earlier per-file commit diffs stored commit instead.
  if (isRecord(value) && "at" in value) {
    return typeof value.path === "string" && value.path && isCommitHash(value.at) ? documentTab(value.path, value.at) : null
  }
  const compare = parseStoredCompare(value)
  return compare ? reviewTab(compare) : null
}

// tabSearch is the /view search that opens tab.
export function tabSearch(project: string, tab: WorkspaceTab) {
  if (tab.kind === "review") return { project, ...compareSearch(tab.compare) }
  return tab.at ? { project, path: tab.path, at: tab.at } : { project, path: tab.path }
}
