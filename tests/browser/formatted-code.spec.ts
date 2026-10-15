import { expect, test } from "@playwright/test"
import { fixtureRoot, rootParam } from "./fixture-root.ts"

const customType = {
  id: "custom", label: "Custom", extensions: [".custom"], filenames: [], language: "plaintext",
  previewKind: "", previewLabel: "", defaultMode: "source", formattable: true,
}

function customPage(source: string) {
  return {
    content: "", source, sourceLines: [source.trimEnd()],
    empty: false, error: "", fileCount: 1, fileSize: source.length, hasFile: true, roots: ["/example"], repository: "", projectKind: "folder", branch: "", worktrees: 0, project: fixtureRoot, projectUnavailable: false, rootName: "example", selected: "sample.custom",
    warnings: [], previewError: "", filenamePatterns: ["*"], documentType: customType,
    binary: false, tooLarge: false, previewDisabled: false, searchExcluded: false,
    tree: [{ name: "sample.custom", path: "sample.custom", fileType: "custom", isDir: false, open: false, selected: true, children: [] }],
  }
}

test("formatted rows come from the format endpoint and keep blank rows and escaped syntax", async ({ page }) => {
  await page.route(`**/api/page?project=${rootParam}&path=sample.custom`, (route) => route.fulfill({ json: customPage("unformatted original\n") }))
  await page.route(`**/api/format?project=${rootParam}&path=sample.custom`, (route) => route.fulfill({ json: {
    formattedLines: [
      '<span class="syntax-s">first</span>',
      "",
      '<span class="syntax-s">  &lt;script&gt;last&lt;/script&gt; &amp; final</span>',
      "",
      "",
    ],
  } }))
  await page.goto("/view?path=sample.custom")
  await expect(page.getByRole("textbox", { name: "Custom source" })).toContainText("unformatted original")
  await page.getByRole("button", { name: "Format", exact: true }).click()
  const formatted = page.getByRole("textbox", { name: "Custom formatted" })
  await expect(formatted.locator(".source-document-line-content")).toHaveText([
    "first", "", "  <script>last</script> & final", "", "",
  ])
  await expect(formatted.locator(".source-document-line-number")).toHaveText(["1", "2", "3", "4", "5"])
  await expect(formatted.locator(".syntax-s")).toHaveCount(2)
  await expect(formatted.locator("script")).toHaveCount(0)
  await expect(page.locator("article")).toHaveCount(0)
  await formatted.locator('[data-line="3"]').click()
  await expect(formatted.locator('[data-line="3"]')).toHaveClass(/active/)
  await page.getByRole("button", { name: "Source", exact: true }).click()
  await expect(page.getByRole("textbox", { name: "Custom source" })).toContainText("unformatted original")
})

test("slow formatting keeps Source visible, marks Format busy, and reports Loading", async ({ page }) => {
  let release = () => {}
  const released = new Promise<void>((resolve) => { release = resolve })
  await page.route(`**/api/page?project=${rootParam}&path=sample.custom`, (route) => route.fulfill({ json: customPage("slow original\n") }))
  await page.route(`**/api/format?project=${rootParam}&path=sample.custom`, async (route) => {
    await released
    await route.fulfill({ json: { formattedLines: ['<span class="syntax-s">slow formatted</span>'] } })
  })
  await page.goto("/view?path=sample.custom")
  const formatButton = page.getByRole("button", { name: "Format", exact: true })
  const status = page.locator(".status-state")
  await expect(status).toHaveText("Ready")
  await formatButton.click()
  await expect(formatButton).toHaveAttribute("aria-busy", "true")
  await expect(page.getByRole("textbox", { name: "Custom source" })).toContainText("slow original")
  await expect(status).toHaveText("Loading…")
  release()
  await expect(page.getByRole("textbox", { name: "Custom formatted" })).toContainText("slow formatted")
  await expect(formatButton).not.toHaveAttribute("aria-busy", "true")
  await expect(status).toHaveText("Ready")
})

test("format failures keep Source and allow a retry", async ({ page }) => {
  let attempts = 0
  await page.route(`**/api/page?project=${rootParam}&path=sample.custom`, (route) => route.fulfill({ json: customPage("retry original\n") }))
  await page.route(`**/api/format?project=${rootParam}&path=sample.custom`, (route) => {
    attempts++
    return attempts === 1
      ? route.fulfill({ status: 422, json: { error: "unexpected token at line 2." } })
      : route.fulfill({ json: { formattedLines: ["retried"] } })
  })
  await page.goto("/view?path=sample.custom")
  const formatButton = page.getByRole("button", { name: "Format", exact: true })
  await formatButton.click()
  await expect(page.getByRole("alert")).toHaveText("Couldn't format: unexpected token at line 2.")
  await expect(formatButton).toHaveAttribute("aria-pressed", "false")
  await expect(page.getByRole("textbox", { name: "Custom source" })).toContainText("retry original")
  await formatButton.click()
  await expect(page.getByRole("textbox", { name: "Custom formatted" })).toContainText("retried")
  await expect(page.getByRole("alert")).toHaveCount(0)
})
