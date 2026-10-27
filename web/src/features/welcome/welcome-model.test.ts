import assert from "node:assert/strict"
import test from "node:test"

import type { ProjectsResponse } from "../../api/projects-contract.ts"
import { guideEntries, listsProjects, projectLines, startEntries, welcomeSections, type ProjectFacts } from "./welcome-model.ts"

const response: ProjectsResponse = {
  roots: [{ path: "/work", name: "work", isRepo: false }],
  projects: [
    { id: "/work", name: "work", path: "/work", kind: "folder", group: "", root: "/work", branch: "", repository: "", available: true },
    { id: "/work/app", name: "app", path: "/work/app", kind: "repo", group: "/work/app", root: "/work", branch: "main", repository: "app", available: true },
    { id: "/work/app-fix", name: "app-fix", path: "/work/app-fix", kind: "worktree", group: "/work/app", root: "/work", branch: "fix", repository: "app", available: true },
    { id: "/work/notes", name: "notes", path: "/work/notes", kind: "repo", group: "/work/notes", root: "/work", branch: "main", repository: "notes", available: true },
  ],
  warnings: [],
}

test("start and guide entries run actions and show the platform's shortcuts", () => {
  assert.deepEqual(startEntries(false).map((entry) => [entry.label, entry.shortcut]),
    [["Open project…", "Ctrl+O"], ["Find a file", "Ctrl+K"], ["Show all commands", "Ctrl+Shift+P"]])
  assert.deepEqual(startEntries(true).map((entry) => entry.shortcut), ["⌘O", "⌘K", "⇧⌘P"])
  const guide = guideEntries(false)
  assert.deepEqual(guide.map((entry) => entry.action),
    ["find-in-files", "source-control", "walkthroughs", "language-servers", "color-theme", "font-size"])
  assert.ok(guide.every((entry) => entry.description))
  assert.equal(guideEntries(true).find((entry) => entry.action === "find-in-files")?.shortcut, "⇧⌘F")
})

test("project lines name the branch, changes, and walkthroughs, linking where to review them", () => {
  const repo: ProjectFacts = { repository: true, branch: "main", changes: 4, walkthroughs: 2, unseen: 0 }
  assert.deepEqual(projectLines(repo), [
    { key: "branch", text: "On main" },
    { key: "changes", text: "4 uncommitted changes", action: "review-uncommitted" },
    { key: "walkthroughs", text: "2 walkthroughs", action: "walkthroughs" },
  ])
  assert.deepEqual(projectLines({ ...repo, changes: 1, unseen: 1 }).map((line) => line.text), ["On main", "1 uncommitted change", "1 new walkthrough"])
  assert.deepEqual(projectLines({ ...repo, changes: 0, walkthroughs: 0 }), [{ key: "branch", text: "On main" }, { key: "changes", text: "No uncommitted changes" }])
  // Unread counts say nothing rather than zero.
  assert.deepEqual(projectLines({ ...repo, changes: null, walkthroughs: null }), [{ key: "branch", text: "On main" }])
  assert.deepEqual(projectLines({ repository: false, branch: "", changes: 0, walkthroughs: 0, unseen: 0 }),
    [{ key: "folder", text: "Folder, not a git repository" }])
})

test("projects list only when there is a choice", () => {
  assert.equal(listsProjects(response), true)
  assert.equal(listsProjects({ ...response, projects: response.projects.slice(0, 1) }), false)
  assert.equal(listsProjects({ ...response, projects: response.projects.map((project, index) => ({ ...project, available: index === 0 })) }), false)
})

test("sections group repositories, carry several worktrees for expansion, and drop Browse", () => {
  const sections = welcomeSections(response, [], "/work/app-fix", "")
  assert.deepEqual(sections.map((section) => section.title), ["Projects"])
  const rows = sections[0]?.rows ?? []
  assert.deepEqual(rows.map((entry) => entry.row.name), ["work", "app", "notes"])
  const app = rows.find((entry) => entry.row.name === "app")
  assert.equal(app?.row.current, true)
  assert.deepEqual(app?.worktrees.map((member) => [member.name, member.current]), [["main", false], ["fix", true]])
  assert.deepEqual(rows.find((entry) => entry.row.name === "notes")?.worktrees, [])
})

test("a query flattens projects into ranked matches without expansion", () => {
  const sections = welcomeSections(response, [], "/work", "fix")
  assert.deepEqual(sections.map((section) => section.rows.map((entry) => [entry.row.name, entry.worktrees.length])), [[["fix", 0]]])
  assert.deepEqual(welcomeSections(response, [], "/work", "zzz"), [])
})
