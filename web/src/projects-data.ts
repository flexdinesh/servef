import { failureReason, fetchJSON } from "./http.ts"
import { isDirectoriesResponse, isProjectsResponse, type DirectoriesResponse, type ProjectsResponse } from "./projects-contract.ts"
import { lastProject, type WorkspaceMemory } from "./workspace/workspace-memory.ts"

export function fetchProjects(signal?: AbortSignal): Promise<ProjectsResponse> {
  return fetchJSON("/api/projects", { signal }, isProjectsResponse, (failure) => `Couldn't list projects: ${failureReason(failure)}.`)
}

// Server reasons for folders are full sentences.
export function fetchDirectories(path: string, signal?: AbortSignal): Promise<DirectoriesResponse> {
  return fetchJSON(`/api/directories?${new URLSearchParams({ path }).toString()}`, { signal }, isDirectoriesResponse,
    (failure) => failure.kind === "rejected" ? failure.error : "Could not read that folder.")
}

export interface Landing {
  project: string
  // listed is false for a folder discovery does not vouch for; check it exists.
  listed: boolean
  // pick opens the picker: nothing is remembered and there is a choice.
  pick: boolean
  // fallback is the first served root, opened when project is gone.
  fallback: string
}

// defaultProject is the first served root, always listed as a project.
export function defaultProject(projects: ProjectsResponse): string {
  return projects.roots[0]?.path ?? ""
}

// landingProject picks the project for a URL without one: the last project
// remembered for these served roots, or the first root.
export function landingProject(projects: ProjectsResponse, memory: WorkspaceMemory): Landing {
  const fallback = defaultProject(projects)
  const last = lastProject(memory, projects.roots.map((root) => root.path))
  if (!last) {
    const choices = projects.projects.filter((project) => project.available).length
    return { project: fallback, listed: true, pick: choices > 1, fallback }
  }
  return { project: last, listed: projects.projects.some((project) => project.available && project.id === last), pick: false, fallback }
}

let landingPick = false

// requestLandingPick asks the next mounted picker to open once.
export function requestLandingPick() {
  landingPick = true
}

export function takeLandingPick(): boolean {
  const pick = landingPick
  landingPick = false
  return pick
}
