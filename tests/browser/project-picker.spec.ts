import { expect, test, type Page } from "@playwright/test"

import { endsWith, fixtureRoot, projectParam, rootParam } from "./fixture-root.ts"

const root = fixtureRoot
const at = (...segments: string[]) => [root, ...segments].join("/")

// The fixture root has no repositories; present some of its folders as
// discovered projects: a repository, and one with a worktree inside the root
// and another that cannot be opened.
async function mockProjects(page: Page) {
  await page.route("**/api/projects", async (route) => {
    const response = await route.fetch()
    const json: unknown = await response.json()
    if (typeof json !== "object" || json === null || !("projects" in json) || !Array.isArray(json.projects)) {
      throw new Error("unexpected projects payload")
    }
    const member = (path: string, kind: string, group: string, branch: string, available = true) => ({
      id: available ? path : "", name: path.split("/").at(-1), path, kind, group, root: available ? root : "", branch,
      repository: group ? "reference" : path.split("/").at(-1), available,
    })
    await route.fulfill({ response, json: { ...json, projects: [
      json.projects[0],
      member(at("guides"), "repo", "", "main"),
      member(at("reference"), "repo", at("reference"), "main"),
      member(at("reference", "topics"), "worktree", at("reference"), "topics-branch"),
      member("/elsewhere/hotfix", "worktree", at("reference"), "hotfix", false),
    ] } })
  })
}

// welcomed matches a switched-to project with nothing to restore, which opens
// on Welcome (decision 0052).
const welcomed = (project: string) => endsWith(`/view?project=${project}&welcome=1`)

const picker = (page: Page) => page.getByRole("dialog", { name: /Switch project|Open folder/ })
const pickerInput = (page: Page) => picker(page).getByRole("combobox")
const tree = (page: Page) => page.getByRole("complementary", { name: "Files" })
const trigger = (page: Page) => tree(page).getByRole("button", { name: /Switch project/ })

// startBrowsing types "/" once projects load: their roots decide where browsing starts.
async function startBrowsing(page: Page) {
  await expect(picker(page).getByRole("option").first()).toBeVisible()
  await pickerInput(page).fill("/")
}

test("a repository opens into its worktrees, which open and are remembered", async ({ page }) => {
  await mockProjects(page)
  await page.goto(`/?project=${rootParam}`)
  await trigger(page).click()
  await expect(picker(page).getByRole("option", { name: /^reference/ })).toBeVisible()
  await pickerInput(page).press("ArrowDown")
  await pickerInput(page).press("ArrowDown")
  await pickerInput(page).press("ArrowRight")

  const worktrees = picker(page).getByRole("listbox", { name: "Worktrees" })
  await expect(worktrees.getByRole("option")).toHaveText([/main/, /topics-branch/, /hotfix.*Outside served folders/])
  await expect(worktrees.getByRole("option", { name: /hotfix/ })).toHaveAttribute("aria-disabled", "true")
  await pickerInput(page).fill("topics")
  await pickerInput(page).press("Enter")
  await expect(page).toHaveURL(welcomed(projectParam("reference", "topics")))
  await expect(tree(page).getByRole("treeitem", { name: "search.md" })).toBeVisible()

  // The repository row now opens the last-used worktree.
  await page.keyboard.press("Control+o")
  const reference = picker(page).getByRole("option", { name: /^reference/ })
  await expect(reference).toContainText("topics-branch")
  await expect(reference).toContainText("Current")
  await picker(page).getByRole("button", { name: "Show reference worktrees" }).click()
  await expect(picker(page).getByRole("listbox", { name: "Worktrees" })).toBeVisible()
  await pickerInput(page).press("Backspace")
  await expect(picker(page).getByRole("listbox", { name: "Projects" })).toBeVisible()
})

test("browsing opens folders, which stay listed until forgotten", async ({ page }) => {
  await mockProjects(page)
  await page.goto(`/?project=${rootParam}`)
  await trigger(page).click()
  await startBrowsing(page)
  await expect(picker(page).getByRole("listbox", { name: "Folders" })).toBeVisible()
  await expect(pickerInput(page)).toHaveValue(`${root}/`)
  const folders = picker(page).getByRole("group", { name: "Folders" }).getByRole("option")
  await expect(folders).toHaveText([/assets/, /code/, /config/, /guides/, /navigation/, /reference/])

  await pickerInput(page).pressSequentially("con")
  await pickerInput(page).press("Tab")
  await expect(pickerInput(page)).toHaveValue(`${root}/config/`)
  await expect(picker(page).getByRole("option").first()).toHaveAccessibleName(`Open ${root}/config/`)
  await pickerInput(page).press("Enter")
  await expect(page).toHaveURL(welcomed(projectParam("config")))
  await expect(tree(page).getByRole("treeitem", { name: "settings.json" })).toBeVisible()

  await page.keyboard.press("Control+o")
  const opened = picker(page).getByRole("group", { name: "Opened folders" })
  await expect(opened.getByRole("option", { name: "config" })).toContainText("Current")
  await pickerInput(page).fill("guides")
  await pickerInput(page).press("Enter")
  await expect(page).toHaveURL(welcomed(projectParam("guides")))

  await page.keyboard.press("Control+o")
  await opened.getByRole("option", { name: "config" }).hover()
  await picker(page).getByRole("button", { name: `Forget ${at("config")}` }).click()
  await expect(opened).toHaveCount(0)
})

test("browsing does not enter repositories but opens them whole", async ({ page }) => {
  await page.route("**/api/directories?*", async (route) => {
    const response = await route.fetch()
    const json: unknown = await response.json()
    if (typeof json !== "object" || json === null || !("directories" in json) || !Array.isArray(json.directories)) {
      throw new Error("unexpected directories payload")
    }
    const directories: unknown[] = json.directories.map((directory: unknown) =>
      typeof directory === "object" && directory !== null && "name" in directory && directory.name === "guides"
        ? { ...directory, isProject: true }
        : directory)
    await route.fulfill({ response, json: { ...json, directories } })
  })
  await page.goto(`/?project=${rootParam}`)
  await trigger(page).click()
  await startBrowsing(page)
  await pickerInput(page).pressSequentially("gui")
  await expect(picker(page).getByRole("option", { name: /guides/ })).toContainText("git")
  await pickerInput(page).press("Tab")
  await expect(pickerInput(page)).toHaveValue(`${root}/gui`)
  await picker(page).getByRole("option", { name: /guides/ }).click()
  await expect(page).toHaveURL(welcomed(projectParam("guides")))
})

test("the picker switches projects from the mobile drawer", async ({ page }) => {
  await mockProjects(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`/?project=${rootParam}`)
  await page.getByRole("button", { name: "Open files" }).click()
  const drawer = page.getByRole("dialog", { name: "Files" })
  await drawer.getByRole("button", { name: /Switch project/ }).click()
  await picker(page).getByRole("option", { name: /reference/ }).first().click()
  await expect(page).toHaveURL(welcomed(projectParam("reference")))
  await expect(picker(page)).toBeHidden()
})
