import { expect, test, type Page } from "@playwright/test"
import { endsWith, rootParam } from "./fixture-root.ts"
import { apiOrigin } from "./ports.ts"

const mermaidCaseCount = 5
const mermaidDiagramCount = 4

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

test("searches fixture content and opens a match at its source line", async ({ page }) => {
  await page.goto("/")

  const search = page.getByRole("textbox", { name: "Search", exact: true })
  await expect(page.getByRole("tab", { name: "Welcome" })).toHaveAttribute("aria-selected", "true")
  await page.locator(".status-bar").getByRole("button", { name: /^Find in files/ }).press("ControlOrMeta+Shift+f")
  await expect(search).toBeFocused()
  await search.fill("luminous-orchid")

  const file = page.getByRole("tree", { name: "Search results" }).getByRole("treeitem").filter({ hasText: "search.md" })
  await expect(file).toHaveAttribute("aria-expanded", "true")
  const match = page.getByRole("tree", { name: "Search results" }).getByRole("treeitem").filter({ hasText: "phrase **luminous-orchid**" })
  await page.evaluate(() => {
    document.body.dataset.navigationMarker = "preserved"
  })
  // Click previews the match and keeps focus in the results, as in VS Code.
  await match.click()
  await expect(page).toHaveURL(endsWith(`/view?project=${rootParam}&path=reference%2Ftopics%2Fsearch.md#L3`))
  await expect(page.getByRole("textbox", { name: "Markdown source" })).toContainText("luminous-orchid")
  await expect(match).toBeFocused()
  await expect(match).toHaveAttribute("aria-selected", "true")
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
  // Enter opens the match and moves focus to the document.
  await match.press("Enter")
  await expect(page.locator("main")).toBeFocused()
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

  await page.locator(".status-bar").getByRole("button", { name: /^Find in files/ }).press("ControlOrMeta+Shift+f")
  const search = page.getByRole("textbox", { name: "Search", exact: true })
  await search.fill("exercise GitHub")
  await page.getByRole("tree", { name: "Search results" }).getByRole("treeitem").filter({ hasText: "exercise GitHub" }).press("Enter")

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

// themeSetting opens Settings and returns its Theme select.
async function themeSetting(page: Page) {
  await page.getByRole("button", { name: "Settings", exact: true }).click()
  return page.getByRole("dialog", { name: "Settings" }).getByRole("combobox", { name: "Appearance: Theme" })
}

test("system theme follows live OS changes", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.addInitScript(() => window.localStorage.setItem("servef-theme", "system"))
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
  await expect(options).toHaveCount(13)
  await expect(options.nth(0)).toHaveAccessibleName(/^Preferences: Open Settings, (Ctrl\+|⌘),$/)
  await expect(options.nth(1)).toHaveAccessibleName("Preferences: Color Theme, Servef Dark")
  await expect(options.nth(2)).toHaveAccessibleName("Settings: Theme, Servef Dark")
  await expect(options.nth(3)).toHaveAccessibleName("Settings: Font Size, 13px")
  await expect(options.nth(4)).toHaveAccessibleName("Settings: Language Servers, On")
  await expect(options.nth(5)).toHaveAccessibleName("Settings: Show Welcome on Startup, On")
  await expect(options.nth(6)).toHaveAccessibleName("Navigation: Go to Definition, F12")
  await expect(options.nth(7)).toHaveAccessibleName("Navigation: Find All References, Shift+F12")
  await expect(options.nth(8)).toHaveAccessibleName(/^Walkthrough: Next Step, (Ctrl\+Right|⌘→)$/)
  await expect(options.nth(9)).toHaveAccessibleName(/^Walkthrough: Previous Step, (Ctrl\+Left|⌘←)$/)
  await expect(options.nth(10)).toHaveAccessibleName("Walkthrough: Present")
  await expect(options.nth(11)).toHaveAccessibleName("Walkthrough: Close Walkthrough")
  await expect(options.nth(12)).toHaveAccessibleName("Help: Welcome")
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
  await page.keyboard.press("ArrowDown")
  await page.keyboard.press("Enter")
  await expect(settings.getByRole("combobox", { name: "Appearance: Theme" })).toBeFocused()

  await page.keyboard.press("Escape")
  await page.keyboard.press("ControlOrMeta+Shift+P")
  await page.keyboard.press("Enter")
  await expect(settings.getByRole("searchbox", { name: "Search settings" })).toBeFocused()
  await expect(settings.locator("[data-revealed]")).toHaveCount(0)
})

test("highlights Source in the browser with the selected theme", async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem("servef-theme", "light"))
  await page.goto("/view?path=code%2Fmain.go")
  const source = page.getByRole("textbox", { name: "Go source" })
  // Highlights keeps trailing whitespace in a token.
  const keyword = source.locator(".hl-keyword").filter({ hasText: /^package\s*$/ })
  await expect(keyword).toHaveCSS("color", "rgb(207, 34, 46)")
  await expect(source.locator(".hl-comment").first()).toHaveCSS("font-style", "italic")
  // Highlighting never changes the text navigation and copy read.
  await expect(source.locator('[data-line="2"] .source-document-line-content')).toHaveText("package main")

  await (await themeSetting(page)).selectOption({ label: "Nord" })
  await expect(keyword).toHaveCSS("color", "rgb(182, 144, 175)")
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
  await expect(sourceLines.nth(2)).toHaveCSS("height", "22px")
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
  // Wait for the new document; the toggle applies to the one displayed.
  await expect(page.getByRole("heading", { level: 1, name: "Mermaid diagrams" })).toBeVisible()
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

test("renders every Mermaid fixture with ondine", async ({ page }) => {
  const cspFailures: string[] = []
  const moduleRequests: string[] = []
  const remoteRequests: string[] = []
  const excalidrawRequests: string[] = []
  page.on("request", (request) => {
    const url = new URL(request.url())
    if ((url.protocol === "http:" || url.protocol === "https:") && url.origin !== apiOrigin) remoteRequests.push(request.url())
    if (url.pathname.endsWith(".wasm")) moduleRequests.push(url.pathname)
    if (url.pathname.includes("excalidraw")) excalidrawRequests.push(url.pathname)
  })
  page.on("requestfailed", (request) => {
    if (request.failure()?.errorText === "csp") cspFailures.push(request.url())
  })
  page.on("console", (message) => {
    if (message.type() === "error" && message.text().includes("Content Security Policy")) cspFailures.push(message.text())
  })
  await page.goto(`${apiOrigin}/view?path=guides%2Fdiagrams.md`)
  await showPreview(page)
  await revealMermaidCases(page)

  const diagrams = page.locator("main .mermaid-diagram")
  await expect(diagrams.locator("svg.ondine")).toHaveCount(mermaidDiagramCount)
  for (const type of ["flowchart", "sequence", "state", "gitGraph"]) {
    await expect(diagrams.locator(`svg.ondine-${type}`), type).toHaveCount(1)
  }
  await expect(page.locator("main .mermaid-error")).toHaveCount(1)
  await expect(page.locator("main .mermaid-error")).toContainText("Parse error on line 3:")
  await expect(page.locator("main .mermaid-error")).toContainText("B --> [broken")
  await expect(diagrams.locator("a")).toHaveCount(0)
  await expect(diagrams.locator("svg.ondine-flowchart")).toContainText("Browser")
  expect(moduleRequests).toHaveLength(1)
  expect(excalidrawRequests).toEqual([])
  expect(cspFailures).toEqual([])
  expect(remoteRequests).toEqual([])
})

test("opens Mermaid files as diagrams with Source available", async ({ page }) => {
  await page.goto(`${apiOrigin}/view?path=guides%2Frequest-flow.mmd`)

  const modes = page.getByRole("group", { name: "Document view" })
  await expect(modes.getByRole("button")).toHaveText(["Preview", "Source"])
  await expect(modes.getByRole("button", { name: "Preview", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(page.locator("main .mermaid-diagram svg.ondine-flowchart")).toBeVisible()
  await expect(page.locator("main .mermaid-diagram svg.ondine-flowchart")).toContainText("Themed SVG")
  await expect(page.locator(".status-bar .status-file-type")).toHaveText("Mermaid")

  await modes.getByRole("button", { name: "Source", exact: true }).click()
  await expect(page.locator("main .mermaid-diagram")).toHaveCount(0)
  await expect(page.locator("main")).toContainText("Open[Open request-flow.mmd] --> Preview{Preview?}")
})

test("expands a Mermaid diagram with zoom and pan", async ({ page }) => {
  await page.goto(`${apiOrigin}/view?path=guides%2Fdiagrams.md`)
  await showPreview(page)
  const diagram = page.locator("main .mermaid-diagram").first()
  await expect(diagram.locator("svg.ondine")).toBeVisible()
  await diagram.getByRole("button", { name: "Expand diagram" }).click()

  const dialog = page.getByRole("dialog", { name: "Mermaid diagram" })
  await expect(dialog).toBeVisible()
  await expect(dialog.locator("svg.ondine-flowchart")).toBeVisible()
  await expect(dialog.locator("a")).toHaveCount(0)
  const zoom = dialog.getByRole("status")
  await expect(zoom).toHaveText("100%")
  await dialog.getByRole("button", { name: "Zoom in" }).click()
  await expect(zoom).toHaveText("125%")
  await dialog.getByRole("button", { name: "Zoom out" }).click()
  await dialog.getByRole("button", { name: "Zoom out" }).click()
  await expect(zoom).toHaveText("80%")

  const viewer = dialog.locator(".mermaid-viewer")
  const bounds = await viewer.boundingBox()
  if (!bounds) throw new Error("Mermaid viewer has no bounds")
  const center = { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 }
  await page.mouse.move(center.x, center.y)
  await page.keyboard.down("Control")
  await page.mouse.wheel(0, -100)
  await page.keyboard.up("Control")
  await expect(zoom).toHaveText("100%")

  const content = dialog.locator(".mermaid-viewer-content")
  const before = await content.boundingBox()
  await page.mouse.down()
  await page.mouse.move(center.x + 60, center.y + 40, { steps: 4 })
  await page.mouse.up()
  const after = await content.boundingBox()
  if (!before || !after) throw new Error("Mermaid viewer content has no bounds")
  expect(Math.round(after.x - before.x)).toBe(60)
  expect(Math.round(after.y - before.y)).toBe(40)

  await dialog.getByRole("button", { name: "Reset zoom" }).click()
  await expect(zoom).toHaveText("100%")
  await page.keyboard.press("Escape")
  await expect(dialog).toHaveCount(0)
  await expect(diagram.locator("svg.ondine")).toBeVisible()
})
