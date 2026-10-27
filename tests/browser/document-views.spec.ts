import { expect, test, type Page } from "@playwright/test"
import { fixtureRoot, rootParam } from "./fixture-root.ts"
import { apiOrigin } from "./ports.ts"

function documentPage(selected: string, documentType: Record<string, unknown>, overrides: Record<string, unknown>) {
  const name = selected.split("/").at(-1) ?? selected
  return {
    content: "", source: "", highlightDisabled: false, empty: false, error: "", fileCount: 1, fileSize: 0, hasFile: true,
    roots: ["/example"], repository: "", projectKind: "folder", branch: "", worktrees: 0, project: fixtureRoot, projectUnavailable: false, rootName: "example", selected, warnings: [], previewError: "", filenamePatterns: ["*"], documentType,
    navigationSupported: false, binary: false, tooLarge: false, previewDisabled: false, searchExcluded: false, at: "",
    tree: [{ name, path: selected, fileType: String(documentType.id), isDir: false, open: false, selected: true, children: [] }],
    ...overrides,
  }
}

function zoom(page: Page) {
  return page.getByRole("group", { name: "Zoom" })
}

test("binary files open to a note without views", async ({ page }) => {
  await page.goto("/view?path=assets%2Fsample.bin")
  await expect(page.getByRole("heading", { name: "Binary file" })).toBeVisible()
  await expect(page.locator("#document-pane")).toContainText("sample.bin · 24 B")
  await expect(page.locator("#document-pane")).toContainText("servef shows text and images only.")
  await expect(page.getByRole("group", { name: "Document view" })).toHaveCount(0)
  await expect(zoom(page)).toHaveCount(0)
  await expect(page.locator("#document-pane").getByRole("textbox")).toHaveCount(0)
  await expect(page.locator(".status-file-type")).toHaveText("Binary")
})

test(`raster images fit, zoom by button and keyboard, and report dimensions (embedded binary)`, async ({ page }) => {
  await page.goto(`${apiOrigin}/view?path=assets%2Fpreview.png`)
  const image = page.getByRole("img", { name: "preview.png" })
  await expect(image).toBeVisible()
  await expect.poll(() => image.evaluate((element) => element instanceof HTMLImageElement ? element.naturalWidth : 0)).toBeGreaterThan(0)
  await expect(page.getByRole("group", { name: "Document view" })).toHaveCount(0)
  const controls = zoom(page)
  await expect(controls.getByRole("button", { name: "Fit" })).toHaveAttribute("aria-pressed", "true")
  const status = page.locator(".status-document-detail")
  await expect(status).toHaveText(/^\d+ × \d+ · \d+%$/)

  await controls.getByRole("button", { name: /reset to 100%$/ }).click()
  await expect(controls.getByRole("button", { name: "100%, reset to 100%" })).toBeVisible()
  await expect(controls.getByRole("button", { name: "Fit" })).toHaveAttribute("aria-pressed", "false")
  await controls.getByRole("button", { name: "Zoom in" }).click()
  await expect(status).toHaveText(/ · 125%$/)
  await expect(controls.locator("[aria-live]")).toHaveText("Zoom 125%")
  const width = await image.evaluate((element) => element.getBoundingClientRect().width)
  const naturalWidth = await image.evaluate((element) => element instanceof HTMLImageElement ? element.naturalWidth : 0)
  expect(Math.abs(width - Math.floor(naturalWidth * 1.25))).toBeLessThanOrEqual(1)

  const viewer = page.getByRole("group", { name: "preview.png image" })
  await viewer.focus()
  await page.keyboard.press("-")
  await expect(status).toHaveText(/ · 100%$/)
  await page.keyboard.press("0")
  await expect(controls.getByRole("button", { name: "Fit" })).toHaveAttribute("aria-pressed", "true")
  await page.keyboard.press("1")
  await expect(status).toHaveText(/ · 100%$/)
})

test("SVG previews as an image and keeps view controls usable on narrow screens", async ({ page }) => {
  await page.goto("/view?path=assets%2Fpreview.svg")
  const view = page.getByRole("group", { name: "Document view" })
  await expect(view.getByRole("button")).toHaveText(["Preview", "Source"])
  await expect(view.getByRole("button", { name: "Preview" })).toHaveAttribute("aria-pressed", "true")
  await expect(page.getByRole("img", { name: "preview.svg" })).toBeVisible()
  await expect(zoom(page)).toBeVisible()
  await expect(page.locator(".status-document-detail")).toHaveText(/^640 × 180 · \d+%$/)
  await page.setViewportSize({ width: 320, height: 680 })
  await expect(view).toBeInViewport({ ratio: 1 })
  await expect(zoom(page)).toBeInViewport({ ratio: 1 })
  await expect(page.getByRole("button", { name: "Settings", exact: true })).toBeInViewport({ ratio: 1 })
  await zoom(page).getByRole("button", { name: "Zoom in" }).click()
  await expect(zoom(page).getByRole("button", { name: "Fit" })).toHaveAttribute("aria-pressed", "false")
  await view.getByRole("button", { name: "Source" }).click()
  await expect(page.getByRole("textbox", { name: "SVG source" })).toContainText("<svg")
  await expect(zoom(page)).toHaveCount(0)
  await expect(page.locator(".status-document-detail")).toHaveCount(0)
})

test("undecodable images explain the failure", async ({ page }) => {
  await page.route(`**/api/raw?project=${rootParam}&path=assets%2Fpreview.png`, (route) => route.fulfill({
    status: 200, contentType: "image/png", body: "not a png",
  }))
  await page.goto("/view?path=assets%2Fpreview.png")
  await expect(page.getByRole("alert")).toHaveText("Can't display this image.")
  await expect(zoom(page).getByRole("button", { name: "Zoom in" })).toBeDisabled()
})

test("large files show Source with a warning and disable preview", async ({ page }) => {
  const markdown = {
    id: "markdown", label: "Markdown", extensions: [".md"], filenames: [], language: "markdown",
    previewKind: "markdown", previewLabel: "Preview", defaultMode: "preview", formattable: false,
  }
  await page.route(`**/api/page?project=${rootParam}&path=big.md`, (route) => route.fulfill({ json: documentPage("big.md", markdown, {
    source: "# Big\n", highlightDisabled: true, fileSize: 6_500_000, previewDisabled: true,
  }) }))
  await page.goto("/view?path=big.md")
  await expect(page.getByRole("note")).toHaveText("Large file (6.2 MB): preview and formatting are off.")
  await expect(page.getByRole("textbox", { name: "Markdown source" })).toContainText("# Big")
  const preview = page.getByRole("button", { name: "Preview", exact: true })
  await expect(preview).toHaveAttribute("aria-disabled", "true")
  await expect(preview).toHaveAttribute("title", "Large file (6.2 MB): preview is off")
  await preview.click({ force: true })
  await expect(page.getByRole("button", { name: "Source", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(page.locator("article")).toHaveCount(0)
})
