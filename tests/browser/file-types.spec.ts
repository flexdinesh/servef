import { expect, test, type Page } from "@playwright/test"

async function selectTypes(page: Page, labels: readonly string[]) {
  await page.getByRole("button", { name: "Filter file types" }).click()
  const menu = page.getByRole("menu", { name: "File types" })
  const clear = menu.getByRole("menuitem", { name: "Show all types" })
  if (await clear.isEnabled()) await clear.click()
  for (const label of labels) {
    await menu.getByRole("menuitemcheckbox").filter({ has: page.getByText(label, { exact: true }) }).click()
  }
  await menu.press("Escape")
}

async function selectAllTypes(page: Page) {
  await page.getByRole("button", { name: "Filter file types" }).click()
  const menu = page.getByRole("menu", { name: "File types" })
  await menu.getByRole("menuitem", { name: "Show all types" }).click()
  await menu.press("Escape")
}

for (const origin of ["", "http://127.0.0.1:18080"]) {
  test(`JSON formatting and source preserve values (${origin || "development"})`, async ({ page }) => {
    await page.goto(`${origin}/view?path=config%2Fsettings.json`)
    const formatted = page.getByRole("button", { name: "Formatted", exact: true })
    await expect(formatted).toHaveAttribute("aria-pressed", "true")
    const formattedCode = page.getByRole("textbox", { name: "JSON formatted" })
    await expect(formattedCode).toContainText('"large": 900719925474099312345')
    await expect(formattedCode.locator(".syntax-s2").first()).toBeVisible()
    await expect(formattedCode.locator(".source-document-line")).toHaveCount(8)
    await expect(page.locator("article")).toHaveCount(0)
    await expect(page.locator("#document-pane")).toHaveClass("source-mode")
    await page.getByRole("button", { name: "Source", exact: true }).click()
    const source = page.getByRole("textbox", { name: "JSON source" })
    await expect(source.locator(".source-document-line")).toHaveCount(1)
    await expect(source).toContainText('"large":900719925474099312345')
    await expect(source.locator("[class^=syntax-]").first()).toBeVisible()
    await formatted.click()
    await expect(formattedCode).toContainText('"enabled": true')
  })
}

test("YAML formatting retains comments, aliases, and multiple documents", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.yaml")
  const preview = page.getByRole("textbox", { name: "YAML formatted" })
  await expect(preview).toContainText("# Shared configuration")
  await expect(preview).toContainText("&base")
  await expect(preview).toContainText("*base")
  await expect(preview).toContainText("# keep this comment")
  await expect(preview).toContainText("script: |")
  await expect(preview.locator(".source-document-line-content").nth(2)).toContainText("  name:")
  await page.getByRole("button", { name: "Source", exact: true }).click()
  await expect(page.getByRole("textbox", { name: "YAML source" })).toContainText("    name:")
  await page.goto("/view?path=config%2Fdocuments.yml")
  await expect(preview.locator(".source-document-line-content")).toHaveText([
    "name: first-document", "---", "name: second-document",
  ])
})

