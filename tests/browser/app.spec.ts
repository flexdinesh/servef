import { expect, test, type Page } from "@playwright/test"
import { endsWith, projectParam, rootParam } from "./fixture-root.ts"
import { apiOrigin, tldrawApiOrigin } from "./ports.ts"

const mermaidCaseCount = 4

// showPreview renders the open document; Markdown opens in Source by default.
async function showPreview(page: Page) {
  await page.getByRole("group", { name: "Document view" }).getByRole("button", { name: "Preview", exact: true }).click()
}

async function revealMermaidCases(page: Page) {
  const blocks = page.locator("main .mermaid-lazy")
  await expect(blocks).toHaveCount(mermaidCaseCount)
  for (let index = 0; index < mermaidCaseCount; index++) {
    await blocks.nth(index).scrollIntoViewIfNeeded()
  }
}

test("uses embedded page data on a direct production load", async ({ page }) => {
  let pageRequests = 0
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/page") pageRequests++
  })

  await page.goto(`${apiOrigin}/view?project=${rootParam}&path=guides%2Fgetting-started.md`)
  await showPreview(page)

  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
  expect(pageRequests).toBe(0)
})

test("renders local Markdown images", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  await showPreview(page)

  const image = page.getByRole("img", { name: "servef preview fixture" })
  await expect(image).toBeVisible()
  await expect.poll(() => image.evaluate((element) => (
    element instanceof HTMLImageElement && element.naturalWidth > 0 && element.naturalHeight > 0
  ))).toBe(true)
})

test("renders SVG, PNG, and JPEG Markdown images", async ({ page }) => {
  await page.goto("/view?path=guides%2Fimages.md")
  await showPreview(page)

  for (const name of ["servef SVG preview", "servef PNG preview", "servef JPEG preview"]) {
    const image = page.getByRole("img", { name })
    await expect(image).toBeVisible()
    await expect.poll(() => image.evaluate((element) => (
      element instanceof HTMLImageElement && element.naturalWidth > 0 && element.naturalHeight > 0
    ))).toBe(true)
  }
})

test("browses nested Markdown without resetting the file tree", async ({ page }) => {
  await page.goto("/")

  const tree = page.getByRole("complementary", { name: "Files" })
  await tree.getByRole("treeitem", { name: "guides", exact: true }).click()
  await tree.getByRole("treeitem", { name: "reference", exact: true }).click()
  await tree.getByRole("treeitem", { name: "topics", exact: true }).click()
  await expect(tree.getByRole("treeitem", { name: "getting-started.md" })).toBeVisible()
  await expect(tree.getByRole("treeitem", { name: "api.markdown" })).toBeVisible()
  await expect(tree.getByRole("treeitem", { name: "search.md" })).toBeVisible()
  await expect(tree.getByRole("treeitem", { name: "notes.txt" })).toBeVisible()
  await expect(tree.getByText("ignored.md")).toHaveCount(0)

  await page.evaluate(() => {
    document.body.dataset.navigationMarker = "preserved"
  })
  await tree.getByRole("treeitem", { name: "getting-started.md" }).click()
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&path=guides%2Fgetting-started.md`))
  await showPreview(page)
  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
  await expect(page.locator("main table")).toContainText("pnpm dev")
  await expect(page).toHaveTitle("guides/getting-started.md · servef")
  await expect(tree.getByRole("treeitem", { name: "getting-started.md" })).toHaveAttribute("aria-selected", "true")
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
  await expect(tree.getByRole("treeitem", { name: "api.markdown" })).toBeVisible()
  await expect(tree.getByRole("treeitem", { name: "search.md" })).toBeVisible()
})

test("modified tree clicks select files without creating browser tabs", async ({ context, page }) => {
  await page.goto("/")
  const tree = page.getByRole("complementary", { name: "Files" })
  await tree.getByRole("treeitem", { name: "guides", exact: true }).click()
  await tree.getByRole("treeitem", { name: "getting-started.md" }).click({ modifiers: ["Control"] })
  await expect(page).toHaveURL(/path=guides%2Fgetting-started.md/)
  expect(context.pages()).toHaveLength(1)
})

test("searches fixture content and opens the result", async ({ page }) => {
  await page.goto("/")

  const search = page.getByPlaceholder("Search document content…")
  await expect(page.getByText("Select a file from the folder tree.")).toBeVisible()
  await page.getByRole("button", { name: /^Find in files/ }).press("ControlOrMeta+Shift+f")
  await expect(search).toBeVisible()
  await search.fill("luminous-orchid")

  const result = page.getByRole("option").filter({ hasText: "search.md" })
  await expect(result).toBeVisible()
  await page.evaluate(() => {
    document.body.dataset.navigationMarker = "preserved"
  })
  await result.click()
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&path=reference%2Ftopics%2Fsearch.md`))
  await expect(page.locator("main")).toContainText("luminous-orchid")
  await expect(page.locator("main")).toBeFocused()
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
})

