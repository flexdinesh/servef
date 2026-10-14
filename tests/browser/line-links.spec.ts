import { expect, test, type Page } from "@playwright/test"

function readClipboard(page: Page) {
  return page.evaluate(() => navigator.clipboard.readText())
}

test.beforeEach(async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
})

test("line numbers select lines and ranges, update the URL, and copy path:line", async ({ page }) => {
  let pageRequests = 0
  page.on("request", (request) => { if (request.url().includes("/api/page")) pageRequests++ })
  await page.goto("/view?path=code%2Fmain.go")
  const source = page.getByRole("textbox", { name: "Go source" })
  const line = (number: number) => source.locator(`[data-line="${number}"]`)
  const gutter = (number: number) => line(number).locator(".source-document-line-number")
  await expect(line(1)).toBeVisible()
  const requestsBefore = pageRequests

  await gutter(4).click()
  await expect(page).toHaveURL(/#L4$/)
  await expect(line(4)).toHaveClass(/selected/)
  await expect(source.locator(".selected")).toHaveCount(1)
  await expect(page.locator(".status-lines")).toHaveText("Ln 4")
  const copyLine = page.getByRole("button", { name: "Copy code/main.go:4" })
  await copyLine.click()
  await expect(line(4).getByRole("status")).toHaveText("Copied")
  await expect.poll(() => readClipboard(page)).toBe("code/main.go:4")

  await gutter(6).click({ modifiers: ["Shift"] })
  await expect(page).toHaveURL(/#L4-L6$/)
  await expect(source.locator(".selected")).toHaveCount(3)
  await expect(page.locator(".status-lines")).toHaveText("Ln 4–6")
  await page.getByRole("button", { name: "Copy code/main.go:4-6" }).click()
  await expect.poll(() => readClipboard(page)).toBe("code/main.go:4-6")
  expect(await page.evaluate(() => window.getSelection()?.toString() ?? "")).toBe("")

  // Shift extends from the anchor line, in either direction.
  await gutter(2).click({ modifiers: ["Shift"] })
  await expect(page).toHaveURL(/#L2-L4$/)

  await source.press("Escape")
  await expect(page).not.toHaveURL(/#/)
  await expect(source.locator(".selected")).toHaveCount(0)
  await expect(page.locator(".status-lines")).toHaveCount(0)
  // Selection changes only the fragment; the document is not fetched again.
  expect(pageRequests).toBe(requestsBefore)
})

test("the footer copies the open file's project-relative path", async ({ page }) => {
  await page.goto("/view?path=code%2Fmain.go")
  const copy = page.getByRole("button", { name: "Copy path" })
  await copy.click()
  await expect.poll(() => readClipboard(page)).toBe("code/main.go")
  await expect(copy).toHaveAttribute("data-status", "copied")
  await expect(page.locator(".status-document").getByRole("status")).toHaveText("Path copied")
})

test("a line anchor opens Markdown in Source with the range highlighted", async ({ page }) => {
  await page.goto("/view?path=guides%2Fcode-blocks.md#L3-L5")
  await expect(page.getByRole("button", { name: "Source", exact: true })).toHaveAttribute("aria-pressed", "true")
  const source = page.getByRole("textbox", { name: "Markdown source" })
  await expect(source.locator(".selected")).toHaveCount(3)
  await expect(source.locator(".selected").first()).toHaveAttribute("data-line", "3")
  await expect(page.getByRole("button", { name: "Copy guides/code-blocks.md:3-5" })).toBeVisible()

  await page.getByRole("button", { name: "Preview", exact: true }).click()
  await expect(page.locator("article")).toBeVisible()
  await expect(page).not.toHaveURL(/#/)
})

test("a line anchor scrolls its line into view and clamps to the document", async ({ page }) => {
  const source = Array.from({ length: 300 }, (_, index) => `line ${index + 1}`).join("\n") + "\n"
  const type = {
    id: "text", label: "Plain text", extensions: [".txt"], filenames: [], language: "plaintext",
    previewKind: "", previewLabel: "", defaultMode: "source", formattable: false,
  }
  await page.route("**/api/page?project=.&path=long.txt", (route) => route.fulfill({ json: {
    content: "", source, sourceLines: [], empty: false, error: "", fileCount: 1, fileSize: source.length,
    hasFile: true, servedRoot: "/example", project: ".", projectUnavailable: false, rootName: "example",
    selected: "long.txt", warnings: [], previewError: "", filenamePatterns: ["*"], documentType: type,
    binary: false, tooLarge: false, previewDisabled: false, searchExcluded: false,
    tree: [{ name: "long.txt", path: "long.txt", fileType: "text", isDir: false, open: false, selected: true, children: [] }],
  } }))
  await page.goto("/view?path=long.txt#L240-L999")
  const text = page.getByRole("textbox", { name: "Plain text source" })
  await expect(text.locator('[data-line="240"]')).toHaveClass(/selected/)
  await expect(text.locator('[data-line="240"]')).toBeInViewport()
  await expect(text.locator(".selected")).toHaveCount(61)
  await expect(page.locator(".status-lines")).toHaveText("Ln 240–300")
})
