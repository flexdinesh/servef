import { expect, test, type Page } from "@playwright/test"
import { rootParam } from "./fixture-root.ts"
import { filteredApiOrigin } from "./ports.ts"

function explorer(page: Page) {
  return page.getByRole("complementary", { name: "Files" })
}

function filterField(page: Page) {
  return explorer(page).getByRole("textbox", { name: "Filter files by name" })
}

test("filename globs narrow the tree without closing tabs and intersect filename search", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.json")
  const files = explorer(page)
  const filter = filterField(page)
  await expect(filter).toHaveAttribute("placeholder", "Filter: *.md, *.go")
  await expect(files.locator(".filename-filter-count")).toBeEmpty()
  await filter.fill("*.YAML, *.yml")
  await expect(files.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await expect(files.getByRole("treeitem", { name: "documents.yml" })).toBeVisible()
  await expect(files.getByRole("treeitem", { name: "settings.json" })).toHaveCount(0)
  await expect(files.getByRole("treeitem", { name: "README.md" })).toHaveCount(0)
  await expect(files.locator(".filename-filter-count > [aria-hidden]")).toHaveText(/^2 \/ \d+$/)
  await expect(files.locator(".filename-filter-count")).toHaveAttribute("role", "status")
  await expect(files.locator(".filename-filter-count")).toContainText(/2 of \d+ files match/)
  await expect(page.getByRole("tab", { name: "settings.json" })).toHaveAttribute("aria-selected", "true")

  await files.getByRole("button", { name: "Search files" }).click()
  const fileSearch = files.getByRole("textbox", { name: "Search…" })
  await fileSearch.fill("settings")
  await expect(files.getByRole("treeitem", { name: "documents.yml" })).toHaveCount(0)
  await expect(files.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await fileSearch.press("Escape")

  await page.getByRole("button", { name: /^Find in files/ }).click()
  await page.getByRole("textbox", { name: "Search", exact: true }).fill("config-search-marker")
  await expect(page.getByRole("tree", { name: "Search results" }).getByRole("treeitem").filter({ hasText: "settings.json" })).toBeVisible()
  await page.getByRole("button", { name: "Explorer" }).click()

  await files.getByRole("button", { name: "Clear filter" }).click()
  await expect(filter).toHaveValue("")
  await expect(filter).toBeFocused()
  await expect(files.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await expect(files.getByRole("button", { name: "Clear filter" })).toHaveCount(0)
})

test("invalid globs keep the last valid filter and unmatched globs offer a clear action", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.json")
  const files = explorer(page)
  const filter = filterField(page)
  await filter.fill("*.json")
  await expect(files.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await expect(files.getByRole("treeitem", { name: "README.md" })).toHaveCount(0)
  await filter.fill("*.json, [a-")
  await expect(filter).toHaveAttribute("aria-invalid", "true")
  await expect(files.getByText('Invalid pattern "[a-"')).toBeVisible()
  await expect(filter).toHaveAccessibleDescription('Invalid pattern "[a-"')
  await expect(files.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await expect(files.getByRole("treeitem", { name: "README.md" })).toHaveCount(0)

  await filter.fill("*.xyz")
  await expect(filter).not.toHaveAttribute("aria-invalid", "true")
  await expect(files.getByText("Invalid pattern")).toHaveCount(0)
  await expect(files).toContainText("No files match *.xyz")
  await expect(files.locator(".filename-filter-count > [aria-hidden]")).toHaveText(/^0 \/ \d+$/)
  await files.getByRole("button", { name: "Clear filter" }).last().click()
  await expect(filter).toHaveValue("")
  await expect(files.getByRole("treeitem", { name: "README.md" })).toBeVisible()
})

test("filter keyboard clears, blurs, and moves into the tree", async ({ page }) => {
  await page.goto("/")
  const files = explorer(page)
  const filter = filterField(page)
  await filter.fill("*.json")
  await filter.press("Escape")
  await expect(filter).toHaveValue("")
  await expect(filter).toBeFocused()
  await filter.press("Escape")
  await expect(filter).not.toBeFocused()

  await filter.fill("settings.*")
  await expect(files.getByRole("treeitem", { name: "README.md" })).toHaveCount(0)
  await filter.press("ArrowDown")
  await expect(files.getByRole("treeitem", { name: "config", exact: true })).toBeFocused()
  await page.keyboard.press("ArrowRight")
  await page.keyboard.press("ArrowDown")
  await expect(files.getByRole("treeitem", { name: "settings.json" })).toBeFocused()
  await filter.focus()
  await filter.press("Enter")
  await expect(files.getByRole("treeitem", { name: "config", exact: true })).toBeFocused()
})

test("collapse and expand all include filtered folders and the filter survives responsive remounts", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  const filter = filterField(page)
  await filter.fill("*.json")
  await expect(page.getByRole("treeitem", { name: "guides", exact: true })).toHaveCount(0)
  await page.getByRole("treeitem", { name: "config", exact: true }).click()
  await expect(page.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await page.getByRole("button", { name: "Collapse all folders" }).click()
  await expect(page.getByRole("treeitem", { name: "settings.json" })).toHaveCount(0)
  await expect(page.getByRole("treeitem", { name: "config", exact: true })).toHaveAttribute("aria-expanded", "false")
  await expect(page.getByRole("tab", { name: "getting-started.md" })).toHaveAttribute("aria-selected", "true")

  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "Open files" }).click()
  const drawer = page.getByRole("dialog", { name: "Files", exact: true })
  const drawerFilter = drawer.getByRole("textbox", { name: "Filter files by name" })
  await expect(drawerFilter).toHaveValue("*.json")
  expect(await drawer.locator(".filename-filter-field").evaluate((element) => element.getBoundingClientRect().height)).toBe(40)
  await drawer.getByRole("button", { name: "Expand all folders" }).click()
  await expect(drawer.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await expect(drawer.getByRole("treeitem", { name: "getting-started.md" })).toHaveCount(0)
  await expect(drawer.getByRole("button", { name: "Collapse all folders" })).toBeVisible()
  await drawerFilter.press("Escape")
  await expect(drawerFilter).toHaveValue("")
  await expect(drawer.getByRole("treeitem", { name: "guides", exact: true })).toHaveAttribute("aria-expanded", "true")
  await expect(drawer).toBeVisible()
  await expect(drawer.getByRole("treeitem", { name: "getting-started.md" })).toBeVisible()
  await drawerFilter.fill("settings.*")
  await drawer.getByRole("treeitem", { name: "settings.yaml" }).click()
  await expect(drawer).toBeHidden()
  await expect(page.getByRole("tab", { name: "settings.yaml" })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("tab", { name: "getting-started.md" })).toBeVisible()
})

test("narrowed CLI filename scope is described under the filter", async ({ page }) => {
  await page.route("**/api/page**", async (route) => {
    const response = await route.fetch()
    const json: unknown = await response.json()
    if (typeof json !== "object" || json === null) throw new Error("unexpected page payload")
    await route.fulfill({ response, json: { ...json, filenamePatterns: ["*.md", "*.go"] } })
  })
  await page.goto("/")
  const filter = filterField(page)
  await expect(explorer(page).getByText("Serving *.md, *.go")).toBeVisible()
  await expect(filter).toHaveAccessibleDescription("Serving *.md, *.go")
})

test("default CLI filename scope shows no scope line", async ({ page }) => {
  await page.goto("/")
  await expect(filterField(page)).toBeVisible()
  await expect(explorer(page).getByText(/^Serving /)).toHaveCount(0)
})

test("CLI filename scope reaches the page contract and prevents opening excluded documents", async ({ page, request }) => {
  const response = await request.get(`${filteredApiOrigin}/api/page?project=${rootParam}`)
  expect(await response.json()).toMatchObject({ filenamePatterns: ["*.md"] })
  await page.goto(`${filteredApiOrigin}/view?path=config%2Fsettings.json`)
  await expect(page.getByRole("alert")).toContainText("That file is unavailable")
})

test("tabs show colored Pierre file icons from one hidden sprite", async ({ page }) => {
  await page.goto("/view?path=README.md")
  await explorer(page).getByRole("treeitem", { name: "config", exact: true }).click()
  await explorer(page).getByRole("treeitem", { name: "settings.json" }).click()
  await expect(page.getByRole("tab", { name: "settings.json" })).toHaveAttribute("aria-selected", "true")
  const markdown = page.getByRole("tab", { name: "README.md" }).locator("svg.file-icon")
  const json = page.getByRole("tab", { name: "settings.json" }).locator("svg.file-icon")
  await expect(markdown).toHaveAttribute("data-icon-token", "markdown")
  await expect(markdown).toHaveAttribute("aria-hidden", "true")
  await expect(json).toHaveAttribute("data-icon-token", "json")
  await expect(json.locator("use")).toHaveAttribute("href", "#file-tree-builtin-json")
  await expect(json).toHaveCSS("width", "12px")
  await expect(json).toHaveCSS("height", "12px")
  const sprite = await page.evaluate(() => ({
    count: document.querySelectorAll(".file-icon-sprite").length,
    symbol: document.getElementById("file-tree-builtin-json") instanceof SVGSymbolElement,
  }))
  expect(sprite).toEqual({ count: 1, symbol: true })
  const tokenColor = (token: string) => page.evaluate((name) => {
    const probe = document.createElement("span")
    probe.style.color = `var(${name})`
    document.body.append(probe)
    const color = getComputedStyle(probe).color
    probe.remove()
    return color
  }, token)
  await expect(markdown).toHaveCSS("color", await tokenColor("--color-success"))
  await expect(json).toHaveCSS("color", await tokenColor("--color-warning"))
})
