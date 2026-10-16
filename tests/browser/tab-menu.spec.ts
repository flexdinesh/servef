import { expect, test, type Page } from "@playwright/test"

async function openTabs(page: Page) {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  const tree = page.getByRole("complementary", { name: "Files" })
  await tree.getByRole("treeitem", { name: "README.md", exact: true }).click()
  await tree.getByRole("treeitem", { name: "code-blocks.md", exact: true }).click()
  await expect(page.getByRole("tab")).toHaveText(["getting-started.md", "README.md", "code-blocks.md"])
}

test("right click leaves the active tab unchanged and Escape restores focus", async ({ page }) => {
  await openTabs(page)
  const target = page.getByRole("tab", { name: "getting-started.md" })
  await target.click({ button: "right" })
  const menu = page.getByRole("menu", { name: "Tab actions for getting-started.md" })
  await expect(menu).toBeVisible()
  await expect(page.getByRole("tab", { name: "code-blocks.md", includeHidden: true })).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/path=guides%2Fcode-blocks.md/)
  await menu.press("Escape")
  await expect(menu).toBeHidden()
  await expect(target).toBeFocused()
})

test("keyboard context menu closes a tab and preserves its nearest neighbour", async ({ page }) => {
  await openTabs(page)
  const active = page.getByRole("tab", { name: "code-blocks.md" })
  await active.focus()
  await active.press("Shift+F10")
  const menu = page.getByRole("menu", { name: "Tab actions for code-blocks.md" })
  await expect(menu).toBeVisible()
  await expect(menu.getByRole("menuitem", { name: "Close to the Right", exact: true })).toHaveAttribute("aria-disabled", "true")
  await page.keyboard.press("ArrowDown")
  await expect(menu.getByRole("menuitem", { name: "Close", exact: true })).toBeFocused()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("tab")).toHaveText(["getting-started.md", "README.md"])
  await expect(page.getByRole("tab", { name: "README.md" })).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/path=README.md/)
})

test("Close Others keeps the context tab, opens it, and remembers the result", async ({ page }) => {
  await openTabs(page)
  await page.getByRole("tab", { name: "README.md" }).click({ button: "right" })
  await page.getByRole("menuitem", { name: "Close Others", exact: true }).click()
  await expect(page.getByRole("tab")).toHaveText(["README.md"])
  await expect(page.getByRole("tab", { name: "README.md" })).toHaveAttribute("aria-selected", "true")
  await page.reload()
  await expect(page.getByRole("tab")).toHaveText(["README.md"])
  await page.getByRole("tab", { name: "README.md" }).press("Shift+F10")
  await expect(page.getByRole("menuitem", { name: "Close Others", exact: true })).toHaveAttribute("aria-disabled", "true")
})

test("Close to the Right preserves tab order and removes the active document", async ({ page }) => {
  await openTabs(page)
  await page.getByRole("tab", { name: "README.md" }).click({ button: "right" })
  await page.getByRole("menuitem", { name: "Close to the Right", exact: true }).click()
  await expect(page.getByRole("tab")).toHaveText(["getting-started.md", "README.md"])
  await expect(page.getByRole("tab", { name: "README.md" })).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/path=README.md/)
})

test("Close All clears the workspace and stays closed after reload", async ({ page }) => {
  await openTabs(page)
  await page.getByRole("tab", { name: "README.md" }).click({ button: "right" })
  await page.getByRole("menuitem", { name: "Close All", exact: true }).click()
  await expect(page.getByRole("tab")).toHaveCount(0)
  await expect(page.getByRole("heading", { name: "Choose a file" })).toBeVisible()
  await expect(page.locator("#document-pane")).toBeFocused()
  await page.reload()
  await expect(page.getByRole("tab")).toHaveCount(0)
  await expect(page.getByRole("heading", { name: "Choose a file" })).toBeVisible()
})
