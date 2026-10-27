import { expect, test } from "@playwright/test"

import { rootParam } from "./fixture-root.ts"

const documents = [
  "guides/getting-started.md", "guides/code-blocks.md", "guides/diagrams.md", "guides/images.md",
  "code/app.js", "code/main.go", "code/styles.css", "notes.txt",
]

test("the active tab scrolls into view in an overflowing strip", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`/view?project=${rootParam}&path=${encodeURIComponent(documents[0] ?? "")}`)
  // Open the rest from the drawer: one page load keeps this fast under load.
  for (const path of documents.slice(1)) {
    await page.getByRole("button", { name: "Open files" }).click()
    const drawer = page.getByRole("dialog", { name: "Files", exact: true })
    const parts = path.split("/")
    for (const folder of parts.slice(0, -1)) {
      const item = drawer.getByRole("treeitem", { name: folder, exact: true })
      if (await item.getAttribute("aria-expanded") === "false") await item.click()
    }
    await drawer.getByRole("treeitem", { name: parts.at(-1), exact: true }).click()
    await expect(page.getByRole("tab", { selected: true })).toHaveAttribute("title", new RegExp(path))
  }
  const strip = page.getByRole("tablist", { name: "Open documents" })
  const visible = async (name: string) => {
    const [tab, bounds] = await Promise.all([page.getByRole("tab", { name, exact: true }).boundingBox(), strip.boundingBox()])
    return tab !== null && bounds !== null && tab.x >= bounds.x - 1 && tab.x + tab.width <= bounds.x + bounds.width + 1
  }
  await expect.poll(() => visible("notes.txt")).toBe(true)
  await page.getByRole("button", { name: "Open files" }).click()
  await page.getByRole("dialog", { name: "Files", exact: true }).getByRole("treeitem", { name: "getting-started.md", exact: true }).click()
  await expect.poll(() => visible("getting-started.md")).toBe(true)
})
