import { expect, test } from "@playwright/test"

import { rootParam } from "./fixture-root.ts"

const documents = [
  "guides/getting-started.md", "guides/code-blocks.md", "guides/diagrams.md", "guides/images.md",
  "code/app.js", "code/main.go", "code/styles.css", "notes.txt",
]

test("the active tab scrolls into view in an overflowing strip", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  for (const path of documents) {
    await page.goto(`/view?project=${rootParam}&path=${encodeURIComponent(path)}`)
    await expect(page.getByRole("tab", { selected: true })).toHaveAttribute("title", new RegExp(path))
  }
  const strip = page.getByRole("tablist", { name: "Open documents" })
  const visible = async (name: string) => {
    const [tab, bounds] = await Promise.all([page.getByRole("tab", { name, exact: true }).boundingBox(), strip.boundingBox()])
    return tab !== null && bounds !== null && tab.x >= bounds.x - 1 && tab.x + tab.width <= bounds.x + bounds.width + 1
  }
  await expect.poll(() => visible("notes.txt")).toBe(true)
  await page.goto(`/view?project=${rootParam}&path=${encodeURIComponent(documents[0] ?? "")}`)
  await expect.poll(() => visible("getting-started.md")).toBe(true)
})

test("opening the drawer by shortcut focuses the revealed view's activity item", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`/?project=${rootParam}`)
  await page.locator("body").press("ControlOrMeta+Shift+G")
  const drawer = page.getByRole("dialog", { name: "Source Control" })
  await expect(drawer).toBeVisible()
  await expect(drawer.getByRole("button", { name: "Source Control", exact: true })).toBeFocused()
})
