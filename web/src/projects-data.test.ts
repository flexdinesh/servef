import assert from "node:assert/strict"
import test from "node:test"

import type { ProjectsResponse } from "./projects-contract.ts"
import { landingProject } from "./projects-data.ts"
import { emptyMemory, forgetProject, rememberProject } from "./workspace/workspace-memory.ts"

const roots = ["/work", "/home"]
const projects: ProjectsResponse = {
  roots: [{ path: "/work", name: "work", isRepo: false }, { path: "/home", name: "home", isRepo: false }],
  projects: [
    { id: "/work", name: "work", path: "/work", kind: "folder", group: "", root: "/work", branch: "", repository: "", available: true },
    { id: "/work/api", name: "api", path: "/work/api", kind: "repo", group: "", root: "/work", branch: "main", repository: "api", available: true },
    { id: "/home", name: "home", path: "/home", kind: "folder", group: "", root: "/home", branch: "", repository: "", available: true },
  ],
  warnings: [],
}

test("landing reopens the last project of the same served roots", () => {
  assert.deepEqual(landingProject(projects, rememberProject(emptyMemory, roots, "/work/api", [], null)),
    { project: "/work/api", listed: true, pick: false, fallback: "/work" })
  assert.deepEqual(landingProject(projects, rememberProject(emptyMemory, roots, "/home", [], null)),
    { project: "/home", listed: true, pick: false, fallback: "/work" })
})

test("landing marks an unlisted last project for checking", () => {
  assert.deepEqual(landingProject(projects, rememberProject(emptyMemory, roots, "/work/notes/2026", [], null)),
    { project: "/work/notes/2026", listed: false, pick: false, fallback: "/work" })
})

test("a forgotten last project no longer reopens", () => {
  const memory = forgetProject(rememberProject(emptyMemory, roots, "/work/api", [], null), "/work/api")
  assert.deepEqual(landingProject(projects, memory), { project: "/work", listed: true, pick: true, fallback: "/work" })
})

test("a first landing with a choice of projects opens the picker on the first root", () => {
  assert.deepEqual(landingProject(projects, emptyMemory), { project: "/work", listed: true, pick: true, fallback: "/work" })
  assert.deepEqual(landingProject(projects, rememberProject(emptyMemory, ["/work"], "/work/api", [], null)),
    { project: "/work", listed: true, pick: true, fallback: "/work" })
  const rootOnly: ProjectsResponse = { ...projects, roots: projects.roots.slice(0, 1), projects: projects.projects.slice(0, 1) }
  assert.deepEqual(landingProject(rootOnly, emptyMemory), { project: "/work", listed: true, pick: false, fallback: "/work" })
})
