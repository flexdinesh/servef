import { expect, test, type Page } from "@playwright/test"
import { fixtureRoot, rootParam } from "./fixture-root.ts"
import { apiOrigin } from "./ports.ts"

async function filterFiles(page: Page, value: string) {
  await page.getByRole("textbox", { name: "Filter files by name" }).fill(value)
}

for (const origin of ["", apiOrigin]) {
  test(`JSON opens as Source and formats on demand, preserving values (${origin || "development"})`, async ({ page }) => {
    let formatRequests = 0
    page.on("request", (request) => {
      if (new URL(request.url()).pathname === "/api/format") formatRequests++
    })
    await page.goto(`${origin}/view?path=config%2Fsettings.json`)
    const view = page.getByRole("group", { name: "Document view" })
    await expect(view.getByRole("button")).toHaveText(["Source", "Format"])
    const sourceButton = view.getByRole("button", { name: "Source", exact: true })
    const formatButton = view.getByRole("button", { name: "Format", exact: true })
    await expect(sourceButton).toHaveAttribute("aria-pressed", "true")
    const source = page.getByRole("textbox", { name: "JSON source" })
    await expect(source.locator(".source-document-line")).toHaveCount(1)
    await expect(source).toContainText('"large":900719925474099312345')
    await expect(source.locator("[class^=hl-]").first()).toBeVisible()
    expect(formatRequests).toBe(0)

    await formatButton.click()
    const formatted = page.getByRole("textbox", { name: "JSON formatted" })
    await expect(formatted).toContainText('"large": 900719925474099312345')
    await expect(formatted.locator(".hl-string").first()).toBeVisible()
    await expect(formatted.locator(".source-document-line")).toHaveCount(8)
    await expect(formatButton).toHaveAttribute("aria-pressed", "true")
    await expect(page.locator("article")).toHaveCount(0)
    await expect(page.locator("#document-pane")).toHaveClass("source-mode")

    await sourceButton.click()
    await expect(source).toContainText('"large":900719925474099312345')
    await formatButton.click()
    await expect(formatted).toContainText('"enabled": true')
    expect(formatRequests).toBe(1)
  })
}

test("HTML, CSS, and JavaScript format on demand without changing tokens", async ({ page }) => {
  for (const [path, label, line] of [
    ["code/index.html", "HTML", '<p>Format reindents blocks; <a href="#top" title="html-search-token">links</a> stay inline.</p>'],
    ["code/styles.css", "CSS", "margin: 0 auto;"],
    ["code/app.js", "JavaScript", 'console.warn("no items")'],
  ]) {
    await page.goto(`/view?path=${encodeURIComponent(path)}`)
    await expect(page.getByRole("textbox", { name: `${label} source` })).toBeVisible()
    await page.getByRole("button", { name: "Format", exact: true }).click()
    const formatted = page.getByRole("textbox", { name: `${label} formatted` })
    await expect(formatted.locator(".source-document-line-content").filter({ hasText: line })).toHaveCount(1)
  }
})

test("YAML formatting retains comments, aliases, and multiple documents", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.yaml")
  await expect(page.getByRole("textbox", { name: "YAML source" })).toContainText("    name:")
  await page.getByRole("button", { name: "Format", exact: true }).click()
  const formatted = page.getByRole("textbox", { name: "YAML formatted" })
  await expect(formatted).toContainText("# Shared configuration")
  await expect(formatted).toContainText("&base")
  await expect(formatted).toContainText("*base")
  await expect(formatted).toContainText("# keep this comment")
  await expect(formatted).toContainText("script: |")
  await expect(formatted.locator(".source-document-line-content").nth(2)).toContainText("  name:")
  await page.getByRole("button", { name: "Source", exact: true }).click()
  await expect(page.getByRole("textbox", { name: "YAML source" })).toContainText("    name:")
  await page.goto("/view?path=config%2Fdocuments.yml")
  await page.getByRole("button", { name: "Format", exact: true }).click()
  await expect(formatted.locator(".source-document-line-content")).toHaveText([
    "name: first-document", "---", "name: second-document",
  ])
})

