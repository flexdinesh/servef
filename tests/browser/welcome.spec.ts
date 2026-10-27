import { expect, test, type Page } from "@playwright/test"

import { endsWith, fixtureRoot, projectParam, rootParam } from "./fixture-root.ts"

const at = (...segments: string[]) => [fixtureRoot, ...segments].join("/")

// The fixture root has no repositories; present some of its folders as
// discovered projects, one repository with a worktree.
async function mockProjects(page: Page) {
  await page.route("**/api/projects", async (route) => {
    const response = await route.fetch()
    const json: unknown = await response.json()
    if (typeof json !== "object" || json === null || !("projects" in json) || !Array.isArray(json.projects)) {
      throw new Error("unexpected projects payload")
    }
    const member = (path: string, kind: string, group: string, branch: string) => ({
      id: path, name: path.split("/").at(-1), path, kind, group, root: fixtureRoot, branch,
      repository: group ? "reference" : path.split("/").at(-1), available: true,
    })
    await route.fulfill({ response, json: { ...json, projects: [
      json.projects[0],
      member(at("guides"), "repo", "", "main"),
      member(at("reference"), "repo", at("reference"), "main"),
      member(at("reference", "topics"), "worktree", at("reference"), "topics-branch"),
    ] } })
  })
}

const tabs = (page: Page) => page.getByRole("tablist", { name: "Open documents" })
const welcome = (page: Page) => page.getByRole("main")
const projects = (page: Page) => page.getByRole("region", { name: "Projects" })

test("a first landing opens Welcome with the projects instead of the picker", async ({ page }) => {
  await mockProjects(page)
  await page.goto("/")
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&welcome=1`))
  await expect(tabs(page).getByRole("tab", { name: "Welcome" })).toHaveAttribute("aria-selected", "true")
  await expect(welcome(page).getByRole("heading", { name: "servef", level: 1 })).toBeVisible()
  await expect(page.getByRole("dialog", { name: "Switch project" })).toBeHidden()
  await expect(page).toHaveTitle("Welcome · servef")

  await expect(projects(page).getByRole("button", { name: /^markdown/ })).toContainText("Current")
  await expect(projects(page).getByRole("button", { name: /^(markdown|guides|reference)/ })).toHaveText([
    /markdown.*served folder.*Current/, /guides.*main/, /reference.*main.*2 worktrees/,
  ])

  // A repository's worktrees expand in place.
  await projects(page).getByRole("button", { name: "Show reference worktrees" }).click()
  const worktrees = projects(page).getByRole("list", { name: "reference worktrees" })
  await expect(worktrees.getByRole("button")).toHaveText([/main/, /topics-branch/])
  await projects(page).getByRole("button", { name: "Hide reference worktrees" }).click()
  await expect(worktrees).toBeHidden()

  // The filter flattens projects into matches, as the picker does.
  await projects(page).getByRole("textbox", { name: "Filter projects" }).fill("topics")
  await expect(projects(page).getByRole("button", { name: /topics-branch/ })).toBeVisible()
  await expect(projects(page).getByRole("button", { name: /^guides/ })).toBeHidden()
  await projects(page).getByRole("textbox", { name: "Filter projects" }).fill("zzz")
  await expect(projects(page).getByRole("status")).toHaveText("No projects match “zzz”.")
  await projects(page).getByRole("textbox", { name: "Filter projects" }).fill("gui")
  await projects(page).getByRole("button", { name: /^guides/ }).click()

  // A project with nothing to restore greets with Welcome too.
  await expect(page).toHaveURL(endsWith(`/view?project=${projectParam("guides")}&welcome=1`))
  await expect(page.getByRole("complementary", { name: "Files" }).getByRole("treeitem", { name: "diagrams.md" })).toBeVisible()
  await expect(tabs(page).getByRole("tab")).toHaveText(["Welcome"])
})

test("remembered tabs restore instead of Welcome; with nothing to restore it returns", async ({ page }) => {
  await page.goto(`/view?project=${rootParam}&path=README.md`)
  await expect(tabs(page).getByRole("tab")).toHaveText(["README.md"])
  await page.goto("/")
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&path=README.md`))
  await expect(tabs(page).getByRole("tab")).toHaveText(["README.md"])

  // A closed Welcome stays closed while other tabs restore.
  await page.getByRole("link", { name: "servef home" }).click()
  await page.getByRole("button", { name: "Close Welcome" }).click()
  await expect(tabs(page).getByRole("tab", { name: "README.md" })).toHaveAttribute("aria-selected", "true")
  await page.reload()
  await expect(tabs(page).getByRole("tab")).toHaveText(["README.md"])

  await page.getByRole("button", { name: "Close README.md" }).click()
  await expect(page.getByRole("heading", { name: "Choose a file" })).toBeVisible()
  await page.reload()
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&welcome=1`))
  await expect(tabs(page).getByRole("tab")).toHaveText(["Welcome"])
})