test("navigates Markdown links and history without losing tree state", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  await showPreview(page)

  const tree = page.getByRole("complementary", { name: "Files" })
  await tree.getByRole("treeitem", { name: "reference", exact: true }).click()
  await tree.getByRole("treeitem", { name: "topics", exact: true }).click()
  await expect(tree.getByRole("treeitem", { name: "search.md" })).toBeVisible()

  await page.evaluate(() => {
    document.body.dataset.navigationMarker = "preserved"
  })
  await page.getByRole("link", { name: "search notes" }).click()
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&path=reference%2Ftopics%2Fsearch.md`))
  await expect(page.getByRole("heading", { level: 1, name: "Search notes" })).toBeVisible()
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
  await expect(tree.getByRole("treeitem", { name: "getting-started.md" })).toBeVisible()
  await expect(tree.getByRole("treeitem", { name: "search.md" })).toBeVisible()

  await page.goBack()
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&path=guides%2Fgetting-started.md`))
  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
  await expect(tree.getByRole("treeitem", { name: "search.md" })).toBeVisible()

  await page.goForward()
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&path=reference%2Ftopics%2Fsearch.md`))
  await expect(page.getByRole("heading", { level: 1, name: "Search notes" })).toBeVisible()
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
  await expect(tree.getByRole("treeitem", { name: "getting-started.md" })).toBeVisible()
})

test("preserves Markdown hashes during client navigation", async ({ page }) => {
  await page.goto("/view?path=reference%2Fapi.markdown")
  await showPreview(page)

  await page.evaluate(() => {
    document.body.dataset.navigationMarker = "preserved"
  })
  await page.getByRole("link", { name: "diagrams" }).click()

  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&path=guides%2Fdiagrams.md#request-flow`))
  await expect(page.getByRole("heading", { level: 2, name: "Request flow" })).toBeInViewport()
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
})

test("recovers from an invalid page response", async ({ page }) => {
  let returnInvalidResponse = true
  await page.route(`**/api/page?project=${rootParam}&path=guides%2Fgetting-started.md`, async (route) => {
    if (returnInvalidResponse) {
      returnInvalidResponse = false
      await route.fulfill({ contentType: "application/json", body: "{}" })
      return
    }
    await route.continue()
  })
  await page.goto("/")

  await page.getByRole("button", { name: /^Find in files/ }).press("ControlOrMeta+Shift+f")
  const search = page.getByPlaceholder("Search document content…")
  await search.fill("getting-started")
  await page.getByRole("option", {
    exact: true,
    name: /getting-started.md guides\/getting-started.md/,
  }).click()

  await expect(page.getByRole("alert")).toContainText("invalid page response")
  await page.getByRole("button", { name: "Try again" }).click()
  await expect(page.getByRole("textbox", { name: "Markdown source" })).toContainText("# Getting started")
})

