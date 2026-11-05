import assert from "node:assert/strict"
import test from "node:test"

import type { ProjectsResponse } from "@servef/contracts"
import { listsProjects, projectLines, startEntries, welcomeSections, type ProjectFacts } from "./welcome-model.ts"

const response: ProjectsResponse = {
  sessionKey: "/work",
  presence: false,
  roots: [{ path: "/work", name: "work", isRepo: false }],
  projects: [
    { id: "/work", name: "work", path: "/work", kind: "folder", group: "", root: "/work", branch: "", repository: "", available: true },
    { id: "/work/app", name: "app", path: "/work/app", kind: "repo", group: "/work/app", root: "/work", branch: "main", repository: "app", available: true },
    { id: "/work/app-fix", name: "app-fix", path: "/work/app-fix", kind: "worktree", group: "/work/app", root: "/work", branch: "fix", repository: "app", available: true },
    { id: "/work/notes", name: "notes", path: "/work/notes", kind: "repo", group: "/work/notes", root: "/work", branch: "main", repository: "notes", available: true },
  ],
  warnings: [],
}

test("Open folder… starts a session only where folders can be opened", () => {
  assert.ok(startEntries(false, true).some((entry) => entry.action === "open-folder"))
  assert.ok(!startEntries(true, false).some((entry) => entry.action === "open-folder"))
})

test("project lines link available counts and omit unread counts", () => {
  const repo: ProjectFacts = { repository: true, branch: "main", changes: 4, walkthroughs: 2, unseen: 0 }
  assert.equal(projectLines(repo).find((line) => line.key === "changes")?.action, "review-uncommitted")
  assert.equal(projectLines(repo).find((line) => line.key === "walkthroughs")?.action, "walkthroughs")
  assert.match(projectLines({ ...repo, unseen: 1 }).find((line) => line.key === "walkthroughs")?.text ?? "", /^1\b/)
  assert.equal(projectLines({ ...repo, changes: 0, walkthroughs: 0 }).find((line) => line.key === "changes")?.action, undefined)
  assert.ok(!projectLines({ ...repo, changes: 0, walkthroughs: 0 }).some((line) => line.key === "walkthroughs"))
  // Unread counts say nothing rather than zero.
  assert.deepEqual(projectLines({ ...repo, changes: null, walkthroughs: null }).map((line) => line.key), ["branch"])
  assert.deepEqual(projectLines({ repository: false, branch: "", changes: 0, walkthroughs: 0, unseen: 0 }).map((line) => line.key), ["folder"])
})

test("projects list only when there is a choice", () => {
  assert.equal(listsProjects(response), true)
  assert.equal(listsProjects({ ...response, projects: response.projects.slice(0, 1) }), false)
  assert.equal(listsProjects({ ...response, projects: response.projects.map((project, index) => ({ ...project, available: index === 0 })) }), false)
})

test("sections group repositories, carry several worktrees for expansion, and drop Browse", () => {
  const sections = welcomeSections(response, [], "/work/app-fix", "")
  const rows = sections.flatMap((section) => section.rows)
  assert.equal(rows.length, 3)
  assert.deepEqual(new Set(rows.map((entry) => entry.row.name)), new Set(["work", "app", "notes"]))
  const app = rows.find((entry) => entry.row.name === "app")
  assert.equal(app?.row.current, true)
  assert.equal(app?.worktrees.length, 2)
  assert.equal(app?.worktrees.find((member) => member.name === "main")?.current, false)
  assert.equal(app?.worktrees.find((member) => member.name === "fix")?.current, true)
  assert.deepEqual(rows.find((entry) => entry.row.name === "notes")?.worktrees, [])
})

test("a query flattens projects into ranked matches without expansion", () => {
  const sections = welcomeSections(response, [], "/work", "fix")
  assert.deepEqual(sections.map((section) => section.rows.map((entry) => [entry.row.name, entry.worktrees.length])), [[["fix", 0]]])
  assert.deepEqual(welcomeSections(response, [], "/work", "zzz"), [])
})