test("type filtering preserves open documents, expansion, filename search, and global search scope", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.json")
  const explorer = page.getByRole("complementary", { name: "Files" })
  await selectTypes(page, ["YAML"])
  await expect(explorer.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await expect(explorer.getByRole("treeitem", { name: "settings.json" })).toHaveCount(0)
  await expect(page.getByRole("textbox", { name: "JSON formatted" })).toContainText("servef-json-fixture")
  await expect(page.getByRole("tab", { name: "settings.json" })).toBeVisible()
  await explorer.getByRole("button", { name: "Search files" }).click()
  const fileSearch = explorer.getByRole("textbox", { name: "Search…" })
  await fileSearch.fill("settings")
  await expect(explorer.getByRole("treeitem", { name: "documents.yml" })).toHaveCount(0)
  await expect(explorer.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await fileSearch.press("Escape")
  await selectAllTypes(page)
  await expect(explorer.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await selectTypes(page, ["YAML"])
  await page.getByRole("button", { name: /^Find in files/ }).click()
  const search = page.getByPlaceholder("Search document content…")
  await search.fill("config-search-marker")
  await expect(page.getByRole("option").filter({ hasText: "settings.json" })).toBeVisible()
})

test("malformed JSON remains readable and searchable", async ({ page }) => {
  await page.goto("/view?path=config%2Fbroken.json")
  await expect(page.getByRole("alert")).toContainText("Switch to Source")
  await page.getByRole("button", { name: "Source", exact: true }).click()
  await expect(page.getByRole("textbox", { name: "JSON source" })).toContainText("malformed-search-marker")
  await page.getByRole("button", { name: /^Find in files/ }).click()
  await page.getByPlaceholder("Search document content…").fill("malformed-search-marker")
  await expect(page.getByRole("option").filter({ hasText: "broken.json" })).toBeVisible()
})

test("CLI scope disables single-type filter and prevents opening excluded documents", async ({ page }) => {
  await page.goto("http://127.0.0.1:18082/")
  const filter = page.getByRole("button", { name: "Filter file types" })
  await expect(filter).toBeDisabled()
  await expect(filter).toContainText("Markdown")
  await expect(page.locator(".file-type-filter")).toContainText("Set by CLI")
  await page.goto("http://127.0.0.1:18082/view?path=config%2Fsettings.json")
  await expect(page.getByRole("alert")).toContainText("That file is unavailable")
})

test("mobile type filter keeps selected document open", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto("/view?path=config%2Fsettings.json")
  await page.getByRole("button", { name: "Open files" }).click()
  const drawer = page.getByRole("dialog", { name: "Files", exact: true })
  await selectTypes(page, ["YAML"])
  await expect(drawer.getByRole("treeitem", { name: "settings.json" })).toHaveCount(0)
  await drawer.getByRole("treeitem", { name: "settings.yaml" }).click()
  await expect(drawer).toBeHidden()
  await expect(page.getByRole("textbox", { name: "YAML formatted" })).toContainText("servef-yaml-fixture")
})

test("source-only capabilities drive the UI without extension-specific components", async ({ page }) => {
  await page.route("**/api/page?path=main.go", async (route) => {
    const fileType = {
      id: "go", label: "Go", extensions: [".go"], language: "go",
      previewKind: "", previewLabel: "", defaultMode: "source",
    }
    await route.fulfill({ json: {
      content: "", source: "package main\n", sourceLines: ['<span class="syntax-kn">package</span> main'],
      empty: false, error: "", fileCount: 1, fileSize: 13, hasFile: true, rootName: "example", selected: "main.go",
      warnings: [], previewError: "", fileTypes: [fileType], documentType: fileType,
      tree: [{ name: "main.go", path: "main.go", fileType: "go", isDir: false, open: false, selected: true, children: [] }],
    } })
  })
  await page.goto("/view?path=main.go")
  await expect(page.getByRole("textbox", { name: "Go source" })).toContainText("package main")
  await expect(page.getByRole("group", { name: "Document view" })).toHaveCount(0)
  await expect(page.locator("article")).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Expand all folders" })).toBeDisabled()
})

test("filter and hidden directory expansion survive responsive remounts", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  const filter = page.getByRole("button", { name: "Filter file types" })
  await selectTypes(page, ["JSON"])
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "Open files" }).click()
  await expect(filter).toContainText("JSON")
  await selectAllTypes(page)
  await expect(page.getByRole("treeitem", { name: "getting-started.md" })).toBeVisible()
  await expect(page.getByRole("tab", { name: "getting-started.md" })).toBeVisible()
})

