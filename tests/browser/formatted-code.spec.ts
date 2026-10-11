import { expect, test } from "@playwright/test"

test("formatted code uses declared capabilities and preserves internal and trailing blank rows and escaped multiline syntax", async ({ page }) => {
  const source = "unformatted original\n"
  await page.route("**/api/page?path=sample.custom", async (route) => {
    const fileType = {
      id: "custom", label: "Custom", extensions: [".custom"], language: "custom",
      previewKind: "code", previewLabel: "Formatted", defaultMode: "preview",
    }
    await route.fulfill({ json: {
      content: '<pre><code class="language-custom syntax-highlight"><span class="syntax-s">first</span>\n\n<span class="syntax-s">  &lt;script&gt;last&lt;/script&gt; &amp; final</span>\n\n</code></pre>\n',
      source, sourceLines: ["unformatted original"],
      empty: false, error: "", fileCount: 1, fileSize: source.length, hasFile: true, rootName: "example", selected: "sample.custom",
      warnings: [], previewError: "", fileTypes: [fileType], documentType: fileType,
      tree: [{ name: "sample.custom", path: "sample.custom", fileType: "custom", isDir: false, open: false, selected: true, children: [] }],
    } })
  })
  await page.goto("/view?path=sample.custom")
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
