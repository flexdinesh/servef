import { expect, test } from "@playwright/test"
import { fixtureRoot, rootParam } from "./fixture-root.ts"

const customType = {
  id: "custom", label: "Custom", extensions: [".custom"], filenames: [], language: "js",
  previewKind: "", previewLabel: "", defaultMode: "source", formattable: true,
}

function customPage(source: string) {
  return {
    content: "", source, highlightDisabled: false,
    empty: false, error: "", fileCount: 1, fileSize: source.length, hasFile: true, roots: ["/example"], repository: "", projectKind: "folder", branch: "", worktrees: 0, project: fixtureRoot, projectUnavailable: false, rootName: "example", selected: "sample.custom",
    warnings: [], previewError: "", filenamePatterns: ["*"], documentType: customType,
    navigationSupported: false, binary: false, tooLarge: false, previewDisabled: false, searchExcluded: false, at: "",
    tree: [{ name: "sample.custom", path: "sample.custom", fileType: "custom", isDir: false, open: false, selected: true, children: [] }],
  }
}

test("format failures keep Source and allow a retry", async ({ page }) => {
  let attempts = 0
  await page.route(`**/api/page?project=${rootParam}&path=sample.custom`, (route) => route.fulfill({ json: customPage("retry original\n") }))
  await page.route(`**/api/format?project=${rootParam}&path=sample.custom`, (route) => {
    attempts++
    return attempts === 1
      ? route.fulfill({ status: 422, json: { error: "unexpected token at line 2." } })
      : route.fulfill({ json: { formatted: "retried\n" } })
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