test("filename filtering preserves open documents, expansion, filename search, and global search scope", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.json")
  const explorer = page.getByRole("complementary", { name: "Files" })
  await filterFiles(page, "*.yaml")
  await expect(explorer.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await expect(explorer.getByRole("treeitem", { name: "settings.json" })).toHaveCount(0)
  await expect(page.getByRole("textbox", { name: "JSON source" })).toContainText("servef-json-fixture")
  await expect(page.getByRole("tab", { name: "settings.json" })).toBeVisible()
  await explorer.getByRole("button", { name: "Search files" }).click()
  const fileSearch = explorer.getByRole("textbox", { name: "Search…" })
  await fileSearch.fill("settings")
  await expect(explorer.getByRole("treeitem", { name: "documents.yml" })).toHaveCount(0)
  await expect(explorer.getByRole("treeitem", { name: "settings.yaml" })).toBeVisible()
  await fileSearch.press("Escape")
  await filterFiles(page, "")
  await expect(explorer.getByRole("treeitem", { name: "settings.json" })).toBeVisible()
  await filterFiles(page, "*.yaml")
  await page.getByRole("button", { name: /^Find in files/ }).click()
  const search = page.getByRole("textbox", { name: "Search", exact: true })
  await search.fill("config-search-marker")
  await expect(page.getByRole("tree", { name: "Search results" }).getByRole("treeitem").filter({ hasText: "settings.json" })).toBeVisible()
})

test("malformed JSON keeps Source, explains the format failure, and stays searchable", async ({ page }) => {
  await page.goto("/view?path=config%2Fbroken.json")
  const formatButton = page.getByRole("button", { name: "Format", exact: true })
  await formatButton.click()
  await expect(page.getByRole("alert")).toHaveText(/^Couldn't format: .+\.$/)
  await expect(formatButton).toHaveAttribute("aria-pressed", "false")
  await expect(page.getByRole("button", { name: "Source", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(page.getByRole("textbox", { name: "JSON source" })).toContainText("malformed-search-marker")
  await expect(page.getByRole("textbox", { name: "JSON formatted" })).toHaveCount(0)
  await page.getByRole("button", { name: /^Find in files/ }).click()
  await page.getByRole("textbox", { name: "Search", exact: true }).fill("malformed-search-marker")
  await expect(page.getByRole("tree", { name: "Search results" }).getByRole("treeitem").filter({ hasText: "broken.json" })).toBeVisible()
})

test("mobile filename filter keeps selected document open", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto("/view?path=config%2Fsettings.json")
  await page.getByRole("button", { name: "Open files" }).click()
  const drawer = page.getByRole("dialog", { name: "Files", exact: true })
  await filterFiles(page, "*.yaml")
  await expect(drawer.getByRole("treeitem", { name: "settings.json" })).toHaveCount(0)
  await drawer.getByRole("treeitem", { name: "settings.yaml" }).click()
  await expect(drawer).toBeHidden()
  await expect(page.getByRole("textbox", { name: "YAML source" })).toContainText("servef-yaml-fixture")
})

test("source-only capabilities drive the UI without extension-specific components", async ({ page }) => {
  await page.route(`**/api/page?project=${rootParam}&path=main.go`, async (route) => {
    const fileType = {
      id: "go", label: "Go", extensions: [".go"], filenames: [], language: "go",
      previewKind: "", previewLabel: "", defaultMode: "source", formattable: false,
    }
    await route.fulfill({ json: {
      content: "", source: "package main\n", highlightDisabled: false,
      empty: false, error: "", fileCount: 1, fileSize: 13, hasFile: true, roots: ["/example"], repository: "", projectKind: "folder", branch: "", worktrees: 0, project: fixtureRoot, projectUnavailable: false, rootName: "example", selected: "main.go",
      warnings: [], previewError: "", filenamePatterns: ["*"], documentType: fileType,
      navigationSupported: false, binary: false, tooLarge: false, previewDisabled: false, searchExcluded: false,
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
  const filter = page.getByRole("textbox", { name: "Filter files by name" })
  await filterFiles(page, "*.json")
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "Open files" }).click()
  await expect(filter).toHaveValue("*.json")
  await filterFiles(page, "")
  await expect(page.getByRole("treeitem", { name: "getting-started.md" })).toBeVisible()
  await expect(page.getByRole("tab", { name: "getting-started.md" })).toBeVisible()
})

test("collapse and expand all preserve document, hidden folders, and responsive state", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  await page.getByRole("button", { name: "Collapse all folders" }).click()
  await expect(page.getByRole("treeitem", { name: "getting-started.md" })).toHaveCount(0)
  await expect(page.getByRole("tab", { name: "getting-started.md" })).toBeVisible()
  await filterFiles(page, "*.json")
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "Open files" }).click()
  await expect(page.getByRole("button", { name: "Expand all folders" })).toBeVisible()
  await filterFiles(page, "")
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
  await filterFiles(page, "*.json, *.yaml")
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
  await filterFiles(page, "*.json")
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "Open files" }).click()
  await filterFiles(page, "")
  await expect(page.getByRole("treeitem", { name: "getting-started.md" })).toBeVisible()
  await page.getByRole("button", { name: "Collapse all folders" }).click()
  await expect(page.getByRole("button", { name: "Expand all folders" })).toBeVisible()
  await expect(page.getByRole("treeitem", { name: "guides" })).toHaveAttribute("aria-expanded", "false")
})
