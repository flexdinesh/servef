import { isDirectoriesResponse, isProjectsResponse, type DirectoriesResponse, type ProjectsResponse } from "./projects-contract.ts"
import { rootMemory, type WorkspaceMemory } from "./workspace-memory.ts"

export const rootProject = "."

export async function fetchProjects(signal?: AbortSignal): Promise<ProjectsResponse> {
  const response = await fetch("/api/projects", { headers: { Accept: "application/json" }, signal })
  const payload: unknown = await response.json()
  if (!response.ok || !isProjectsResponse(payload)) throw new Error("Could not list projects.")
  return payload
}

export async function fetchDirectories(path: string, signal?: AbortSignal): Promise<DirectoriesResponse> {
  const response = await fetch(`/api/directories?${new URLSearchParams({ path }).toString()}`, {
    headers: { Accept: "application/json" },
    signal,
  })
  const payload: unknown = await response.json()
  if (response.ok && isDirectoriesResponse(payload)) return payload
  const message = typeof payload === "object" && payload !== null && "error" in payload && typeof payload.error === "string"
    ? payload.error
    : "Could not read that folder."
  throw new Error(message)
}

// landingProject picks the project for a URL without one: the last project
// remembered for this served root, or the root. Listed tells whether discovery
// vouches for it; an unlisted folder must be checked before reopening.
export function landingProject(projects: ProjectsResponse, memory: WorkspaceMemory): { project: string; listed: boolean } {
  const last = rootMemory(memory, projects.servedRoot).last
  if (!last || last === rootProject) return { project: rootProject, listed: true }
  return { project: last, listed: projects.projects.some((project) => project.available && project.id === last) }
}
