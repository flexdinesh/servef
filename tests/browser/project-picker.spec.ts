import { expect, test, type Page } from "@playwright/test"

// The fixture root has no repositories; present some of its folders as
// discovered projects, including a worktree outside the root.
async function mockProjects(page: Page) {
  await page.route("**/api/projects", async (route) => {
    const response = await route.fetch()
    const json: unknown = await response.json()
    if (typeof json !== "object" || json === null || !("projects" in json) || !Array.isArray(json.projects)) {
      throw new Error("unexpected projects payload")
    }
    await route.fulfill({ response, json: { ...json, projects: [
      json.projects[0],
      { id: "guides", name: "guides", path: "guides", kind: "repo", group: "", available: true },
      { id: "reference", name: "reference", path: "reference", kind: "repo", group: "reference", available: true },
      { id: "reference/topics", name: "topics", path: "reference/topics", kind: "worktree", group: "reference", available: true },
      { id: "", name: "hotfix", path: "/elsewhere/hotfix", kind: "worktree", group: "reference", available: false },
    ] } })
  })
}

const picker = (page: Page) => page.getByRole("dialog", { name: /Switch project|Open folder/ })
const pickerInput = (page: Page) => picker(page).getByRole("combobox")
const tree = (page: Page) => page.getByRole("complementary", { name: "Files" })
const trigger = (page: Page) => tree(page).getByRole("button", { name: /Switch project/ })

test("Ctrl+O switches projects and starts a fresh workspace", async ({ page }) => {
  await mockProjects(page)
  await page.goto("/view?project=.&path=guides%2Fgetting-started.md")
  await page.getByRole("textbox", { name: /Filter/ }).fill("*.md")
  await page.keyboard.press("Control+o")
  await expect(picker(page)).toBeVisible()
  await expect(picker(page).getByRole("option")).toHaveText([
    /markdown.*Current/, /guides/, /reference/, /topics/, /hotfix.*Outside markdown/, /Open folder/,
  ])

  await pickerInput(page).fill("gui")
  await expect(picker(page).getByRole("option")).toHaveText([/guides/, /Open folder/])
  await pickerInput(page).press("Enter")

  await expect(page).toHaveURL(/\/\?project=guides$/)
  await expect(picker(page)).toBeHidden()
  await expect(trigger(page)).toHaveAccessibleName("Switch project (guides)")
  await expect(page.getByRole("textbox", { name: /Filter/ })).toHaveValue("")
  await expect(page.getByRole("tablist", { name: "Open documents" }).getByRole("tab")).toHaveCount(0)
  await expect(tree(page).getByRole("treeitem", { name: "diagrams.md" })).toBeVisible()
})

test("worktrees outside the served root are listed but cannot be opened", async ({ page }) => {
  await mockProjects(page)
  await page.goto("/?project=.")
  await trigger(page).click()
  const hotfix = picker(page).getByRole("option", { name: /hotfix/ })
  await expect(hotfix).toHaveAttribute("aria-disabled", "true")
  await expect(hotfix).toContainText("Outside markdown")
  await hotfix.click({ force: true })
  await expect(picker(page)).toBeVisible()
  await expect(page).toHaveURL(/\/\?project=\.$/)
})

test("browsing opens any folder, which stays listed until forgotten", async ({ page }) => {
  await mockProjects(page)
  await page.goto("/?project=.")
  await trigger(page).click()
  await pickerInput(page).fill("/")
  await expect(picker(page).getByRole("listbox", { name: "Folders" })).toBeVisible()
  const folders = picker(page).getByRole("group", { name: "Folders" }).getByRole("option")
  await expect(folders).toHaveText([/assets/, /code/, /config/, /guides/, /reference/])

  await pickerInput(page).pressSequentially("con")
  await pickerInput(page).press("Tab")
  await expect(pickerInput(page)).toHaveValue("config/")
  await expect(picker(page).getByRole("option").first()).toHaveAccessibleName("Open config/")
  await pickerInput(page).press("Enter")
  await expect(page).toHaveURL(/\/\?project=config$/)
  await expect(tree(page).getByRole("treeitem", { name: "settings.json" })).toBeVisible()

  await page.keyboard.press("Control+o")
  const opened = picker(page).getByRole("group", { name: "Opened folders" })
  await expect(opened.getByRole("option", { name: "config" })).toContainText("Current")
  await pickerInput(page).fill("guides")
  await pickerInput(page).press("Enter")
  await expect(page).toHaveURL(/\/\?project=guides$/)

  await page.keyboard.press("Control+o")
  await opened.getByRole("option", { name: "config" }).hover()
  await picker(page).getByRole("button", { name: "Forget config" }).click()
  await expect(opened).toHaveCount(0)
})

test("Backspace walks up while browsing and back to projects", async ({ page }) => {
  await page.goto("/?project=.")
  await page.keyboard.press("Control+o")
  await pickerInput(page).fill("/")
  await pickerInput(page).pressSequentially("reference/topics/")
  await expect(picker(page).getByRole("option").first()).toHaveAccessibleName("Open reference/topics/")
  await pickerInput(page).press("Backspace")
  await expect(pickerInput(page)).toHaveValue("reference/")
  await pickerInput(page).press("Backspace")
  await expect(pickerInput(page)).toHaveValue("")
  await pickerInput(page).press("Backspace")
  await expect(picker(page).getByRole("listbox", { name: "Projects" })).toBeVisible()
})

test("Escape closes the picker and returns focus to its trigger", async ({ page }) => {
  await page.goto("/?project=.")
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
  await expect(page).toHaveURL(/\/\?project=\.$/)
})

test("the picker switches projects from the mobile drawer", async ({ page }) => {
  await mockProjects(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto("/?project=.")
  await page.getByRole("button", { name: "Open files" }).click()
  const drawer = page.getByRole("dialog", { name: "Files" })
  await drawer.getByRole("button", { name: /Switch project/ }).click()
  await picker(page).getByRole("option", { name: /reference/ }).first().click()
  await expect(page).toHaveURL(/\/\?project=reference$/)
  await expect(picker(page)).toBeHidden()
})

test("the mobile toolbar opens the picker without the drawer", async ({ page }) => {
  await mockProjects(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto("/?project=.")
  const toolbarButton = page.getByRole("region", { name: "Document workspace" }).getByRole("button", { name: "Switch project (markdown)" })
  await toolbarButton.click()
  await expect(page.getByRole("dialog", { name: "Files" })).toBeHidden()
  await picker(page).getByRole("option", { name: /guides/ }).click()
  await expect(page).toHaveURL(/\/\?project=guides$/)
  await expect(page.getByRole("region", { name: "Document workspace" }).getByRole("button", { name: "Switch project (guides)" })).toBeVisible()
})

test("the toolbar project button is hidden on desktop", async ({ page }) => {
  await page.goto("/?project=.")
  await expect(trigger(page)).toBeVisible()
  await expect(page.getByRole("region", { name: "Document workspace" }).getByRole("button", { name: /Switch project/ })).toBeHidden()
})
