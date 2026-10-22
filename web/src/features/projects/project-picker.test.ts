import assert from "node:assert/strict"
import { readFile } from "node:fs/promises"
import test from "node:test"

import {
  browseRows,
  browseStart,
  browseTarget,
  canDrill,
  currentGroup,
  matchScore,
  parentInput,
  projectSections,
  selectableRows,
  splitBrowseInput,
  type PickerSection,
} from "./project-picker.ts"
import { isProjectsResponse, type ProjectsResponse } from "../../projects-contract.ts"

async function projects(): Promise<ProjectsResponse> {
  const value: unknown = JSON.parse(await readFile(new URL("../../../../testdata/api/default/projects.json", import.meta.url), "utf8"))
  assert.ok(isProjectsResponse(value))
  return value
}

const workspace = "/home/user/markdown"
const servef = `${workspace}/servef/main`

function summary(sections: readonly PickerSection[]) {
  return sections.map((section) => [section.title, section.rows.map((row) => {
    if (row.kind !== "project") return row.kind
    const extras = [
      row.repository && `in ${row.repository}`,
      row.branch && `@${row.branch}`,
      row.worktrees > 1 && `${row.worktrees} worktrees`,
      row.current && "*",
      row.disabled && `(${row.note})`,
    ].filter(Boolean)
    return [row.name, ...extras].join(" ")
  })])
}

test("projects list each root's folder and repositories, a repository with worktrees as one row", async () => {
  assert.deepEqual(summary(projectSections(await projects(), [], `${workspace}/servef/wt-feat-project-picker`, "")), [
    ["markdown", ["markdown", "notes @main", "servef @main 3 worktrees *"]],
    ["personal", ["personal", "blog @draft"]],
    ["", ["browse"]],
  ])
})

test("a repository row opens its last-used worktree", async () => {
  const response = await projects()
  const remembered = [`${workspace}/notes`, "/tmp/servef-hotfix", servef]
  const row = projectSections(response, remembered, "", "")[0]?.rows[2]
  assert.ok(row?.kind === "project")
  assert.equal(row.id, "/tmp/servef-hotfix")
  assert.equal(row.branch, "hotfix")
  assert.ok(canDrill(row))
  const fresh = projectSections(response, [], "", "")[0]?.rows[2]
  assert.equal(fresh?.kind === "project" && fresh.id, servef)
})

test("a repository's worktree list shows branches, main first", async () => {
  const response = await projects()
  assert.deepEqual(summary(projectSections(response, [], servef, "", servef)), [
    ["servef worktrees", ["main @main *", "feat-project-picker @feat-project-picker", "hotfix @hotfix"]],
  ])
  assert.deepEqual(summary(projectSections(response, [], servef, "hot", servef)), [["servef worktrees", ["hotfix @hotfix"]]])
  assert.deepEqual(projectSections(response, [], servef, "zzz", servef), [])
  assert.equal(currentGroup(response, "/tmp/servef-hotfix"), servef)
  assert.equal(currentGroup(response, `${workspace}/notes`), "")
})

test("unavailable worktrees are listed but not selectable", async () => {
  const response = await projects()
  const hotfix = response.projects.find((project) => project.path === "/tmp/servef-hotfix")
  assert.ok(hotfix)
  const unavailable: ProjectsResponse = {
    ...response,
    projects: response.projects.map((project) => project === hotfix ? { ...project, id: "", root: "", available: false } : project),
  }
  const sections = projectSections(unavailable, [], "", "", servef)
  assert.deepEqual(summary(sections)[0]?.[1], ["main @main", "feat-project-picker @feat-project-picker", "hotfix @hotfix (Outside served folders)"])
  assert.equal(selectableRows(sections).length, 2)
})

test("opened folders list remembered folders in the roots that discovery does not", async () => {
  const opened = `${workspace}/docs/2026`
  const sections = projectSections(await projects(), [`${workspace}/notes`, opened, "/elsewhere/old", workspace], opened, "")
  assert.deepEqual(summary(sections)[2], ["Opened folders", ["2026 *"]])
})

test("a query ranks matches flat, including each worktree by branch", async () => {
  assert.deepEqual(summary(projectSections(await projects(), [`${workspace}/notes/wt`], "", "feat")), [
    ["Projects", ["feat-project-picker in servef @feat-project-picker"]],
    ["", ["browse"]],
  ])
  assert.deepEqual(summary(projectSections(await projects(), [`${workspace}/notes/wt`], "", "wt")), [
    ["Projects", ["feat-project-picker in servef @feat-project-picker"]],
    ["Opened folders", ["wt"]],
    ["", ["browse"]],
  ])
  assert.deepEqual(summary(projectSections(await projects(), [], "", "zzz")), [["", ["browse"]]])
})

test("matchScore prefers names over paths and requires characters in order", () => {
  assert.equal(matchScore("ser", "servef", "work/servef"), 0)
  assert.equal(matchScore("vef", "servef", "work/servef"), 1)
  assert.equal(matchScore("work", "servef", "work/servef"), 2)
  assert.equal(matchScore("wsf", "servef", "work/servef"), 3)
  assert.equal(matchScore("fsw", "servef", "work/servef"), null)
  assert.equal(matchScore("  ", "a", "a"), 0)
})

test("browse input splits into a folder within the roots and a partial name", () => {
  const roots = ["/w", "/home/p"]
  assert.deepEqual(splitBrowseInput("", roots), { folder: "", partial: "" })
  assert.deepEqual(splitBrowseInput("wo", roots), { folder: "", partial: "wo" })
  assert.deepEqual(splitBrowseInput("/home/", roots), { folder: "", partial: "/home/" })
  assert.deepEqual(splitBrowseInput("/w/", roots), { folder: "/w", partial: "" })
  assert.deepEqual(splitBrowseInput("/w/work/ap", roots), { folder: "/w/work", partial: "ap" })
  assert.deepEqual(splitBrowseInput("/wx/a", roots), { folder: "", partial: "/wx/a" })
  assert.equal(parentInput("/w/work/api/", roots), "/w/work/")
  assert.equal(parentInput("/w/work/", roots), "/w/")
  assert.equal(parentInput("/w/", roots), "")
  assert.equal(parentInput("", roots), "")
  assert.equal(browseStart(["/w"]), "/w/")
  assert.equal(browseStart(roots), "")
})

test("browse rows open the folder first, then matching children; repositories open whole", () => {
  const directories = [
    { name: ".github", path: "/w/work/.github", isProject: false },
    { name: "api", path: "/w/work/api", isProject: true },
    { name: "apps", path: "/w/work/apps", isProject: false },
    { name: "web", path: "/w/work/web", isProject: true },
  ]
  const sections = browseRows("/w/work", "ap", directories)
  assert.deepEqual(sections.map((section) => section.rows.map((row) => row.kind === "directory" ? row.path : row.kind)), [
    ["open"],
    ["/w/work/api", "/w/work/apps"],
  ])
  assert.equal(browseRows("/w/work", "zz", directories).length, 1)
  const rows = selectableRows(sections)
  assert.equal(browseTarget(rows, rows[1])?.path, "/w/work/apps")
  assert.equal(browseTarget(rows, rows[2])?.path, "/w/work/apps")
})

test("the root list offers no folder to open", () => {
  const sections = browseRows("", "", [{ name: "w", path: "/w", isProject: false }, { name: "repo", path: "/repo", isProject: true }])
  assert.deepEqual(sections.map((section) => [section.title, section.rows.map((row) => row.kind)]), [["Served folders", ["directory", "directory"]]])
})
