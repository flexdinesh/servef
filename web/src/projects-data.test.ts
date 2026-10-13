import assert from "node:assert/strict"
import test from "node:test"

import type { ProjectsResponse } from "./projects-contract.ts"
import { landingProject } from "./projects-data.ts"
import { forgetProject, rememberProject } from "./workspace-memory.ts"

const projects: ProjectsResponse = {
  servedRoot: "/work",
  rootName: "work",
  projects: [
    { id: ".", name: "work", path: ".", kind: "folder", group: "", available: true },
    { id: "api", name: "api", path: "api", kind: "repo", group: "", available: true },
  ],
  warnings: [],
}

test("landing reopens the last project of the same served root", () => {
  assert.deepEqual(landingProject(projects, rememberProject({}, "/work", "api", [], "")), { project: "api", listed: true, pick: false })
  assert.deepEqual(landingProject(projects, rememberProject({}, "/work", ".", [], "")), { project: ".", listed: true, pick: false })
})

test("landing marks an unlisted last project for checking", () => {
  assert.deepEqual(landingProject(projects, rememberProject({}, "/work", "notes/2026", [], "")), { project: "notes/2026", listed: false, pick: false })
})

test("a forgotten last project no longer reopens", () => {
  const memory = forgetProject(rememberProject({}, "/work", "api", [], ""), "/work", "api")
  assert.deepEqual(landingProject(projects, memory), { project: ".", listed: true, pick: true })
})

test("a first landing with a choice of projects opens the picker", () => {
  assert.deepEqual(landingProject(projects, {}), { project: ".", listed: true, pick: true })
  assert.deepEqual(landingProject(projects, rememberProject({}, "/other", "api", [], "")), { project: ".", listed: true, pick: true })
  const rootOnly = { ...projects, projects: projects.projects.slice(0, 1) }
  assert.deepEqual(landingProject(rootOnly, {}), { project: ".", listed: true, pick: false })
})
