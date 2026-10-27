import { expect, test } from "@playwright/test"

import { endsWith, fixtureRoot, projectParam, rootParam } from "./fixture-root.ts"

test("folder expansion survives reloads and stays independent per project", async ({ context, page: firstPage }) => {
  let tree = firstPage.getByRole("complementary", { name: "Files" })
  await firstPage.goto(`/?project=${rootParam}`)
  await tree.getByRole("treeitem", { name: "config", exact: true }).click()
  await expect(tree.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await firstPage.reload()
  await expect(tree.getByRole("treeitem", { name: "config", exact: true })).toHaveAttribute("aria-expanded", "true")
  await expect(tree.getByRole("treeitem", { name: "guides", exact: true })).toHaveAttribute("aria-expanded", "false")

  await firstPage.goto(`/?project=${projectParam("reference")}`)
  await tree.getByRole("treeitem", { name: "topics", exact: true }).click()
  await expect(tree.getByRole("treeitem", { name: "search.md" })).toBeVisible()

  // Each Vite dev load fetches about 200 modules, and Chromium fails a page's
  // requests with ERR_INSUFFICIENT_RESOURCES after about 1,000. Continue in a
  // new tab; it shares localStorage, where the expansion is remembered.
  const page = await context.newPage()
  await firstPage.close()
  tree = page.getByRole("complementary", { name: "Files" })
  await page.goto(`/?project=${rootParam}`)
  await expect(tree.getByRole("treeitem", { name: "config", exact: true })).toHaveAttribute("aria-expanded", "true")
  await tree.getByRole("button", { name: "Collapse all folders" }).click()
  await page.reload()
  await expect(tree.getByRole("button", { name: "Expand all folders" })).toBeVisible()
  await expect(tree.getByRole("treeitem", { name: "config", exact: true })).toHaveAttribute("aria-expanded", "false")
  await page.goto(`/?project=${projectParam("reference")}`)
  await expect(tree.getByRole("treeitem", { name: "topics", exact: true })).toHaveAttribute("aria-expanded", "true")
})

test("all-collapsed expansion survives reload with an active document", async ({ page }) => {
  await page.goto(`/view?project=${rootParam}&path=guides%2Fgetting-started.md`)
  const tree = page.getByRole("complementary", { name: "Files" })
  await tree.getByRole("button", { name: "Collapse all folders" }).click()
  await page.reload()
  await expect(tree.getByRole("treeitem", { name: "guides", exact: true })).toHaveAttribute("aria-expanded", "false")
  await expect(tree.getByRole("button", { name: "Expand all folders" })).toBeVisible()
  await expect(page.getByRole("tab", { name: "getting-started.md" })).toHaveAttribute("aria-selected", "true")
})

test("filters, search, and responsive remounts preserve remembered folder expansion", async ({ page }) => {
  await page.goto(`/?project=${rootParam}`)
  const tree = page.getByRole("complementary", { name: "Files" })
  await tree.getByRole("treeitem", { name: "guides", exact: true }).click()
  const filter = tree.getByRole("textbox", { name: "Filter files by name" })
  await filter.fill("*.json")
  await tree.getByRole("treeitem", { name: "config", exact: true }).click()
  await tree.getByRole("button", { name: "Search files" }).click()
  await tree.getByRole("textbox", { name: "Search…" }).fill("settings")

  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "Open files" }).click()
  const drawer = page.getByRole("dialog", { name: "Files", exact: true })
  await expect(drawer.getByRole("textbox", { name: "Filter files by name" })).toHaveValue("*.json")
  await expect(drawer.getByRole("textbox", { name: "Search…" })).toHaveValue("settings")
  await page.reload()
  await page.getByRole("button", { name: "Open files" }).click()
  await expect(drawer.getByRole("textbox", { name: "Filter files by name" })).toHaveValue("")
  await expect(drawer.getByRole("textbox", { name: "Search…" })).toBeHidden()
  await expect(drawer.getByRole("treeitem", { name: "guides", exact: true })).toHaveAttribute("aria-expanded", "true")
  await expect(drawer.getByRole("treeitem", { name: "config", exact: true })).toHaveAttribute("aria-expanded", "true")
})

test("restoring tabs never writes an empty workspace before navigation finishes", async ({ page }) => {
  await page.goto(`/?project=${rootParam}`)
  await page.evaluate((project) => {
    window.localStorage.setItem("servef-workspaces-v2", JSON.stringify({ last: {}, projects: {
      [project]: { tabs: ["README.md", "config/settings.json", "removed.md"], active: "config/settings.json", expanded: ["config/", "removed/", "../outside/", "README.md"] },
    } }))
  }, fixtureRoot)
  await page.addInitScript(() => {
    const original = Storage.prototype.setItem
    Storage.prototype.setItem = function (key, value) {
      if (key === "servef-workspaces-v2") {
        const writes: unknown = JSON.parse(document.documentElement.dataset.workspaceWrites ?? "[]")
        document.documentElement.dataset.workspaceWrites = JSON.stringify([...(Array.isArray(writes) ? writes : []), JSON.parse(value)])
      }
      original.call(this, key, value)
    }
  })
  // The first visit routed Welcome; land on the project home again.
  await page.goto(`/?project=${rootParam}`)
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&path=config%2Fsettings.json`))
  const tabs = page.getByRole("tablist", { name: "Open documents" })
  await expect(tabs.getByRole("tab")).toHaveCount(2)
  await expect(tabs.getByRole("tab", { name: "settings.json" })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("treeitem", { name: "config", exact: true })).toHaveAttribute("aria-expanded", "true")
  const writes: unknown = await page.evaluate(() => JSON.parse(document.documentElement.dataset.workspaceWrites ?? "[]"))
  expect(writes).toEqual(expect.arrayContaining([expect.objectContaining({ projects: expect.objectContaining({
    [fixtureRoot]: { tabs: ["README.md", "config/settings.json"], active: "config/settings.json", expanded: ["config/"] },
  }) })]))
  expect(writes).not.toEqual(expect.arrayContaining([expect.objectContaining({ projects: expect.objectContaining({
    [fixtureRoot]: expect.objectContaining({ tabs: [] }),
  }) })]))
})
