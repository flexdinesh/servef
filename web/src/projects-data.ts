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

export interface Landing {
  project: string
  // listed is false for a folder discovery does not vouch for; check it exists.
  listed: boolean
  // pick opens the picker: nothing is remembered and there is a choice.
  pick: boolean
}

// landingProject picks the project for a URL without one: the last project
// remembered for this served root, or the root.
export function landingProject(projects: ProjectsResponse, memory: WorkspaceMemory): Landing {
  const last = rootMemory(memory, projects.servedRoot).last
  if (!last) {
    const choices = projects.projects.filter((project) => project.available).length
    return { project: rootProject, listed: true, pick: choices > 1 }
  }
  if (last === rootProject) return { project: rootProject, listed: true, pick: false }
  return { project: last, listed: projects.projects.some((project) => project.available && project.id === last), pick: false }
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
