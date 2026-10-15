import { expect, test, type Locator, type Page } from "@playwright/test"

function source(page: Page, label: string) {
  return page.getByRole("textbox", { name: label })
}

// word finds a name in a Source line's highlighted code.
function word(code: Locator, line: number, text: string) {
  return code.locator(`[data-line="${line}"] .source-document-line-content`).getByText(text, { exact: true })
}

test("Ctrl/Cmd+click goes to a definition in another file, and Back returns", async ({ page }) => {
  await page.goto("/view?path=navigation%2Fgreet.go")
  const greet = source(page, "Go source")
  await word(greet, 6, "Format").click({ modifiers: ["ControlOrMeta"] })

  await expect(page).toHaveURL(/path=navigation%2Fformat\.go.*#L6$/)
  const format = source(page, "Go source")
  await expect(format.locator('[data-line="6"]')).toHaveClass(/selected/)
  await expect(format.locator('[data-line="6"]')).toContainText("func Format(words ...string) string {")

  await page.goBack()
  await expect(page).toHaveURL(/path=navigation%2Fgreet\.go/)
  await expect(word(source(page, "Go source"), 6, "Format")).toBeVisible()
})

test("Ctrl/Cmd+click on a definition lists its references", async ({ page }) => {
  await page.goto("/view?path=navigation%2Fformat.go")
  await word(source(page, "Go source"), 6, "Format").click({ modifiers: ["ControlOrMeta"] })

  const dialog = page.getByRole("dialog", { name: "2 references to Format" })
  await expect(dialog).toBeVisible()
  const options = dialog.getByRole("option")
  await expect(options).toHaveCount(2)
  await expect(options.nth(0)).toContainText("6:6")
  await expect(options.nth(0)).toContainText("definition")
  await expect(dialog.getByRole("group", { name: "navigation/greet.go" })).toBeVisible()
  await expect(dialog.getByText("navigation/format.go (1)")).toBeVisible()
  await expect(dialog.locator("mark").first()).toHaveText("Format")

  await page.keyboard.press("ArrowDown")
  await expect(options.nth(1)).toHaveAttribute("aria-selected", "true")
  await page.keyboard.press("Enter")
  await expect(dialog).toBeHidden()
  await expect(page).toHaveURL(/path=navigation%2Fgreet\.go.*#L6$/)
})

test("F12 and Shift+F12 act on the last clicked name; Escape closes", async ({ page }) => {
  await page.goto("/view?path=navigation%2Fgreet.go")
  const greet = source(page, "Go source")
  await word(greet, 6, "Format").click()
  await page.keyboard.press("Shift+F12")
  const dialog = page.getByRole("dialog", { name: "2 references to Format" })
  await expect(dialog).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(dialog).toBeHidden()
  await expect(greet).toBeFocused()

  await page.keyboard.press("F12")
  await expect(page).toHaveURL(/path=navigation%2Fformat\.go.*#L6$/)
})

test("the command palette runs navigation and explains when nothing is clicked", async ({ page }) => {
  await page.goto("/view?path=navigation%2Fgreet.go")
  const greet = source(page, "Go source")
  await greet.focus()
  await page.keyboard.press("ControlOrMeta+Shift+P")
  const palette = page.getByRole("dialog", { name: "Command palette" })
  await palette.getByRole("combobox", { name: "Command" }).fill("definition")
  await expect(palette.getByRole("option")).toHaveAccessibleName("Navigation: Go to Definition, F12")
  await page.keyboard.press("Enter")
  const explain = page.getByRole("dialog", { name: "Go to Definition" })
  await expect(explain.getByRole("status")).toHaveText("Click a name in the code first, then try again.")
  await page.keyboard.press("Escape")

  await word(greet, 6, "name").click()
  await page.keyboard.press("ControlOrMeta+Shift+P")
  await palette.getByRole("combobox", { name: "Command" }).fill("references")
  await page.keyboard.press("Enter")
  // A parameter's references stay in its function.
  await expect(page.getByRole("dialog", { name: "2 references to name" })).toBeVisible()
})

test("TSX components and imports resolve through the module", async ({ page }) => {
  await page.goto("/view?path=navigation%2Fweb%2Fapp.tsx")
  await word(source(page, "TSX source"), 4, "label").click({ modifiers: ["ControlOrMeta"] })
  await expect(page).toHaveURL(/path=navigation%2Fweb%2Flabel\.ts.*#L2$/)
})

test("holding Ctrl/Cmd over a name marks it as a link", async ({ page }) => {
  await page.goto("/view?path=navigation%2Fgreet.go")
  const greet = source(page, "Go source")
  const name = word(greet, 6, "Format")
  await name.hover()
  await expect(greet).not.toHaveClass(/navigation-linking/)
  const modifier = process.platform === "darwin" ? "Meta" : "Control"
  await page.keyboard.down(modifier)
  await expect(greet).toHaveClass(/navigation-linking/)
  // The underline is a CSS custom highlight over the name.
  await expect.poll(() => page.evaluate(() => [...CSS.highlights.get("servef-navigation-link") ?? []].map(String).join())).toBe("Format")
  await page.keyboard.up(modifier)
  await expect(greet).not.toHaveClass(/navigation-linking/)
  await expect.poll(() => page.evaluate(() => CSS.highlights.has("servef-navigation-link"))).toBe(false)
})

test("files without navigation ignore Ctrl/Cmd+click", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.json")
  let requests = 0
  page.on("request", (request) => { if (request.url().includes("/api/definition")) requests++ })
  await source(page, "JSON source").locator('[data-line="1"] .source-document-line-content').click({ modifiers: ["ControlOrMeta"] })
  await expect(page.getByRole("dialog")).toHaveCount(0)
  expect(requests).toBe(0)
})
