import { expect, test, type Locator, type Page } from "@playwright/test"

function source(page: Page, label: string) {
  return page.getByRole("textbox", { name: label })
}

// word finds a name in a Source line's highlighted code.
function word(code: Locator, line: number, text: string) {
  return code.locator(`[data-line="${line}"] .source-document-line-content`).getByText(text, { exact: true })
}

// Browser navigation tests exercise UI contracts without installed servers.
function location(path: string, line: number, column: number, kind = "definition", symbol = "Format") {
  return { path, line, column, endLine: line, endColumn: column + symbol.length, kind, snippet: kind === "reference" ? `return ${symbol}(name)` : `func ${symbol}(words ...string) string {` }
}

test.beforeEach(async ({ page }) => {
  await page.route("**/api/navigation-status?*", async (route) => {
    const path = new URL(route.request().url()).searchParams.get("path") ?? ""
    const enabled = route.request().headers()["x-servef-language-servers"] === "on"
    await route.fulfill({ json: path === "notes.txt"
      ? { supported: false, enabled, eligible: false, reason: "No language server is configured for this language.", server: null }
      : { ...missingGopls, enabled, server: { ...missingGopls.server, state: "stopped" } } })
  })
  await page.route("**/api/definition?*", async (route) => {
    const path = new URL(route.request().url()).searchParams.get("path")
    const target = path?.endsWith("app.tsx") ? location("navigation/web/label.ts", 2, 13, "definition", "label") : location("navigation/format.go", 6, 5)
    await route.fulfill({ json: { symbol: path?.endsWith("app.tsx") ? "label" : "Format", locations: [target], server: "gopls", filtered: 0, truncated: false } })
  })
  await page.route("**/api/references?*", async (route) => {
    const query = new URL(route.request().url()).searchParams
    const symbol = Number(query.get("column")) > 20 ? "name" : "Format"
    await route.fulfill({ json: { symbol, locations: [location("navigation/format.go", 6, 5, "definition", symbol), location("navigation/greet.go", 6, 8, "reference", symbol)], server: "gopls", filtered: 0, truncated: false } })
  })
})

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

for (const modifier of [process.platform === "darwin" ? "Meta" : "Control"]) {
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

const missingGopls = {
  supported: true, enabled: true, eligible: true, reason: "",
  server: {
    name: "gopls", language: "Go", state: "missing", message: "",
    install: [{ label: "Go", command: "go install golang.org/x/tools/gopls@latest" }],
    docs: "https://go.dev/gopls/", memory: 0, idleMinutes: 10, capabilities: null,
  },
}

test("language servers default on and explicit off persists", async ({ page }) => {
  const headers: (string | undefined)[] = []
  page.on("request", (request) => {
    if (request.url().includes("/api/references?")) headers.push(request.headers()["x-servef-language-servers"])
  })
  await page.goto("/view?path=navigation%2Fformat.go")
  await word(source(page, "Go source"), 6, "Format").click({ modifiers: ["ControlOrMeta"] })
  const dialog = page.getByRole("dialog", { name: "2 references to Format" })
  await expect(dialog.getByText("gopls", { exact: true })).toBeVisible()
  expect(headers).toEqual(["on"])
  await page.keyboard.press("Escape")
  await page.keyboard.press("ControlOrMeta+,")
  const settings = page.getByRole("dialog", { name: "Settings" })
  await settings.getByRole("combobox", { name: "Code Navigation: Language Servers" }).selectOption("off")
  await settings.getByRole("button", { name: "Close settings" }).click()
  await page.reload()
  await expect(page.getByText("LSP off", { exact: true })).toBeVisible()
  await word(source(page, "Go source"), 6, "Format").click({ modifiers: ["ControlOrMeta"] })
  expect(headers).toEqual(["on"])
})

test("navigation sends the actual caret column to the language server", async ({ page }) => {
  let column = -1
  await page.route("**/api/definition?*", async (route) => {
    column = Number(new URL(route.request().url()).searchParams.get("column"))
    await route.fulfill({ json: { symbol: "Format", locations: [], server: "gopls", filtered: 0, truncated: false } })
  })
  await page.goto("/view?path=navigation%2Fgreet.go")
  await word(source(page, "Go source"), 6, "Format").click({ modifiers: ["ControlOrMeta"] })
  await expect(page.getByRole("dialog", { name: "Go to Definition" })).toBeVisible()
  expect(column).toBeGreaterThan(8)
  expect(column).toBeLessThan(14)
})

test("Escape cancels a slow definition and ignores its late result", async ({ page }) => {
  let release = () => {}
  const released = new Promise<void>((resolve) => { release = resolve })
  await page.route("**/api/definition?*", async (route) => {
    await released
    await route.fulfill({ json: { symbol: "Format", locations: [location("navigation/format.go", 6, 5)], server: "gopls", filtered: 0, truncated: false } })
  })
  await page.goto("/view?path=navigation%2Fgreet.go")
  await word(source(page, "Go source"), 6, "Format").click({ modifiers: ["ControlOrMeta"] })
  const loading = page.getByRole("dialog", { name: "Finding definition for Format…" })
  await expect(loading).toBeVisible()
  await page.keyboard.press("Escape")
  release()
  await expect(loading).toBeHidden()
  await expect(page).toHaveURL(/path=navigation%2Fgreet.go/)
})

test("navigation errors refresh availability and show the language server failure", async ({ page }) => {
  let statuses = 0
  await page.route("**/api/navigation-status?*", async (route) => {
    statuses++
    await route.fulfill({ json: { ...missingGopls, server: { ...missingGopls.server, state: statuses > 1 ? "failed" : "stopped", message: "start failed" } } })
  })
  await page.route("**/api/definition?*", (route) => route.fulfill({ status: 503, json: { error: "gopls could not start.", code: "failed" } }))
  await page.goto("/view?path=navigation%2Fgreet.go")
  await expect(page.getByText("LSP available", { exact: true })).toBeVisible()
  await word(source(page, "Go source"), 6, "Format").click({ modifiers: ["ControlOrMeta"] })
  await expect(page.getByRole("dialog").getByRole("status")).toHaveText("gopls could not start.")
  await expect(page.getByText("LSP failed", { exact: true })).toBeVisible()
})
