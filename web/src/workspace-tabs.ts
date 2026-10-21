// Open tabs are documents or working-tree diffs of a project-relative path.
// Pure helpers; components and memory share them.

export interface WorkspaceTab {
  kind: "document" | "diff"
  path: string
}

// StoredTab is a remembered tab: a plain path for a document, as earlier
// versions stored it, or {diff: path}.
export type StoredTab = string | { diff: string }

export function documentTab(path: string): WorkspaceTab {
  return { kind: "document", path }
}

export function diffTab(path: string): WorkspaceTab {
  return { kind: "diff", path }
}

export function sameTab(left: WorkspaceTab | null, right: WorkspaceTab | null): boolean {
  return left !== null && right !== null && left.kind === right.kind && left.path === right.path
}

// tabKey identifies a tab uniquely across kinds.
export function tabKey(tab: WorkspaceTab): string {
  return `${tab.kind}:${tab.path}`
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

// tabName names a tab as VS Code does: diffs show the compared side.
export function tabName(tab: WorkspaceTab, deleted: boolean): TabName {
  if (tab.kind === "document") return { label: fileName(tab.path), title: tab.path }
  const side = deleted ? "Deleted" : "Working Tree"
  return { label: `${fileName(tab.path)} (${side})`, title: `${tab.path} (${side})` }
}

export function storedTab(tab: WorkspaceTab): StoredTab {
  return tab.kind === "diff" ? { diff: tab.path } : tab.path
}

// parseStoredTab accepts legacy path strings and {diff: path} entries.
export function parseStoredTab(value: unknown): WorkspaceTab | null {
  if (typeof value === "string") return value ? documentTab(value) : null
  if (typeof value !== "object" || value === null || Array.isArray(value) || !("diff" in value)) return null
  return typeof value.diff === "string" && value.diff ? diffTab(value.diff) : null
}
