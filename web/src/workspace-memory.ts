// Browser memory of project workspaces. Projects are absolute paths, so each
// project's tabs are shared by every servef session that serves it; the last
// project is kept per set of served roots. Pure functions return new values;
// readMemory/writeMemory are the only storage access and never throw.

export const MEMORY_STORAGE_KEY = "servef-workspaces-v2"

const maxProjects = 100
const maxSessions = 20

export interface ProjectMemory {
  tabs: string[]
  active: string
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

function parseProject(value: unknown): ProjectMemory | null {
  if (!isRecord(value)) return null
  return { tabs: stringArray(value.tabs), active: typeof value.active === "string" ? value.active : "" }
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

export function rememberProject(memory: WorkspaceMemory, roots: readonly string[], project: string, tabs: readonly string[], active: string): WorkspaceMemory {
  return {
    last: withEntry(memory.last, rootsKey(roots), project, maxSessions),
    projects: withEntry(memory.projects, project, { tabs: [...tabs], active: tabs.includes(active) ? active : "" }, maxProjects),
  }
}

// forgetProject drops a project's remembered tabs; a forgotten last project
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

// restoreTabs keeps remembered tabs that still exist in the fresh tree.
export function restoreTabs(memory: ProjectMemory | undefined, files: ReadonlySet<string>): ProjectMemory {
  const tabs = (memory?.tabs ?? []).filter((tab, index, all) => files.has(tab) && all.indexOf(tab) === index)
  const active = memory && tabs.includes(memory.active) ? memory.active : ""
  return { tabs, active }
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
