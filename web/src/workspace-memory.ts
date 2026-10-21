// Browser memory of project workspaces. Projects are absolute paths, so each
// project's tabs and folder expansion are shared by sessions that serve it; the last
// project is kept per set of served roots. Pure functions return new values;
// readMemory/writeMemory are the only storage access and never throw.

import { includesTab, parseStoredTab, sameTab, storedTab, type StoredTab, type WorkspaceTab } from "./workspace-tabs.ts"

export const MEMORY_STORAGE_KEY = "servef-workspaces-v2"

const maxProjects = 100
const maxSessions = 20

export interface ProjectMemory {
  // Tabs are stored as plain paths (documents) or {diff: path}; "" is no active tab.
  tabs: StoredTab[]
  active: StoredTab
  // Absent uses the server's initial expansion; [] remembers all collapsed.
  expanded?: string[]
}

export interface WorkspaceMemory {
  // last maps a rootsKey to the project last open in that session shape.
  last: Record<string, string>
  projects: Record<string, ProjectMemory>
}

export const emptyMemory: WorkspaceMemory = { last: {}, projects: {} }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((entry: unknown) => typeof entry === "string") : []
}

function storedTabs(value: unknown): StoredTab[] {
  if (!Array.isArray(value)) return []
  return value.flatMap((entry: unknown) => {
    const tab = parseStoredTab(entry)
    return tab ? [storedTab(tab)] : []
  })
}

function parseProject(value: unknown): ProjectMemory | null {
  if (!isRecord(value)) return null
  const active = parseStoredTab(value.active)
  return {
    tabs: storedTabs(value.tabs),
    active: active ? storedTab(active) : "",
    ...(Array.isArray(value.expanded) ? { expanded: stringArray(value.expanded) } : {}),
  }
}

function entries(value: unknown): [string, unknown][] {
  return isRecord(value) ? Object.entries(value).filter(([key]) => key !== "__proto__") : []
}

// parseMemory keeps every well-formed entry of untrusted stored data.
export function parseMemory(value: unknown): WorkspaceMemory {
  if (!isRecord(value)) return emptyMemory
  const last: Record<string, string> = {}
  for (const [roots, project] of entries(value.last)) {
    if (typeof project === "string") last[roots] = project
  }
  const projects: Record<string, ProjectMemory> = {}
  for (const [id, entry] of entries(value.projects)) {
    const project = parseProject(entry)
    if (project) projects[id] = project
  }
  return { last, projects }
}

// rootsKey identifies a session by its served roots.
export function rootsKey(roots: readonly string[]): string {
  return roots.join("\n")
}

export function lastProject(memory: WorkspaceMemory, roots: readonly string[]): string {
  const key = rootsKey(roots)
  return Object.hasOwn(memory.last, key) ? memory.last[key] ?? "" : ""
}

export function projectMemory(memory: WorkspaceMemory, project: string): ProjectMemory | undefined {
  return Object.hasOwn(memory.projects, project) ? memory.projects[project] : undefined
}

// Re-inserting moves an entry to the end, so object order is least recent first.
function withEntry<T>(record: Record<string, T>, key: string, value: T, max: number): Record<string, T> {
  const { [key]: _previous, ...rest } = record
  return { ...Object.fromEntries(Object.entries(rest).slice(-(max - 1))), [key]: value }
}

export function rememberProject(memory: WorkspaceMemory, roots: readonly string[], project: string, tabs: readonly WorkspaceTab[], active: WorkspaceTab | null): WorkspaceMemory {
  return {
    last: withEntry(memory.last, rootsKey(roots), project, maxSessions),
    projects: withEntry(memory.projects, project, {
      ...projectMemory(memory, project),
      tabs: tabs.map(storedTab),
      active: active && includesTab(tabs, active) ? storedTab(active) : "",
    }, maxProjects),
  }
}

// Folder and tab updates preserve one another, including when saved by
// different components during the same render.
export function rememberExpanded(memory: WorkspaceMemory, project: string, expanded: readonly string[]): WorkspaceMemory {
  return {
    ...memory,
    projects: withEntry(memory.projects, project, {
      tabs: [],
      active: "",
      ...projectMemory(memory, project),
      expanded: [...new Set(expanded)],
    }, maxProjects),
  }
}

// forgetProject drops a project's workspace; a forgotten last project
// no longer reopens on landing.
export function forgetProject(memory: WorkspaceMemory, project: string): WorkspaceMemory {
  const { [project]: _forgotten, ...projects } = memory.projects
  const last = Object.fromEntries(Object.entries(memory.last).filter(([, id]) => id !== project))
  return { last, projects }
}

// rememberedProjects lists remembered project IDs, most recent first.
export function rememberedProjects(memory: WorkspaceMemory): string[] {
  return Object.keys(memory.projects).reverse()
}

export interface RestoredTabs {
  tabs: WorkspaceTab[]
  active: WorkspaceTab | null
}

// restoreTabs keeps remembered documents that still exist in the fresh tree.
// Diff tabs restore regardless: a deleted file has a diff but no tree entry.
export function restoreTabs(memory: ProjectMemory | undefined, files: ReadonlySet<string>): RestoredTabs {
  const tabs: WorkspaceTab[] = []
  for (const entry of memory?.tabs ?? []) {
    const tab = parseStoredTab(entry)
    if (tab && (tab.kind === "diff" || files.has(tab.path)) && !includesTab(tabs, tab)) tabs.push(tab)
  }
  const remembered = parseStoredTab(memory?.active)
  const active = tabs.find((tab) => sameTab(tab, remembered)) ?? null
  return { tabs, active }
}

// Only restore directories still present in the fresh project tree. Keep an
// absent preference distinct from the explicit all-collapsed state.
export function restoreExpanded(memory: ProjectMemory | undefined, directories: ReadonlySet<string>): string[] | undefined {
  return memory?.expanded?.filter((path, index, all) => directories.has(path) && all.indexOf(path) === index)
}

export function readMemory(): WorkspaceMemory {
  try {
    const raw = window.localStorage.getItem(MEMORY_STORAGE_KEY)
    return raw ? parseMemory(JSON.parse(raw)) : emptyMemory
  } catch {
    return emptyMemory
  }
}

export function writeMemory(memory: WorkspaceMemory) {
  try {
    window.localStorage.setItem(MEMORY_STORAGE_KEY, JSON.stringify(memory))
  } catch {
    // Storage is a convenience; a blocked or full store keeps the session usable.
  }
}

// updateMemory applies a change to the latest stored memory, so concurrent
// browser tabs only overwrite the entries they changed.
export function updateMemory(change: (memory: WorkspaceMemory) => WorkspaceMemory) {
  writeMemory(change(readMemory()))
}
