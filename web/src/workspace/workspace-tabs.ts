import {
  compareKey, compareName, compareSearch, parseStoredCompare, storedCompare,
  type Compare, type CompareContext, type StoredCompare,
} from "../features/source-control/compare.ts"

// Open tabs are documents of a project-relative path, or review tabs of one
// compare: uncommitted changes, the branch, a commit, or a commit range
// (decision 0035). Pure helpers; components and memory share them.

export type WorkspaceTab =
  | { kind: "document"; path: string }
  | { kind: "review"; compare: Compare }

// StoredTab is a remembered tab: a plain path for a document, as earlier
// versions stored it, or a stored compare.
export type StoredTab = string | StoredCompare

export function documentTab(path: string): WorkspaceTab {
  return { kind: "document", path }
}

export function reviewTab(compare: Compare): WorkspaceTab {
  return { kind: "review", compare }
}

// tabKey identifies a tab uniquely across kinds.
export function tabKey(tab: WorkspaceTab): string {
  return tab.kind === "review" ? `review:${compareKey(tab.compare)}` : `document:${tab.path}`
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
  if (tab.kind === "document") return { label: fileName(tab.path), title: tab.path }
  return compareName(tab.compare, context)
}

export function storedTab(tab: WorkspaceTab): StoredTab {
  return tab.kind === "review" ? storedCompare(tab.compare) : tab.path
}

// parseStoredTab accepts document paths, stored compares, and the per-file
// diff entries earlier versions stored, which reopen as their review.
export function parseStoredTab(value: unknown): WorkspaceTab | null {
  if (typeof value === "string") return value ? documentTab(value) : null
  const compare = parseStoredCompare(value)
  return compare ? reviewTab(compare) : null
}

// tabSearch is the /view search that opens tab.
export function tabSearch(project: string, tab: WorkspaceTab) {
  return tab.kind === "review" ? { project, ...compareSearch(tab.compare) } : { project, path: tab.path }
}
