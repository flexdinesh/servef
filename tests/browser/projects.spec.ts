import { expect, test } from "@playwright/test"

test("a URL without a project opens the served root", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  await expect(page).toHaveURL(/\/view\?project=\.&path=guides%2Fgetting-started\.md$/)
  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
})

test("a project scopes the tree to its directory", async ({ page }) => {
  await page.goto("/view?project=.&path=guides%2Fgetting-started.md")
  const tree = page.getByRole("complementary", { name: "Files" })
  const tabs = page.getByRole("tablist", { name: "Open documents" })
  await tree.getByRole("treeitem", { name: "README.md" }).click()
  await expect(tabs.getByRole("tab")).toHaveCount(2)

  await page.goto("/?project=guides")

  await expect(tree.getByText("guides", { exact: true })).toBeVisible()
  await expect(tree.getByRole("treeitem", { name: "diagrams.md" })).toBeVisible()
  await expect(tree.getByRole("treeitem", { name: "README.md" })).toHaveCount(0)
  await expect(tabs.getByRole("tab")).toHaveCount(0)

  await tree.getByRole("treeitem", { name: "diagrams.md" }).click()
  await expect(page).toHaveURL(/\/view\?project=guides&path=diagrams\.md$/)
  await expect(tabs.getByRole("tab")).toHaveCount(1)
})

test("an unavailable project explains itself", async ({ page }) => {
  await page.goto("/?project=missing")
  await expect(page.getByRole("alert")).toContainText("The project missing is unavailable.")
  await page.getByRole("button", { name: "Choose another project" }).click()
  await expect(page.getByRole("dialog", { name: "Switch project" })).toBeVisible()
})

test("projects remember their tabs and the last project reopens", async ({ page }) => {
  const tree = page.getByRole("complementary", { name: "Files" })
  const tabs = page.getByRole("tablist", { name: "Open documents" })
  await page.goto("/view?project=.&path=guides%2Fgetting-started.md")
  await tree.getByRole("treeitem", { name: "README.md" }).click()
  await expect(tabs.getByRole("tab")).toHaveCount(2)

  await page.goto("/?project=guides")
  await expect(tabs.getByRole("tab")).toHaveCount(0)
  await tree.getByRole("treeitem", { name: "diagrams.md" }).click()
  await expect(tabs.getByRole("tab", { name: "diagrams.md" })).toHaveAttribute("aria-selected", "true")

  await page.goto("/")
  await expect(page).toHaveURL(/\/view\?project=guides&path=diagrams\.md$/)
  await expect(tabs.getByRole("tab")).toHaveCount(1)

  await page.goto("/?project=.")
  await expect(page).toHaveURL(/\/view\?project=\.&path=README\.md$/)
  await expect(tabs.getByRole("tab", { name: "getting-started.md" })).toBeVisible()
  await expect(tabs.getByRole("tab", { name: "README.md" })).toHaveAttribute("aria-selected", "true")
})
