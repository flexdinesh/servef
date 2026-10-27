import { writeFileSync } from "node:fs"
import path from "node:path"

import { expect, test, type Page } from "@playwright/test"

import { liveFixtureRoot, longDocument } from "./live-fixture.ts"
import { liveApiOrigin } from "./ports.ts"

// Live updates (decision 0050): the open project follows changes on disk.
const project = encodeURIComponent(liveFixtureRoot)

function liveIndicator(page: Page) {
  return page.locator(".status-live")
}

function explorer(page: Page) {
  return page.getByRole("complementary", { name: "Files" })
}

test("an edited document updates in place and keeps its scroll", async ({ page }) => {
  await page.goto(`${liveApiOrigin}/view?project=${project}&path=long.md#L5`)
  await expect(liveIndicator(page)).toHaveAttribute("data-mode", "watching")
  await expect(liveIndicator(page)).toHaveAttribute("title", /watching the project/)
  const pane = page.getByRole("main")
  await expect(pane.getByText("Line 300 of the long document.")).toBeAttached()
  // Scrolled away from the line anchor, a reread must not jump back to it.
  await pane.evaluate((element) => { element.scrollTop = 3000 })
  await expect.poll(() => pane.evaluate((element) => element.scrollTop)).toBe(3000)

  writeFileSync(path.join(liveFixtureRoot, "long.md"), longDocument("edited on disk"))
  await expect(pane.getByText("# Long edited on disk")).toBeAttached()
  expect(await pane.evaluate((element) => element.scrollTop)).toBe(3000)
})

test("created files appear in the tree and Source Control", async ({ page }) => {
  await page.goto(`${liveApiOrigin}/?project=${project}`)
  await expect(liveIndicator(page)).toHaveAttribute("data-mode", "watching")
  await expect(explorer(page).getByRole("treeitem", { name: "docs" })).toBeVisible()

  writeFileSync(path.join(liveFixtureRoot, "fresh.md"), "# Fresh\n")
  await expect(explorer(page).getByRole("treeitem", { name: "fresh.md" })).toBeVisible()

  await page.keyboard.press("ControlOrMeta+Shift+g")
  const uncommitted = page.getByRole("region", { name: "Source Control" }).getByRole("region", { name: "Uncommitted", exact: true })
  await expect(uncommitted.getByRole("option", { name: /^fresh\.md, / })).toBeVisible()
})
