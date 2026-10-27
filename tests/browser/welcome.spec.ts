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

test("one project shows no project list", async ({ page }) => {
  await page.goto("/")
  await expect(tabs(page).getByRole("tab", { name: "Welcome" })).toBeVisible()
  await expect(welcome(page).getByRole("heading", { name: "Start" })).toBeVisible()
  await expect(projects(page)).toHaveCount(0)
  await expect(welcome(page).getByText("Folder, not a git repository")).toBeVisible()

  // The Welcome URL reloads as Welcome, not as a missing file.
  await page.reload()
  await expect(welcome(page).getByRole("heading", { name: "servef", level: 1 })).toBeVisible()
  await expect(page.getByText("No file was selected.")).toHaveCount(0)
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

test("the footer brand and the palette open Welcome beside other tabs", async ({ page }) => {
  await page.goto(`/view?project=${rootParam}&path=README.md`)
  await page.getByRole("link", { name: "servef home" }).click()
  await expect(tabs(page).getByRole("tab")).toHaveText(["README.md", "Welcome"])
  await expect(tabs(page).getByRole("tab", { name: "Welcome" })).toHaveAttribute("aria-selected", "true")

  await tabs(page).getByRole("tab", { name: "README.md" }).click()
  await page.keyboard.press("ControlOrMeta+Shift+p")
  await page.getByRole("dialog", { name: "Command palette" }).getByRole("combobox", { name: "Command" }).fill("help welcome")
  await page.keyboard.press("Enter")
  await expect(tabs(page).getByRole("tab", { name: "Welcome" })).toHaveAttribute("aria-selected", "true")
  await expect(tabs(page).getByRole("tab")).toHaveCount(2)
})

test("Welcome entries run what they name", async ({ page }) => {
  await page.goto("/")
  await welcome(page).getByRole("button", { name: "Find in files" }).click()
  await expect(page.getByRole("textbox", { name: "Search", exact: true })).toBeFocused()

  await welcome(page).getByRole("button", { name: "Find a file" }).click()
  await expect(page.getByRole("textbox", { name: "Search…" })).toBeFocused()

  await welcome(page).getByRole("button", { name: "Change in Settings" }).click()
  const settings = page.getByRole("dialog", { name: "Settings" })
  await expect(settings.getByRole("combobox", { name: "Workbench: Show Welcome on Startup" })).toBeFocused()

  await expect(welcome(page).getByRole("link", { name: /Documentation/ })).toHaveAttribute("href", "https://github.com/flexdinesh/servef#readme")
})

test("the empty pane follows the sidebar view", async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem("servef-settings", JSON.stringify({ showWelcome: "off" })))
  await page.goto(`/?project=${rootParam}`)
  const pane = page.locator("#document-pane")
  await expect(pane.getByRole("heading", { name: "Choose a file" })).toBeVisible()
  await pane.getByRole("button", { name: /Find a file/ }).click()
  await expect(page.getByRole("textbox", { name: "Search…" })).toBeFocused()

  await page.keyboard.press("ControlOrMeta+Shift+f")
  await expect(pane.getByRole("heading", { name: "Search file contents" })).toBeVisible()
  await page.getByRole("textbox", { name: "Search", exact: true }).fill("luminous-orchid")
  await expect(pane.getByRole("heading", { name: "Open a match" })).toBeVisible()

  await page.getByRole("button", { name: "Walkthroughs" }).click()
  await expect(pane.getByRole("heading", { name: /walkthrough/i })).toBeVisible()

  await page.getByRole("button", { name: "Source Control" }).click()
  await expect(pane.getByRole("heading", { name: "Not a git repository" })).toBeVisible()

  await page.getByRole("button", { name: "Source Control" }).click()
  await expect(pane.getByRole("heading", { name: "Nothing open" })).toBeVisible()
  await pane.getByRole("button", { name: /Show sidebar/ }).click()
  await expect(page.getByRole("region", { name: "Source Control" })).toBeVisible()

  await pane.getByRole("button", { name: "Show Welcome" }).click()
  await expect(tabs(page).getByRole("tab", { name: "Welcome" })).toHaveAttribute("aria-selected", "true")
})
