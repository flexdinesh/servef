export type ProjectKind = "folder" | "repo" | "worktree"

// Project mirrors projects.Project. Unavailable projects lie outside the served
// root: their id is empty and path is absolute.
export interface Project {
  id: string
  name: string
  path: string
  kind: ProjectKind
  group: string
  available: boolean
}

export interface ProjectsResponse {
  servedRoot: string
  rootName: string
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
    && typeof value.available === "boolean"
    && (value.available === (value.id !== ""))
}

function isDirectory(value: unknown): value is Directory {
  return isRecord(value)
    && typeof value.name === "string"
    && typeof value.path === "string"
    && typeof value.isProject === "boolean"
}

export function isProjectsResponse(value: unknown): value is ProjectsResponse {
  return isRecord(value)
    && typeof value.servedRoot === "string"
    && typeof value.rootName === "string"
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
