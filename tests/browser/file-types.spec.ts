import { expect, test } from "@playwright/test"
import { apiOrigin } from "./ports.ts"

test(`JSON opens as Source and formats on demand, preserving values (embedded binary)`, async ({ page }) => {
  let formatRequests = 0
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/format") formatRequests++
  })
  await page.goto(`${apiOrigin}/view?path=config%2Fsettings.json`)
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
