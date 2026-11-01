import { writeFileSync } from "node:fs"
import path from "node:path"

import { expect, type Page } from "@playwright/test"

import { longDocument } from "./live-fixture.ts"
import { test } from "./live-test.ts"

// Live updates (decision 0050): the open project follows changes on disk.

function liveIndicator(page: Page) {
  return page.locator(".status-live")
}

function explorer(page: Page) {
  return page.getByRole("complementary", { name: "Files" })
}

test("an edited document updates in place and keeps its scroll", async ({ page, live }) => {
  const { root: liveFixtureRoot, origin: liveApiOrigin } = live
  const project = encodeURIComponent(liveFixtureRoot)
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

test("created files appear in the tree and Source Control", async ({ page, live }) => {
  const { root: liveFixtureRoot, origin: liveApiOrigin } = live
  const project = encodeURIComponent(liveFixtureRoot)
  await page.goto(`${liveApiOrigin}/?project=${project}`)
  await expect(liveIndicator(page)).toHaveAttribute("data-mode", "watching")
  await expect(explorer(page).getByRole("treeitem", { name: "docs" })).toBeVisible()

  writeFileSync(path.join(liveFixtureRoot, "fresh.md"), "# Fresh\n")
  await expect(explorer(page).getByRole("treeitem", { name: "fresh.md" })).toBeVisible()

  await page.keyboard.press("ControlOrMeta+Shift+g")
  const uncommitted = page.getByRole("region", { name: "Source Control" }).getByRole("region", { name: "Uncommitted", exact: true })
  await expect(uncommitted.getByRole("option", { name: /^fresh\.md, / })).toBeVisible()
})

test("a branch review with uncommitted changes lists files created on disk", async ({ page, live }) => {
  const { root: liveFixtureRoot, origin: liveApiOrigin } = live
  const project = encodeURIComponent(liveFixtureRoot)
  // With nothing ahead of main, the branch's merge base is HEAD (decision 0051).
  writeFileSync(path.join(liveFixtureRoot, "scoped.md"), "# Scoped\n")
  await page.goto(`${liveApiOrigin}/view?project=${project}&review=branch&uncommitted=1`)
  await expect(liveIndicator(page)).toHaveAttribute("data-mode", "watching")
  const main = page.getByRole("main")
  await expect(main.getByRole("region", { name: "scoped.md", exact: true })).toBeVisible()

  writeFileSync(path.join(liveFixtureRoot, "later.md"), "# Later\n")
  await expect(main.getByRole("region", { name: "later.md", exact: true })).toBeVisible()
})