test("commits only the latest rapid navigation", async ({ page }) => {
  let releaseDelayedResponse = () => {}
  const delayedResponse = new Promise<void>((resolve) => {
    releaseDelayedResponse = resolve
  })
  await page.route(`**/api/page?project=${rootParam}&path=guides%2Fdiagrams.md`, async (route) => {
    await delayedResponse
    await route.continue().catch(() => {})
  })
  await page.goto("/")

  const tree = page.getByRole("complementary", { name: "Files" })
  await tree.getByRole("treeitem", { name: "guides", exact: true }).click()
  await tree.getByRole("treeitem", { name: "diagrams.md" }).dispatchEvent("click")
  await tree.getByRole("treeitem", { name: "getting-started.md" }).dispatchEvent("click")

  await expect(page.getByRole("textbox", { name: "Markdown source" })).toContainText("# Getting started")
  releaseDelayedResponse()
  await page.waitForTimeout(100)
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&path=guides%2Fgetting-started.md`))
})

test("opens content search from the status bar", async ({ page }) => {
  await page.goto("/")

  await page.getByRole("button", { name: /^Find in files/ }).click()
  await expect(page.getByPlaceholder("Search document content…")).toBeFocused()
})

// themeSetting opens Settings and returns its Theme select.
async function themeSetting(page: Page) {
  await page.getByRole("button", { name: "Settings" }).click()
  return page.getByRole("dialog", { name: "Settings" }).getByRole("combobox", { name: "Appearance: Theme" })
}

test("selects and persists an explicit theme in settings", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/")

  const theme = await themeSetting(page)
  await expect(theme).toHaveValue("system")
  await theme.selectOption({ label: "Dark" })

  await expect(page.locator("html")).toHaveClass(/dark/)
  await expect(page.locator("html")).toHaveCSS("background-color", "rgb(24, 24, 24)")
  expect(await page.evaluate(() => localStorage.getItem("servef-theme"))).toBe("dark")

  await page.reload()
  await expect(page.locator("html")).toHaveClass(/dark/)
  await expect(await themeSetting(page)).toHaveValue("dark")

  const dialog = page.getByRole("dialog", { name: "Settings" })
  await dialog.getByRole("button", { name: "Reset all" }).click()
  await expect(page.locator("html")).toHaveClass(/light/)
  await expect(page.locator("html")).toHaveCSS("background-color", "rgb(248, 248, 248)")
  await expect(dialog.getByRole("combobox", { name: "Appearance: Theme" })).toHaveValue("system")
})

test("system theme follows live OS changes", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/")

  await expect(page.locator("html")).toHaveClass(/light/)
  await page.emulateMedia({ colorScheme: "dark" })
  await expect(page.locator("html")).toHaveClass(/dark/)
})

test("command palette opens settings at the chosen setting", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  const source = page.getByRole("textbox", { name: "Markdown source" })
  await source.click()

  await page.keyboard.press("ControlOrMeta+Shift+P")
  const palette = page.getByRole("dialog", { name: "Command palette" })
  const input = palette.getByRole("combobox", { name: "Command" })
  await expect(input).toBeFocused()
  const options = palette.getByRole("option")
  await expect(options).toHaveCount(3)
  await expect(options.nth(0)).toHaveAccessibleName(/^Preferences: Open Settings, (Ctrl\+|⌘),$/)
  await expect(options.nth(1)).toHaveAccessibleName("Settings: Theme, System")
  await expect(options.nth(2)).toHaveAccessibleName("Settings: Font Size, 13px")
  await expect(options.first()).toHaveAttribute("aria-selected", "true")

  await input.fill("nothing here")
  await expect(palette.getByText("No matching commands")).toBeVisible()
  await input.fill("size")
  await expect(options).toHaveCount(1)
  await expect(options.first()).toHaveAccessibleName("Settings: Font Size, 13px")
  await expect(palette.locator("mark")).toHaveText(["Size"])

  await page.keyboard.press("Escape")
  await expect(palette).toBeHidden()
  await expect(source).toBeFocused()

  await page.keyboard.press("F1")
  await expect(input).toHaveValue("")
  await input.fill("font")
  await page.keyboard.press("Enter")
  await expect(palette).toBeHidden()
  const settings = page.getByRole("dialog", { name: "Settings" })
  const fontSize = settings.getByRole("spinbutton", { name: "Appearance: Font Size" })
  await expect(fontSize).toBeFocused()
  const item = settings.locator('[data-setting="fontSize"]')
  await expect(item).toHaveAttribute("data-revealed", "true")
  await expect(item).not.toHaveAttribute("data-revealed")

  await page.keyboard.type("15")
  await expect(source).toHaveCSS("font-size", "15px")
  await page.keyboard.press("Escape")
  await page.keyboard.press("ControlOrMeta+Shift+P")
  await page.keyboard.press("ArrowDown")
  await page.keyboard.press("Enter")
  await expect(settings.getByRole("combobox", { name: "Appearance: Theme" })).toBeFocused()

  await page.keyboard.press("Escape")
  await page.keyboard.press("ControlOrMeta+Shift+P")
  await page.keyboard.press("Enter")
  await expect(settings.getByRole("searchbox", { name: "Search settings" })).toBeFocused()
  await expect(settings.locator("[data-revealed]")).toHaveCount(0)
})

test("uses readable body and navigation text sizes", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")
  await showPreview(page)

  const tree = page.getByRole("complementary", { name: "Files" })
  const selected = tree.getByRole("treeitem", { name: "getting-started.md" })
  await expect(page.locator("article")).toHaveCSS("font-size", "14px")
  await expect(selected).toHaveCSS("font-size", "13px")
  await expect(selected).toHaveCSS("border-radius", "0px")
})

test("font size setting scales the interface, persists, and resets", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")

  const tree = page.getByRole("complementary", { name: "Files" })
  const selected = tree.getByRole("treeitem", { name: "getting-started.md" })
  const source = page.getByRole("textbox", { name: "Markdown source" })
  const status = page.locator(".status-bar")
  await expect(source).toHaveCSS("font-size", "13px")

  await page.getByRole("button", { name: "Settings" }).click()
  const dialog = page.getByRole("dialog", { name: "Settings" })
  await expect(dialog.getByRole("searchbox", { name: "Search settings" })).toBeFocused()
  const fontSize = dialog.getByRole("spinbutton", { name: "Appearance: Font Size" })
  await expect(fontSize).toHaveValue("13")
  await expect(dialog.getByRole("button", { name: "Reset all" })).toBeDisabled()

  await fontSize.fill("16")
  await expect(source).toHaveCSS("font-size", "16px")
  await expect(selected).toHaveCSS("font-size", "16px")
  // The status bar keeps its 12:13 ratio to the base size.
  await expect.poll(() => status.evaluate((element) => parseFloat(getComputedStyle(element).fontSize))).toBeCloseTo(12 * 16 / 13)
  await expect(dialog.getByText("Modified")).toBeVisible()
  expect(await page.evaluate(() => localStorage.getItem("servef-settings"))).toBe('{"fontSize":16}')

  await fontSize.fill("40")
  await expect(dialog.getByText("Enter a whole number from 10 to 20.")).toBeVisible()
  await expect(source).toHaveCSS("font-size", "16px")

  await page.keyboard.press("Escape")
  await expect(dialog).toBeHidden()
  await page.reload()
  await expect(source).toHaveCSS("font-size", "16px")
  await expect(selected).toHaveCSS("font-size", "16px")

  await page.keyboard.press("ControlOrMeta+,")
  await expect(dialog).toBeVisible()
  await dialog.getByRole("searchbox", { name: "Search settings" }).fill("colour")
  await expect(dialog.getByText("No settings match “colour”.")).toBeVisible()
  await dialog.getByRole("button", { name: "Reset all" }).click()
  await expect(source).toHaveCSS("font-size", "13px")
  expect(await page.evaluate(() => localStorage.getItem("servef-settings"))).toBeNull()
})

test("highlights fenced code with the selected theme", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/view?path=reference%2Fapi.markdown")
  await showPreview(page)

  const keyword = page.locator(".syntax-highlight [class^='syntax-k']").first()
  await expect(keyword).toHaveText("func")
  await expect(keyword).toHaveCSS("color", "rgb(207, 34, 46)")

  await (await themeSetting(page)).selectOption({ label: "Nord" })
  await expect(keyword).toHaveCSS("color", "rgb(129, 161, 193)")
})

test("labels and copies fenced code", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
  await page.goto("/view?path=guides%2Fcode-blocks.md")
  await showPreview(page)

  const codeBlocks = page.locator(".code-block")
  await expect(codeBlocks).toHaveCount(5)
  for (const [index, language] of ["json", "bash", "shell"].entries()) {
    await expect(codeBlocks.nth(index).getByRole("button")).toHaveAccessibleName(`Copy ${language} code`)
  }
  await expect(codeBlocks.nth(3).getByRole("button")).toHaveAccessibleName("Copy code")
  await expect(codeBlocks.nth(4).getByRole("button")).toHaveAccessibleName("Copy code")

  const codeBlock = codeBlocks.first()
  const copy = codeBlock.getByRole("button")
  await expect(copy).toHaveAccessibleName("Copy json code")
  const blockRadius = await codeBlock.locator("pre").evaluate((element) => window.getComputedStyle(element).borderRadius)
  await expect(copy).toHaveCSS("border-radius", blockRadius)
  await expect(copy.locator(".code-block-control-language")).toHaveText("json")
  const languageColor = await copy.locator(".code-block-control-language").evaluate(
    (element) => window.getComputedStyle(element).color,
  )
  await expect(copy.locator(".code-block-control-copy")).toBeHidden()
  expect(await copy.evaluate((element) => element.getBoundingClientRect().width)).toBeGreaterThan(24)
  await codeBlock.hover({ position: { x: 20, y: 20 } })
  await expect(copy).toHaveCSS("width", "24px")
  await expect(copy.locator(".code-block-control-language")).toBeHidden()
  const copyIcon = copy.locator(".code-block-control-copy")
  await expect(copyIcon).toBeVisible()
  await expect(copyIcon).toHaveCSS("width", "12px")
  await expect(copyIcon).toHaveCSS("height", "12px")
  await expect(copyIcon).toHaveCSS("color", languageColor)
  await expect(copyIcon).toHaveCSS("stroke-width", "2px")
  await copy.click()
  await expect(copy).toHaveAccessibleName("Copy json code")
  await page.mouse.move(0, 0)
  await expect(copy).toHaveCSS("width", "24px")
  await expect(copy.locator(".code-block-control-language")).toBeHidden()
  await expect(copy.locator(".code-block-control-copy")).toBeVisible()
  await expect(codeBlock.getByRole("status")).toHaveText("Copied")
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toContain('"host": "localhost"')
})

test("copies fenced code when the modern clipboard API is unavailable", async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: undefined })
  })
  await page.goto("/view?path=guides%2Fcode-blocks.md")
  await showPreview(page)

  const codeBlock = page.locator(".code-block").first()
  const copy = codeBlock.getByRole("button", { name: "Copy json code" })
  await copy.click()
  await expect(codeBlock.getByRole("status")).toHaveText("Copied")
  await expect(copy).toHaveCSS("outline-style", "none")
})

test("opens, switches, and closes document tabs", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")

  const tree = page.getByRole("complementary", { name: "Files" })
  await tree.getByRole("treeitem", { name: "diagrams.md" }).click()

  const tabs = page.getByRole("tablist", { name: "Open documents" })
  await expect(tabs.getByRole("tab", { name: "getting-started.md" })).toBeVisible()
  await expect(tabs.getByRole("tab", { name: "diagrams.md" })).toHaveAttribute("aria-selected", "true")

  await tabs.getByRole("tab", { name: "getting-started.md" }).click()
  await expect(page).toHaveURL(/path=guides%2Fgetting-started\.md/)
  await tabs.getByRole("button", { name: "Close guides/getting-started.md" }).click()

  await expect(page).toHaveURL(/path=guides%2Fdiagrams\.md/)
  await expect(tabs.getByRole("tab", { name: "getting-started.md" })).toHaveCount(0)
  await expect(tabs.getByRole("tab", { name: "diagrams.md" })).toHaveAttribute("aria-selected", "true")
})

test("switches document modes and remembers mode per open tab", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")

  const view = page.getByRole("group", { name: "Document view" })
  const preview = view.getByRole("button", { name: "Preview" })
  const source = view.getByRole("button", { name: "Source" })
  await expect(view.getByRole("button")).toHaveText(["Source", "Preview"])
  await expect(source).toHaveAttribute("aria-pressed", "true")

  const sourceView = page.getByRole("textbox", { name: "Markdown source" })
  await expect(sourceView).toContainText("# Getting started")
  await expect(sourceView).toHaveAttribute("aria-readonly", "true")
  await expect(sourceView.locator(".source-document-line-number").first()).toHaveText("1")
  await expect(sourceView.locator(".source-document-line-number").nth(1)).toHaveText("2")
  const sourceLines = sourceView.locator(".source-document-line")
  await expect(sourceView.locator(".source-document-line.active")).toHaveCount(0)
  await sourceLines.nth(2).click()
  await expect(sourceLines.nth(2)).toHaveClass(/active/)
  await expect(sourceLines.first()).not.toHaveClass(/active/)
  await expect(sourceLines.nth(2)).toHaveCSS("height", "20px")
  await expect(sourceView).toHaveCSS("font-size", "13px")
  const sourceLeft = await sourceView.evaluate((element) => element.getBoundingClientRect().left)
  const paneLeft = await page.locator("#document-pane").evaluate((element) => element.getBoundingClientRect().left)
  expect(sourceLeft).toBe(paneLeft)
  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toHaveCount(0)

  await preview.press("Enter")
  await expect(preview).toHaveAttribute("aria-pressed", "true")
  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
  await expect(sourceView).toHaveCount(0)

  const tree = page.getByRole("complementary", { name: "Files" })
  await tree.getByRole("treeitem", { name: "diagrams.md" }).click()
  await expect(preview).toHaveAttribute("aria-pressed", "true")
  await source.click()
  await expect(source).toHaveAttribute("aria-pressed", "true")

  const tabs = page.getByRole("tablist", { name: "Open documents" })
  await tabs.getByRole("tab", { name: "getting-started.md" }).click()
  await expect(preview).toHaveAttribute("aria-pressed", "true")
  await tabs.getByRole("tab", { name: "diagrams.md" }).click()
  await expect(source).toHaveAttribute("aria-pressed", "true")

  await tabs.getByRole("button", { name: "Close guides/getting-started.md" }).click()
  await tree.getByRole("treeitem", { name: "getting-started.md" }).click()
  await expect(source).toHaveAttribute("aria-pressed", "true")
  await expect(sourceView).toContainText("# Getting started")
})

test("choosing Preview or Source sets the view new documents open in", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")

  const view = page.getByRole("group", { name: "Document view" })
  const preview = view.getByRole("button", { name: "Preview" })
  const source = view.getByRole("button", { name: "Source" })
  const tree = page.getByRole("complementary", { name: "Files" })
  const tabs = page.getByRole("tablist", { name: "Open documents" })
  await expect(source).toHaveAttribute("aria-pressed", "true")

  await preview.click()
  await tree.getByRole("treeitem", { name: "diagrams.md" }).click()
  await expect(page).toHaveURL(/path=guides%2Fdiagrams\.md/)
  await expect(preview).toHaveAttribute("aria-pressed", "true")
  await expect(page.locator("article")).toBeVisible()

  await tree.getByRole("treeitem", { name: "assets", exact: true }).click()
  await tree.getByRole("treeitem", { name: "preview.svg" }).click()
  await expect(view.getByRole("button")).toHaveText(["Preview", "Source"])
  await expect(preview).toHaveAttribute("aria-pressed", "true")

  await tabs.getByRole("tab", { name: "diagrams.md" }).click()
  await source.click()
  await tree.getByRole("treeitem", { name: "code-blocks.md" }).click()
  await expect(page).toHaveURL(/path=guides%2Fcode-blocks\.md/)
  await expect(source).toHaveAttribute("aria-pressed", "true")
  await expect(page.getByRole("textbox", { name: "Markdown source" })).toContainText("# Code block examples")

  await tabs.getByRole("button", { name: "Close assets/preview.svg" }).click()
  await tree.getByRole("treeitem", { name: "preview.svg" }).click()
  await expect(preview).toHaveAttribute("aria-pressed", "true")
  await expect(page.getByRole("img", { name: "preview.svg" })).toBeVisible()

  await tabs.getByRole("tab", { name: "code-blocks.md" }).click()
  await preview.click()
  await page.reload()
  await expect(page).toHaveURL(/path=guides%2Fcode-blocks\.md/)
  await expect(preview).toHaveAttribute("aria-pressed", "true")
  await tree.getByRole("treeitem", { name: "images.md" }).click()
  await expect(page).toHaveURL(/path=guides%2Fimages\.md/)
  await expect(preview).toHaveAttribute("aria-pressed", "true")
})

test("the Preview choice lasts for the browser tab, per project", async ({ page, context }) => {
  const view = page.getByRole("group", { name: "Document view" })
  const tree = page.getByRole("complementary", { name: "Files" })
  await page.goto(`/view?project=${rootParam}&path=guides%2Fgetting-started.md`)
  await view.getByRole("button", { name: "Preview" }).click()
  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()

  await page.goto(`/?project=${projectParam("guides")}`)
  await tree.getByRole("treeitem", { name: "diagrams.md" }).click()
  await expect(page).toHaveURL(endsWith(`/view?project=${projectParam("guides")}&path=diagrams.md`))
  await expect(view.getByRole("button", { name: "Source" })).toHaveAttribute("aria-pressed", "true")

  await page.goto(`/view?project=${rootParam}&path=guides%2Fdiagrams.md`)
  await expect(view.getByRole("button", { name: "Preview" })).toHaveAttribute("aria-pressed", "true")

  const other = await context.newPage()
  await other.goto(`/view?project=${rootParam}&path=guides%2Fdiagrams.md`)
  await expect(other.getByRole("group", { name: "Document view" }).getByRole("button", { name: "Source" })).toHaveAttribute("aria-pressed", "true")
})

test("resets per-tab document mode after reload", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")

  const view = page.getByRole("group", { name: "Document view" })
  await view.getByRole("button", { name: "Preview" }).click()
  await page.getByRole("complementary", { name: "Files" }).getByRole("treeitem", { name: "diagrams.md" }).click()
  await view.getByRole("button", { name: "Source" }).click()
  await page.getByRole("tablist", { name: "Open documents" }).getByRole("tab", { name: "getting-started.md" }).click()
  await expect(view.getByRole("button", { name: "Preview" })).toHaveAttribute("aria-pressed", "true")

  await page.reload()
  await expect(view.getByRole("button", { name: "Source" })).toHaveAttribute("aria-pressed", "true")
  await expect(page.getByLabel("Markdown source")).toBeVisible()
  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toHaveCount(0)
})

test("shows document metadata and process metrics in the status line", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")

  const status = page.locator(".status-bar")
  await expect(status.locator(".status-path")).toHaveText("guides/getting-started.md")
  await expect(status.locator(".status-path")).toHaveAttribute("title", "guides/getting-started.md")
  await expect(status).toContainText("22 files")
  await expect(status.locator(".status-file-type")).toHaveText("Markdown")
  await status.locator("summary").click()
  if (process.platform === "linux") {
    await expect(status).toContainText("CPU")
    await expect(status).toContainText("Resident RAM")
  } else {
    await expect(status).not.toContainText("CPU")
    await expect(status).toContainText("Go runtime system memory")
  }
})

test("resizes and persists the file tree", async ({ page }) => {
  await page.goto("/")

  const tree = page.getByRole("complementary", { name: "Files" })
  const resizer = page.getByRole("separator", { name: "Resize file tree" })
  const initialWidth = await tree.evaluate((element) => element.getBoundingClientRect().width)
  await resizer.press("ArrowRight")
  await expect.poll(() => tree.evaluate((element) => element.getBoundingClientRect().width)).toBe(initialWidth + 16)

  await page.reload()
  await expect.poll(() => tree.evaluate((element) => element.getBoundingClientRect().width)).toBe(initialWidth + 16)
})

test("applies an editor palette while preserving dark rendering", async ({ page }) => {
  await page.goto("/")

  await (await themeSetting(page)).selectOption({ label: "Gruvbox Dark" })

  await expect(page.locator("html")).toHaveAttribute("data-theme", "gruvbox-dark")
  await expect(page.locator("html")).toHaveClass(/dark/)
  await expect(page.locator("html")).toHaveCSS("background-color", "rgb(29, 32, 33)")
})

test("renders every Mermaid fixture with default Excalidraw", async ({ page }) => {
  const cspFailures: string[] = []
  const fontResponses: number[] = []
  const remoteFontRequests: string[] = []
  page.on("request", (request) => {
    if (request.url().startsWith("https://esm.sh/")) remoteFontRequests.push(request.url())
  })
  page.on("requestfailed", (request) => {
    if (request.failure()?.errorText === "csp") cspFailures.push(request.url())
  })
  page.on("response", (response) => {
    const url = new URL(response.url())
    if (url.pathname.startsWith("/assets/excalidraw/fonts/")) fontResponses.push(response.status())
  })
  await page.goto(`${apiOrigin}/view?path=guides%2Fdiagrams.md`)
  await showPreview(page)
  await revealMermaidCases(page)

  await expect(page.locator("main .mermaid-diagram.ready")).toHaveCount(mermaidCaseCount)
  await expect(page.locator("main .mermaid-excalidraw")).toHaveCount(mermaidCaseCount)
  await expect(page.locator("main .mermaid-excalidraw .excalidraw").first()).toBeVisible()
  await expect(page.locator("main .mermaid-excalidraw [data-testid='main-menu-trigger']").first()).toBeHidden()
  await expect(page.locator("main .mermaid-excalidraw .zoom-actions").first()).toBeHidden()
  await expect(page.locator("main .tl-container")).toHaveCount(0)
  await expect.poll(() => fontResponses.some((status) => status === 200)).toBe(true)
  expect(cspFailures).toEqual([])
  expect(remoteFontRequests).toEqual([])

  const canvas = page.locator("main .mermaid-excalidraw canvas").first()
  await canvas.scrollIntoViewIfNeeded()
  const bounds = await canvas.boundingBox()
  if (!bounds) throw new Error("Excalidraw canvas has no bounds")
  const pointer = {
    x: Math.round(bounds.x + bounds.width / 3),
    y: Math.round(bounds.y + bounds.height / 3),
  }
  await page.evaluate(() => {
    const capturePointer = (event: PointerEvent) => {
      if (event.isTrusted) return
      document.documentElement.dataset.excalidrawPointer = `${event.clientX},${event.clientY}`
      document.removeEventListener("pointermove", capturePointer)
    }
    document.addEventListener("pointermove", capturePointer)
  })
  await canvas.dispatchEvent("wheel", {
    bubbles: true,
    cancelable: true,
    clientX: pointer.x,
    clientY: pointer.y,
    ctrlKey: true,
    deltaY: -1,
  })
  await expect(page.locator("html")).toHaveAttribute("data-excalidraw-pointer", `${pointer.x},${pointer.y}`)
})

test("uses tldraw for Mermaid when the feature is enabled", async ({ page }) => {
  await page.goto(`${tldrawApiOrigin}/view?path=guides%2Fdiagrams.md`)
  await showPreview(page)
  await revealMermaidCases(page)

  await expect(page.locator("main .mermaid-diagram.ready")).toHaveCount(mermaidCaseCount)
  await expect(page.locator("main .tl-container")).toHaveCount(mermaidCaseCount)
  await expect(page.locator("main .mermaid-excalidraw")).toHaveCount(0)
})
