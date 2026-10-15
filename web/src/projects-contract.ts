export type ProjectKind = "folder" | "repo" | "worktree"

// Root mirrors projects.Root, a served root directory.
export interface Root {
  path: string
  name: string
  isRepo: boolean
}

// Project mirrors projects.Project. The id is the absolute path, or empty for
// a project that cannot be opened. Members of one repository share group, the
// path of its main worktree; root is the served root it, or its repository,
// lives in.
export interface Project {
  id: string
  name: string
  path: string
  kind: ProjectKind
  group: string
  root: string
  branch: string
  available: boolean
}

export interface ProjectsResponse {
  roots: Root[]
  projects: Project[]
  warnings: string[]
}

export interface Directory {
  name: string
  path: string
  isProject: boolean
}

export interface DirectoriesResponse {
  path: string
  directories: Directory[]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

function isProject(value: unknown): value is Project {
  return isRecord(value)
    && typeof value.id === "string"
    && typeof value.name === "string"
    && typeof value.path === "string"
    && (value.kind === "folder" || value.kind === "repo" || value.kind === "worktree")
    && typeof value.group === "string"
    && typeof value.root === "string"
    && typeof value.branch === "string"
    && typeof value.available === "boolean"
    && (value.available === (value.id !== ""))
}

function isDirectory(value: unknown): value is Directory {
  return isRecord(value)
    && typeof value.name === "string"
    && typeof value.path === "string"
    && typeof value.isProject === "boolean"
}

function isRoot(value: unknown): value is Root {
  return isRecord(value)
    && typeof value.path === "string"
    && typeof value.name === "string"
    && typeof value.isRepo === "boolean"
}

export function isProjectsResponse(value: unknown): value is ProjectsResponse {
  return isRecord(value)
    && Array.isArray(value.roots)
    && value.roots.every(isRoot)
    && Array.isArray(value.projects)
    && value.projects.every(isProject)
    && Array.isArray(value.warnings)
    && value.warnings.every((warning: unknown) => typeof warning === "string")
}

export function isDirectoriesResponse(value: unknown): value is DirectoriesResponse {
  return isRecord(value)
    && typeof value.path === "string"
    && Array.isArray(value.directories)
    && value.directories.every(isDirectory)
}