test("type menu supports multiple selections, no selection shows all, and keyboard dismissal", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.json")
  await selectTypes(page, ["JSON", "YAML"])
  const filter = page.getByRole("button", { name: "Filter file types" })
  await expect(filter).toContainText("2 types")
  await expect(filter).toHaveAttribute("title", "JSON, YAML")
  await expect(page.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await expect(page.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await expect(page.getByRole("treeitem", { name: "guides" })).toHaveCount(0)
  await expect(filter).toBeFocused()
  await filter.press("Enter")
  const menu = page.getByRole("menu", { name: "File types" })
  await menu.getByRole("menuitemcheckbox", { name: /^JSON/ }).click()
  await expect(page.getByRole("treeitem", { name: "settings.json" })).toHaveCount(0)
  await menu.getByRole("menuitemcheckbox", { name: /^YAML/ }).click()
  await expect(menu).toBeVisible()
  await expect(page.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await expect(page.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await expect(menu.getByRole("menuitemcheckbox", { checked: true })).toHaveCount(0)
  await expect(menu.getByRole("menuitem", { name: "Show all types" })).toBeDisabled()
  await expect(page.getByRole("tab", { name: "settings.json" })).toBeVisible()
  await menu.press("Escape")
  await expect(filter).toContainText("All types")
  await expect(filter).toBeFocused()
  await expect(page.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
})

test("collapse and expand all preserve document, hidden folders, and responsive state", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  await page.getByRole("button", { name: "Collapse all folders" }).click()
  await expect(page.getByRole("treeitem", { name: "getting-started.md" })).toHaveCount(0)
  await expect(page.getByRole("tab", { name: "getting-started.md" })).toBeVisible()
  await selectTypes(page, ["JSON"])
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "Open files" }).click()
  await expect(page.getByRole("button", { name: "Expand all folders" })).toBeVisible()
  await selectAllTypes(page)
  await expect(page.getByRole("treeitem", { name: "guides" })).toHaveAttribute("aria-expanded", "false")
  await expect(page.getByRole("treeitem", { name: "getting-started.md" })).toHaveCount(0)
  await page.getByRole("button", { name: "Expand all folders" }).click()
  await expect(page.getByRole("treeitem", { name: "getting-started.md" })).toBeVisible()
  await page.getByRole("button", { name: "Search files" }).click()
  const search = page.getByRole("textbox", { name: "Search…" })
  await search.fill("settings")
  const collapse = page.getByRole("button", { name: "Collapse all folders" })
  await expect(collapse).toBeDisabled()
  await expect(collapse).toHaveAttribute("title", "Clear filename search to collapse or expand folders")
  await selectTypes(page, ["JSON", "YAML"])
  await expect(search).toHaveValue("settings")
  await expect(page.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await expect(page.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await search.fill("")
  await expect(collapse).toBeEnabled()
  await collapse.click()
  await page.setViewportSize({ width: 1280, height: 720 })
  await expect(page.getByRole("button", { name: "Expand all folders" })).toBeVisible()
  await expect(page.getByRole("treeitem", { name: "config" })).toHaveAttribute("aria-expanded", "false")
})

test("switching nested tabs after collapse updates the collapse action and persists expansion", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  await page.getByRole("button", { name: "Collapse all folders" }).click()
  await page.getByRole("button", { name: "Expand all folders" }).click()
  await page.getByRole("treeitem", { name: "settings.json" }).click()
  await expect(page.getByRole("tab", { name: "settings.json" })).toHaveAttribute("aria-selected", "true")
  await page.getByRole("button", { name: "Collapse all folders" }).click()
  await page.getByRole("tab", { name: "getting-started.md" }).click()
  await expect(page.getByRole("treeitem", { name: "guides" })).toHaveAttribute("aria-expanded", "true")
  await expect(page.getByRole("treeitem", { name: "config" })).toHaveAttribute("aria-expanded", "false")
  await expect(page.getByRole("button", { name: "Collapse all folders" })).toBeVisible()
  await selectTypes(page, ["JSON"])
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "Open files" }).click()
  await selectAllTypes(page)
  await expect(page.getByRole("treeitem", { name: "getting-started.md" })).toBeVisible()
  await page.getByRole("button", { name: "Collapse all folders" }).click()
  await expect(page.getByRole("button", { name: "Expand all folders" })).toBeVisible()
  await expect(page.getByRole("treeitem", { name: "guides" })).toHaveAttribute("aria-expanded", "false")
})
