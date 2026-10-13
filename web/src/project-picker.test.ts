import assert from "node:assert/strict"
import { readFile } from "node:fs/promises"
import test from "node:test"

import {
  browseRows,
  matchScore,
  parentInput,
  projectSections,
  selectableRows,
  splitBrowseInput,
  type PickerSection,
} from "./project-picker.ts"
import { isProjectsResponse, type ProjectsResponse } from "./projects-contract.ts"

async function projects(): Promise<ProjectsResponse> {
  const value: unknown = JSON.parse(await readFile(new URL("../../testdata/api/default/projects.json", import.meta.url), "utf8"))
  assert.ok(isProjectsResponse(value))
  return value
}

function summary(sections: readonly PickerSection[]) {
  return sections.map((section) => [section.title, section.rows.map((row) => {
    if (row.kind !== "project") return row.kind
    return `${row.nested ? "  " : ""}${row.path}${row.current ? " *" : ""}${row.disabled ? ` (${row.note})` : ""}`
  })])
}

test("projects nest worktrees under their main worktree and mark the current one", async () => {
  assert.deepEqual(summary(projectSections(await projects(), [], "servef/main", "")), [
    ["Projects", [
      ".",
      "notes",
      "servef/main *",
      "  servef/wt-feat-project-picker",
      "  /tmp/servef-hotfix (Outside workspace)",
    ]],
    ["", ["browse"]],
  ])
})

test("opened folders list remembered projects that discovery does not", async () => {
  const sections = projectSections(await projects(), ["notes", "docs/2026", ".", "servef/main"], "docs/2026", "")
  assert.deepEqual(summary(sections)[1], ["Opened folders", ["docs/2026 *"]])
  const opened = sections[1]?.rows[0]
  assert.equal(opened?.kind === "project" && opened.name, "2026")
})

test("a query ranks matches flat, name prefixes first", async () => {
  assert.deepEqual(summary(projectSections(await projects(), ["notes/wt"], ".", "wt")), [
    ["Projects", ["servef/wt-feat-project-picker"]],
    ["Opened folders", ["notes/wt"]],
    ["", ["browse"]],
  ])
  assert.deepEqual(summary(projectSections(await projects(), [], ".", "zzz")), [["", ["browse"]]])
})

test("unavailable projects are listed but not selectable", async () => {
  const rows = selectableRows(projectSections(await projects(), [], ".", ""))
  assert.equal(rows.some((row) => row.kind === "project" && row.disabled), false)
  assert.equal(rows.at(-1)?.kind, "browse")
})

test("matchScore prefers names over paths and requires characters in order", () => {
  assert.equal(matchScore("ser", "servef", "work/servef"), 0)
  assert.equal(matchScore("vef", "servef", "work/servef"), 1)
  assert.equal(matchScore("work", "servef", "work/servef"), 2)
  assert.equal(matchScore("wsf", "servef", "work/servef"), 3)
  assert.equal(matchScore("fsw", "servef", "work/servef"), null)
  assert.equal(matchScore("  ", "a", "a"), 0)
})

test("browse input splits into the listed folder and a partial name", () => {
  assert.deepEqual(splitBrowseInput(""), { folder: ".", partial: "" })
  assert.deepEqual(splitBrowseInput("ap"), { folder: ".", partial: "ap" })
  assert.deepEqual(splitBrowseInput("work/"), { folder: "work", partial: "" })
  assert.deepEqual(splitBrowseInput("./work/ap"), { folder: "work", partial: "ap" })
  assert.deepEqual(splitBrowseInput("/work/api/x"), { folder: "work/api", partial: "x" })
  assert.equal(parentInput("work/api/"), "work/")
  assert.equal(parentInput("work/"), "")
  assert.equal(parentInput(""), "")
})

test("browse rows open the folder first, then matching children", () => {
  const directories = [
    { name: ".github", path: "work/.github", isProject: false },
    { name: "api", path: "work/api", isProject: true },
    { name: "apps", path: "work/apps", isProject: false },
    { name: "web", path: "work/web", isProject: true },
  ]
  const sections = browseRows("work", "ap", directories)
  assert.deepEqual(sections.map((section) => section.rows.map((row) => row.kind === "directory" ? row.path : row.kind)), [
    ["open"],
    ["work/api", "work/apps"],
  ])
  assert.equal(browseRows("work", "zz", directories).length, 1)
})
