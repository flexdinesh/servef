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

for (const modifier of ["Control", "Meta"]) {
  test(`${modifier} hover preserves selected source text for copying`, async ({ page }) => {
    await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
    await page.goto("/view?path=navigation%2Fgreet.go")
    const greet = source(page, "Go source")
    await greet.focus()
    await word(greet, 6, "Format").hover()
    const selected = await greet.locator('[data-line="6"] .source-document-line-content').evaluate((line) => {
      const selection = window.getSelection()
      if (!selection) throw new Error("Text selection unavailable")
      // Select backwards across syntax spans, as when dragging over source.
      selection.setBaseAndExtent(line, line.childNodes.length, line, 0)
      return selection.toString()
    })
    expect(selected).toContain('return Format("hello", name)')
    const readSelection = () => page.evaluate(() => window.getSelection()?.toString())

    await page.keyboard.down(modifier)
    await expect(greet).toHaveClass(/navigation-linking/)
    await expect.poll(readSelection).toBe(selected)
    await word(greet, 5, "Greet").hover()
    await expect.poll(() => page.evaluate(() => [...CSS.highlights.get("servef-navigation-link") ?? []].map(String).join())).toBe("Greet")
    await expect.poll(readSelection).toBe(selected)
    await page.keyboard.up(modifier)
    await expect(greet).not.toHaveClass(/navigation-linking/)
    await expect.poll(readSelection).toBe(selected)
    await page.keyboard.press("ControlOrMeta+c")
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(selected)
  })
}

test("files without navigation ignore Ctrl/Cmd+click", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.json")
  let requests = 0
  page.on("request", (request) => { if (request.url().includes("/api/definition")) requests++ })
  await source(page, "JSON source").locator('[data-line="1"] .source-document-line-content').click({ modifiers: ["ControlOrMeta"] })
  await expect(page.getByRole("dialog")).toHaveCount(0)
  expect(requests).toBe(0)
})

const missingGopls = {
  navigable: true,
  languageServers: false,
  provider: "syntax",
  server: {
    name: "gopls",
    language: "Go",
    state: "missing",
    message: "",
    install: [{ label: "Go", command: "go install golang.org/x/tools/gopls@latest" }],
    docs: "https://go.dev/gopls/",
    memory: 0,
    idleMinutes: 10,
  },
}

test("the status bar explains how to install a missing language server", async ({ page }) => {
  const refreshes: string[] = []
  await page.route("**/api/navigation-status?*", async (route) => {
    if (new URL(route.request().url()).searchParams.get("refresh") === "1") refreshes.push(route.request().url())
    await route.fulfill({ json: missingGopls })
  })
  await page.goto("/view?path=navigation%2Fgreet.go")
  const provider = page.getByRole("group", { name: "Document status" }).getByText("Syntax", { exact: true })
  await provider.click()
  await expect(page.getByText("gopls is not installed.")).toBeVisible()
  await expect(page.getByText("go install golang.org/x/tools/gopls@latest")).toBeVisible()
  await page.getByRole("button", { name: "Check again" }).click()
  await expect.poll(() => refreshes.length).toBe(1)

  await page.getByRole("button", { name: "Turn on in Settings" }).click()
  const settings = page.getByRole("dialog", { name: "Settings" })
  await expect(settings.getByRole("combobox", { name: "Code Navigation: Language Servers" })).toBeFocused()
})

test("files without navigation show no navigation status", async ({ page }) => {
  await page.goto("/view?path=config%2Fsettings.json")
  await expect(page.getByRole("group", { name: "Document status" })).toContainText("JSON")
  await expect(page.locator(".status-navigation")).toHaveCount(0)
})

test("turning on language servers opts navigation requests in and shows who answered", async ({ page }) => {
  const headers: (string | undefined)[] = []
  await page.route("**/api/references?*", async (route) => {
    headers.push(route.request().headers()["x-servef-language-servers"])
    const response = await route.fetch()
    const body: unknown = await response.json()
    await route.fulfill({ json: typeof body === "object" && body !== null ? { ...body, provider: "language-server", server: "gopls" } : body })
  })
  await page.goto("/view?path=navigation%2Fformat.go")
  const format = word(source(page, "Go source"), 6, "Format")
  await format.click({ modifiers: ["ControlOrMeta"] })
  const dialog = page.getByRole("dialog", { name: "2 references to Format" })
  await expect(dialog).toBeVisible()
  await page.keyboard.press("Escape")
  expect(headers).toEqual([undefined])

  await page.keyboard.press("ControlOrMeta+,")
  const settings = page.getByRole("dialog", { name: "Settings" })
  await settings.getByRole("combobox", { name: "Code Navigation: Language Servers" }).selectOption("on")
  await settings.getByRole("button", { name: "Close settings" }).click()
  await expect(settings).toBeHidden()
  await format.click({ modifiers: ["ControlOrMeta"] })
  await expect(dialog.getByText("gopls", { exact: true })).toBeVisible()
  expect(headers).toEqual([undefined, "on"])
})
