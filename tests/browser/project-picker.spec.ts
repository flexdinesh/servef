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

interface Checkout {
  repository: string
  projectKind: "folder" | "repo" | "worktree"
  branch: string
  worktrees: number
}

// withCheckout reports a repository checkout for pages of the given project.
async function withCheckout(page: Page, project: string, checkout: Checkout) {
  await page.route(`**/api/page?project=${project}*`, async (route) => {
    const response = await route.fetch()
    const json: unknown = await response.json()
    if (typeof json !== "object" || json === null) throw new Error("unexpected page payload")
    await route.fulfill({ response, json: { ...json, ...checkout } })
  })
}

const picker = (page: Page) => page.getByRole("dialog", { name: /Switch project|Open folder/ })
const pickerInput = (page: Page) => picker(page).getByRole("combobox")
const tree = (page: Page) => page.getByRole("complementary", { name: "Files" })
const trigger = (page: Page) => tree(page).getByRole("button", { name: /Switch project/ })

// startBrowsing types "/" once projects load: their roots decide where browsing starts.
async function startBrowsing(page: Page) {
  await expect(picker(page).getByRole("option").first()).toBeVisible()
  await pickerInput(page).fill("/")
}

test("Ctrl+O switches projects and starts a fresh workspace", async ({ page }) => {
  await mockProjects(page)
  await page.goto(`/view?project=${rootParam}&path=guides%2Fgetting-started.md`)
  await page.getByRole("textbox", { name: /Filter/ }).fill("*.md")
  await page.keyboard.press("Control+o")
  await expect(picker(page)).toBeVisible()
  await expect(picker(page).getByRole("option")).toHaveText([
    /markdown.*served folder.*Current/, /guides.*main/, /reference.*main.*3 worktrees/, /Open folder/,
  ])

  await pickerInput(page).fill("gui")
  await expect(picker(page).getByRole("option")).toHaveText([/guides/, /Open folder/])
  await pickerInput(page).press("Enter")

  await expect(page).toHaveURL(endsWith(`/?project=${projectParam("guides")}`))
  await expect(picker(page)).toBeHidden()
  await expect(trigger(page)).toHaveAccessibleName("Switch project (guides)")
  await expect(page.getByRole("textbox", { name: /Filter/ })).toHaveValue("")
  await expect(page.getByRole("tablist", { name: "Open documents" }).getByRole("tab")).toHaveCount(0)
  await expect(tree(page).getByRole("treeitem", { name: "diagrams.md" })).toBeVisible()
})

