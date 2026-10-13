// Browser memory of project workspaces, keyed by the served root so separate
// servef sessions on one origin stay apart. Pure functions return new values;
// readMemory/writeMemory are the only storage access and never throw.

export const MEMORY_STORAGE_KEY = "servef-workspaces"

const maxProjects = 50
const maxRoots = 20

export interface ProjectMemory {
  tabs: string[]
  active: string
}

export interface RootMemory {
  last: string
  projects: Record<string, ProjectMemory>
}

export type WorkspaceMemory = Record<string, RootMemory>

const emptyRoot: RootMemory = { last: "", projects: {} }

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

function parseRoot(value: unknown): RootMemory | null {
  if (!isRecord(value)) return null
  const projects: Record<string, ProjectMemory> = {}
  if (isRecord(value.projects)) {
    for (const [id, entry] of Object.entries(value.projects)) {
      if (id === "__proto__") continue
      const project = parseProject(entry)
      if (project) projects[id] = project
    }
  }
  return { last: typeof value.last === "string" ? value.last : "", projects }
}

// parseMemory keeps every well-formed entry of untrusted stored data.
export function parseMemory(value: unknown): WorkspaceMemory {
  const memory: WorkspaceMemory = {}
  if (!isRecord(value)) return memory
  for (const [root, entry] of Object.entries(value)) {
    if (root === "__proto__") continue
    const parsed = parseRoot(entry)
    if (parsed) memory[root] = parsed
  }
  return memory
}

export function rootMemory(memory: WorkspaceMemory, root: string): RootMemory {
  return Object.hasOwn(memory, root) ? memory[root] ?? emptyRoot : emptyRoot
}

export function projectMemory(memory: WorkspaceMemory, root: string, project: string): ProjectMemory | undefined {
  const projects = rootMemory(memory, root).projects
  return Object.hasOwn(projects, project) ? projects[project] : undefined
}

// Re-inserting moves an entry to the end, so object order is least recent first.
function withRoot(memory: WorkspaceMemory, root: string, entry: RootMemory): WorkspaceMemory {
  const { [root]: _previous, ...rest } = memory
  const roots = Object.entries(rest).slice(-(maxRoots - 1))
  return { ...Object.fromEntries(roots), [root]: entry }
}

export function rememberProject(memory: WorkspaceMemory, root: string, project: string, tabs: readonly string[], active: string): WorkspaceMemory {
  const current = rootMemory(memory, root)
  const { [project]: _previous, ...rest } = current.projects
  const kept = Object.entries(rest).slice(-(maxProjects - 1))
  return withRoot(memory, root, {
    ...current,
    last: project,
    projects: { ...Object.fromEntries(kept), [project]: { tabs: [...tabs], active: tabs.includes(active) ? active : "" } },
  })
}

// forgetProject drops a project's remembered tabs; a forgotten last project
// no longer reopens on landing.
export function forgetProject(memory: WorkspaceMemory, root: string, project: string): WorkspaceMemory {
  const current = rootMemory(memory, root)
  const { [project]: _forgotten, ...projects } = current.projects
  return withRoot(memory, root, { last: current.last === project ? "" : current.last, projects })
}

// rememberedProjects lists remembered project IDs, most recent first.
export function rememberedProjects(memory: WorkspaceMemory, root: string): string[] {
  return Object.keys(rootMemory(memory, root).projects).reverse()
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
    return raw ? parseMemory(JSON.parse(raw)) : {}
  } catch {
    return {}
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
