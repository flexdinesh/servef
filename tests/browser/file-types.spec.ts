import { expect, test } from "@playwright/test"

for (const origin of ["", "http://127.0.0.1:18080"]) {
  test(`JSON formatting and source preserve values (${origin || "development"})`, async ({ page }) => {
    await page.goto(`${origin}/view?path=config%2Fsettings.json`)
    const formatted = page.getByRole("button", { name: "Formatted", exact: true })
    await expect(formatted).toHaveAttribute("aria-pressed", "true")
    await expect(page.locator("article pre code")).toContainText('"large": 900719925474099312345')
    await expect(page.locator("article .syntax-s2").first()).toBeVisible()
    await page.getByRole("button", { name: "Source", exact: true }).click()
    const source = page.getByRole("textbox", { name: "JSON source" })
    await expect(source.locator(".source-document-line")).toHaveCount(1)
    await expect(source).toContainText('"large":900719925474099312345')
    await expect(source.locator("[class^=syntax-]").first()).toBeVisible()
    await formatted.click()
    await expect(page.locator("article pre code")).toContainText('"enabled": true')
  })
}

test("YAML preview retains comments, aliases, and multiple documents", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.yaml")
  const preview = page.locator("article pre code")
  await expect(preview).toContainText("# Shared configuration")
  await expect(preview).toContainText("&base")
  await expect(preview).toContainText("*base")
  await expect(preview).toContainText("# keep this comment")
  await expect(preview).toContainText("script: |")
  await page.getByRole("button", { name: "Source", exact: true }).click()
  await expect(page.getByRole("textbox", { name: "YAML source" })).toContainText("    name:")
  await page.goto("/view?path=config%2Fdocuments.yml")
  await expect(page.locator("article pre code")).toContainText("first-document\n---\nname: second-document")
})

test("type filtering preserves open documents, expansion, filename search, and global search scope", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.json")
  const explorer = page.getByRole("complementary", { name: "Files" })
  const filter = explorer.getByRole("combobox", { name: "Filter file types" })
  await filter.selectOption("yaml")
  await expect(explorer.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await expect(explorer.getByRole("treeitem", { name: "settings.json" })).toHaveCount(0)
  await expect(page.locator("article pre code")).toContainText("servef-json-fixture")
  await expect(page.getByRole("tab", { name: "settings.json" })).toBeVisible()
  await explorer.getByRole("button", { name: "Search files" }).click()
  const fileSearch = explorer.getByRole("textbox", { name: "Search…" })
  await fileSearch.fill("settings")
  await expect(explorer.getByRole("treeitem", { name: "documents.yml" })).toHaveCount(0)
  await expect(explorer.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await fileSearch.press("Escape")
  await filter.selectOption("")
  await expect(explorer.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await filter.selectOption("yaml")
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
  const filter = page.getByRole("combobox", { name: "Filter file types" })
  await expect(filter).toBeDisabled()
  await expect(filter).toHaveValue("markdown")
  await expect(page.locator(".file-type-filter")).toContainText("Set by CLI")
  await page.goto("http://127.0.0.1:18082/view?path=config%2Fsettings.json")
  await expect(page.getByRole("alert")).toContainText("That file is unavailable")
})

test("mobile type filter keeps selected document open", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto("/view?path=config%2Fsettings.json")
  await page.getByRole("button", { name: "Open files" }).click()
  const drawer = page.getByRole("dialog", { name: "Files", exact: true })
  await drawer.getByRole("combobox", { name: "Filter file types" }).selectOption("yaml")
  await expect(drawer.getByRole("treeitem", { name: "settings.json" })).toHaveCount(0)
  await drawer.getByRole("treeitem", { name: "settings.yaml" }).click()
  await expect(drawer).toBeHidden()
  await expect(page.locator("article pre code")).toContainText("servef-yaml-fixture")
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
})

test("filter and hidden directory expansion survive responsive remounts", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  const filter = page.getByRole("combobox", { name: "Filter file types" })
  await filter.selectOption("json")
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "Open files" }).click()
  await expect(filter).toHaveValue("json")
  await filter.selectOption("")
  await expect(page.getByRole("treeitem", { name: "getting-started.md" })).toBeVisible()
  await expect(page.getByRole("tab", { name: "getting-started.md" })).toBeVisible()
})