test("a repository opens into its worktrees, which open and are remembered", async ({ page }) => {
  await mockProjects(page)
  await page.goto(`/?project=${rootParam}`)
  await trigger(page).click()
  await pickerInput(page).press("ArrowDown")
  await pickerInput(page).press("ArrowDown")
  await pickerInput(page).press("ArrowRight")

  const worktrees = picker(page).getByRole("listbox", { name: "Worktrees" })
  await expect(worktrees.getByRole("option")).toHaveText([/main/, /topics-branch/, /hotfix.*Outside served folders/])
  await expect(worktrees.getByRole("option", { name: /hotfix/ })).toHaveAttribute("aria-disabled", "true")
  await pickerInput(page).fill("topics")
  await pickerInput(page).press("Enter")
  await expect(page).toHaveURL(endsWith(`/?project=${projectParam("reference", "topics")}`))
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

test("searching finds worktrees by branch", async ({ page }) => {
  await mockProjects(page)
  await page.goto(`/?project=${rootParam}`)
  await trigger(page).click()
  await pickerInput(page).fill("topics-b")
  await expect(picker(page).getByRole("option")).toHaveText([/reference ›.*topics-branch/, /Open folder/])
  await pickerInput(page).press("Enter")
  await expect(page).toHaveURL(endsWith(`/?project=${projectParam("reference", "topics")}`))
})

test("the header branch lists the repository's worktrees", async ({ page }) => {
  await mockProjects(page)
  await withCheckout(page, projectParam("reference"), { repository: "reference", projectKind: "repo", branch: "main", worktrees: 3 })
  await page.goto(`/?project=${projectParam("reference")}`)
  await tree(page).getByRole("button", { name: "Switch worktree (main)" }).click()
  const worktrees = picker(page).getByRole("listbox", { name: "Worktrees" })
  await expect(worktrees.getByRole("option", { name: /main/ })).toContainText("Current")
  await worktrees.getByRole("option", { name: /topics-branch/ }).click()
  await expect(page).toHaveURL(endsWith(`/?project=${projectParam("reference", "topics")}`))
})

test("a linked worktree shows its repository name and a worktree icon", async ({ page }) => {
  await mockProjects(page)
  await withCheckout(page, projectParam("reference", "topics"), { repository: "refdocs", projectKind: "worktree", branch: "topics-branch", worktrees: 3 })
  await page.goto(`/?project=${projectParam("reference", "topics")}`)
  await expect(trigger(page)).toHaveAccessibleName("Switch project (refdocs)")
  const branch = tree(page).getByRole("button", { name: "Switch worktree (topics-branch)" })
  await expect(branch.locator(".worktree-icon")).toBeVisible()
  await branch.click()
  await expect(picker(page).getByRole("listbox", { name: "Worktrees" })).toBeVisible()
})

test("a repository without worktrees shows its branch as text", async ({ page }) => {
  await mockProjects(page)
  await withCheckout(page, projectParam("guides"), { repository: "guides", projectKind: "repo", branch: "main", worktrees: 1 })
  await page.goto(`/?project=${projectParam("guides")}`)
  await expect(tree(page).locator(".project-branch")).toHaveText("main")
  await expect(tree(page).getByRole("button", { name: /Switch worktree/ })).toHaveCount(0)
  await expect(tree(page).locator(".project-branch .worktree-icon")).toHaveCount(0)
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
  await expect(page).toHaveURL(endsWith(`/?project=${projectParam("config")}`))
  await expect(tree(page).getByRole("treeitem", { name: "settings.json" })).toBeVisible()

  await page.keyboard.press("Control+o")
  const opened = picker(page).getByRole("group", { name: "Opened folders" })
  await expect(opened.getByRole("option", { name: "config" })).toContainText("Current")
  await pickerInput(page).fill("guides")
  await pickerInput(page).press("Enter")
  await expect(page).toHaveURL(endsWith(`/?project=${projectParam("guides")}`))

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
  await expect(page).toHaveURL(endsWith(`/?project=${projectParam("guides")}`))
})

test("Backspace walks up while browsing and back to projects", async ({ page }) => {
  await page.goto(`/?project=${rootParam}`)
  // The load event precedes the app's first render; wait for its shortcuts.
  await expect(trigger(page)).toBeVisible()
  await page.keyboard.press("Control+o")
  await startBrowsing(page)
  await pickerInput(page).pressSequentially("reference/topics/")
  await expect(picker(page).getByRole("option").first()).toHaveAccessibleName(`Open ${root}/reference/topics/`)
  await pickerInput(page).press("Backspace")
  await expect(pickerInput(page)).toHaveValue(`${root}/reference/`)
  await pickerInput(page).press("Backspace")
  await expect(pickerInput(page)).toHaveValue(`${root}/`)
  await pickerInput(page).press("Backspace")
  await expect(pickerInput(page)).toHaveValue("")
  await expect(picker(page).getByRole("group", { name: "Served folders" }).getByRole("option")).toHaveText([/markdown/])
  await pickerInput(page).press("Backspace")
  await expect(picker(page).getByRole("listbox", { name: "Projects" })).toBeVisible()
})

test("Escape closes the picker and returns focus to its trigger", async ({ page }) => {
  await page.goto(`/?project=${rootParam}`)
  await trigger(page).click()
  await expect(pickerInput(page)).toBeFocused()
  await page.keyboard.press("Escape")
  await expect(picker(page)).toBeHidden()
  await expect(trigger(page)).toBeFocused()
})

test("a first visit with several projects opens the picker", async ({ page }) => {
  await mockProjects(page)
  await page.goto("/")
  await expect(picker(page)).toBeVisible()
  await expect(page).toHaveURL(endsWith(`/?project=${rootParam}`))
})

test("the picker switches projects from the mobile drawer", async ({ page }) => {
  await mockProjects(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`/?project=${rootParam}`)
  await page.getByRole("button", { name: "Open files" }).click()
  const drawer = page.getByRole("dialog", { name: "Files" })
  await drawer.getByRole("button", { name: /Switch project/ }).click()
  await picker(page).getByRole("option", { name: /reference/ }).first().click()
  await expect(page).toHaveURL(endsWith(`/?project=${projectParam("reference")}`))
  await expect(picker(page)).toBeHidden()
})

test("the mobile toolbar opens the picker without the drawer", async ({ page }) => {
  await mockProjects(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`/?project=${rootParam}`)
  const toolbarButton = page.getByRole("region", { name: "Document workspace" }).getByRole("button", { name: "Switch project (markdown)" })
  await toolbarButton.click()
  await expect(page.getByRole("dialog", { name: "Files" })).toBeHidden()
  await picker(page).getByRole("option", { name: /guides/ }).click()
  await expect(page).toHaveURL(endsWith(`/?project=${projectParam("guides")}`))
  await expect(page.getByRole("region", { name: "Document workspace" }).getByRole("button", { name: "Switch project (guides)" })).toBeVisible()
})

test("the toolbar project button is hidden on desktop", async ({ page }) => {
  await page.goto(`/?project=${rootParam}`)
  await expect(trigger(page)).toBeVisible()
  await expect(page.getByRole("region", { name: "Document workspace" }).getByRole("button", { name: /Switch project/ })).toBeHidden()
})
