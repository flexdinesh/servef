import { expect, test, type Page } from "@playwright/test"

const mermaidCaseCount = 4

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

  await page.goto("http://127.0.0.1:18080/view?path=guides%2Fgetting-started.md")

  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
  expect(pageRequests).toBe(0)
})

test("browses nested Markdown without resetting the file tree", async ({ page }) => {
  await page.goto("/")

  const tree = page.getByRole("complementary", { name: "Markdown files" })
  await tree.getByText("guides/").click()
  await tree.getByText("reference/").click()
  await tree.getByText("topics/").click()
  await expect(tree.getByRole("link", { name: "getting-started.md" })).toBeVisible()
  await expect(tree.getByRole("link", { name: "api.markdown" })).toBeVisible()
  await expect(tree.getByRole("link", { name: "search.md" })).toBeVisible()
  await expect(tree.getByText("guides/", { exact: true }).locator("svg"))
    .toHaveClass(/lucide-folder/)
  await expect(tree.getByRole("link", { name: "getting-started.md" }).locator("svg"))
    .toHaveClass(/lucide-square-m/)
  await expect(tree.getByRole("link", { name: "api.markdown" }).locator("svg"))
    .toHaveAttribute("aria-hidden", "true")
  await expect(tree.getByText("notes.txt")).toHaveCount(0)
  await expect(tree.getByText("ignored.md")).toHaveCount(0)

  const treeScroll = await tree.evaluate((element) => {
    element.style.height = "80px"
    element.scrollTop = 40
    element.addEventListener("click", () => {
      element.dataset.scrollAtNavigation = String(element.scrollTop)
    }, { capture: true, once: true })
    return element.scrollTop
  })
  expect(treeScroll).toBeGreaterThan(0)

  await page.evaluate(() => {
    document.body.dataset.navigationMarker = "preserved"
  })
  await tree.getByRole("link", { name: "getting-started.md" }).click()
  await expect(page).toHaveURL(/\/view\?path=guides%2Fgetting-started\.md$/)
  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
  await expect(page.locator("main table")).toContainText("pnpm dev")
  await expect(page).toHaveTitle("guides/getting-started.md · servef")
  await expect(tree.getByRole("link", { name: "getting-started.md" })).toHaveAttribute("aria-current", "page")
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
  await expect.poll(() => tree.evaluate((element) => (
    element.dataset.scrollAtNavigation === String(element.scrollTop)
  ))).toBe(true)
  await expect(tree.getByRole("link", { name: "api.markdown" })).toBeVisible()
  await expect(tree.getByRole("link", { name: "search.md" })).toBeVisible()
})

test("keeps modified tree clicks native", async ({ context, page }) => {
  await page.goto("/")

  const tree = page.getByRole("complementary", { name: "Markdown files" })
  await tree.getByText("guides/").click()
  const openedPage = context.waitForEvent("page")
  await tree.getByRole("link", { name: "getting-started.md" }).click({ modifiers: ["Control"] })
  const documentPage = await openedPage

  await expect(page).toHaveURL(/\/$/)
  await expect(documentPage).toHaveURL(/\/view\?path=guides%2Fgetting-started\.md$/)
  await documentPage.close()
})

test("searches fixture content and opens the result", async ({ page }) => {
  await page.goto("/")

  const search = page.getByPlaceholder("Search files and content…")
  await expect(page.getByText("Select a Markdown file from the folder tree.")).toBeVisible()
  await page.locator("body").press("ControlOrMeta+k")
  await expect(search).toBeVisible()
  await search.fill("luminous-orchid")

  const result = page.getByRole("option").filter({ hasText: "search.md" })
  await expect(result).toBeVisible()
  await page.evaluate(() => {
    document.body.dataset.navigationMarker = "preserved"
  })
  await result.click()
  await expect(page).toHaveURL(/\/view\?path=reference%2Ftopics%2Fsearch\.md$/)
  await expect(page.locator("main")).toContainText("luminous-orchid")
  await expect(page.locator("main")).toBeFocused()
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
})

test("navigates Markdown links and history without losing tree state", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")

  const tree = page.getByRole("complementary", { name: "Markdown files" })
  await tree.getByText("reference/").click()
  await tree.getByText("topics/").click()
  await expect(tree.getByRole("link", { name: "search.md" })).toBeVisible()

  await page.evaluate(() => {
    document.body.dataset.navigationMarker = "preserved"
  })
  await page.getByRole("link", { name: "search notes" }).click()
  await expect(page).toHaveURL(/\/view\?path=reference%2Ftopics%2Fsearch\.md$/)
  await expect(page.getByRole("heading", { level: 1, name: "Search notes" })).toBeVisible()
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
  await expect(tree.getByRole("link", { name: "getting-started.md" })).toBeVisible()
  await expect(tree.getByRole("link", { name: "search.md" })).toBeVisible()

  await page.goBack()
  await expect(page).toHaveURL(/\/view\?path=guides%2Fgetting-started\.md$/)
  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
  await expect(tree.getByRole("link", { name: "search.md" })).toBeVisible()

  await page.goForward()
  await expect(page).toHaveURL(/\/view\?path=reference%2Ftopics%2Fsearch\.md$/)
  await expect(page.getByRole("heading", { level: 1, name: "Search notes" })).toBeVisible()
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
  await expect(tree.getByRole("link", { name: "getting-started.md" })).toBeVisible()
})

test("preserves Markdown hashes during client navigation", async ({ page }) => {
  await page.goto("/view?path=reference%2Fapi.markdown")

  await page.evaluate(() => {
    document.body.dataset.navigationMarker = "preserved"
  })
  await page.getByRole("link", { name: "diagrams" }).click()

  await expect(page).toHaveURL(/\/view\?path=guides%2Fdiagrams\.md#request-flow$/)
  await expect(page.getByRole("heading", { level: 2, name: "Request flow" })).toBeInViewport()
  await expect(page.locator("body")).toHaveAttribute("data-navigation-marker", "preserved")
})

test("recovers from an invalid page response", async ({ page }) => {
  let returnInvalidResponse = true
  await page.route("**/api/page?path=guides%2Fgetting-started.md", async (route) => {
    if (returnInvalidResponse) {
      returnInvalidResponse = false
      await route.fulfill({ contentType: "application/json", body: "{}" })
      return
    }
    await route.continue()
  })
  await page.goto("/")

  await page.locator("body").press("ControlOrMeta+k")
  const search = page.getByPlaceholder("Search files and content…")
  await search.fill("getting-started")
  await page.getByRole("option", {
    exact: true,
    name: "getting-started.md guides/getting-started.md",
  }).click()

  await expect(page.getByRole("alert")).toContainText("invalid page response")
  await page.getByRole("button", { name: "Try again" }).click()
  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
})

test("commits only the latest rapid navigation", async ({ page }) => {
  let releaseDelayedResponse = () => {}
  const delayedResponse = new Promise<void>((resolve) => {
    releaseDelayedResponse = resolve
  })
  await page.route("**/api/page?path=guides%2Fdiagrams.md", async (route) => {
    await delayedResponse
    await route.continue().catch(() => {})
  })
  await page.goto("/")

  const tree = page.getByRole("complementary", { name: "Markdown files" })
  await tree.getByText("guides/").click()
  await tree.getByRole("link", { name: "diagrams.md" }).dispatchEvent("click")
  await tree.getByRole("link", { name: "getting-started.md" }).dispatchEvent("click")

  await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
  releaseDelayedResponse()
  await page.waitForTimeout(100)
  await expect(page).toHaveURL(/\/view\?path=guides%2Fgetting-started\.md$/)
})

test("opens search from the header control", async ({ page }) => {
  await page.goto("/")

  await page.getByRole("button", { name: /Search/ }).click()
  await expect(page.getByPlaceholder("Search files and content…")).toBeFocused()
})

test("selects and persists an explicit theme", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/")

  const theme = page.getByRole("button", { name: "Theme: System" })
  await theme.press("Enter")
  const dark = page.getByRole("menuitemradio", { name: "Dark", exact: true })
  await expect(dark).toBeVisible()
  await dark.click()

  await expect(page.locator("html")).toHaveClass(/dark/)
  await expect(page.locator("html")).toHaveCSS("background-color", "rgb(16, 18, 23)")
  expect(await page.evaluate(() => localStorage.getItem("servef-theme"))).toBe("dark")

  await page.reload()
  await expect(page.getByRole("button", { name: "Theme: Dark" })).toBeVisible()
  await expect(page.locator("html")).toHaveClass(/dark/)

  await page.getByRole("button", { name: "Theme: Dark" }).click()
  await page.getByRole("menuitemradio", { name: "Light", exact: true }).click()
  await expect(page.locator("html")).toHaveClass(/light/)
  await expect(page.locator("html")).toHaveCSS("background-color", "rgb(247, 248, 250)")
})

test("system theme follows live OS changes", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/")

  await expect(page.getByRole("button", { name: "Theme: System" })).toBeVisible()
  await expect(page.locator("html")).toHaveClass(/light/)
  await page.emulateMedia({ colorScheme: "dark" })
  await expect(page.locator("html")).toHaveClass(/dark/)
  await expect(page.getByRole("button", { name: "Theme: System" })).toBeVisible()
})

test("uses readable body and navigation text sizes", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")

  const tree = page.getByRole("complementary", { name: "Markdown files" })
  const selected = tree.getByRole("link", { name: "getting-started.md" })
  await expect(page.locator("article")).toHaveCSS("font-size", "16px")
  await expect(selected).toHaveCSS("font-size", "13px")
  await expect(selected).toHaveCSS("border-radius", "0px")
})

test("highlights fenced code with the selected theme", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" })
  await page.goto("/view?path=reference%2Fapi.markdown")

  const keyword = page.locator(".syntax-highlight [class^='syntax-k']").first()
  await expect(keyword).toHaveText("func")
  await expect(keyword).toHaveCSS("color", "rgb(207, 34, 46)")

  await page.getByRole("button", { name: "Theme: System" }).click()
  await page.getByRole("menuitemradio", { name: "Nord" }).click()
  await expect(keyword).toHaveCSS("color", "rgb(129, 161, 193)")
})

test("opens, switches, and closes document tabs", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")

  const tree = page.getByRole("complementary", { name: "Markdown files" })
  await tree.getByRole("link", { name: "diagrams.md" }).click()

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

test("shows document metadata and process metrics in the status line", async ({ page }) => {
  await page.goto("/view?path=guides%2Fgetting-started.md")

  const status = page.locator(".status-bar")
  await expect(status).toContainText("guides/getting-started.md")
  await expect(status).toContainText("5 files")
  if (process.platform === "linux") {
    await expect(status).toContainText("CPU")
    await expect(status).toContainText("RAM")
  } else {
    await expect(status).not.toContainText("CPU")
    await expect(status).toContainText("MEM")
  }
})

test("resizes and persists the file tree", async ({ page }) => {
  await page.goto("/")

  const tree = page.getByRole("complementary", { name: "Markdown files" })
  const resizer = page.getByRole("separator", { name: "Resize file tree" })
  const initialWidth = await tree.evaluate((element) => element.getBoundingClientRect().width)
  await resizer.press("ArrowRight")
  await expect.poll(() => tree.evaluate((element) => element.getBoundingClientRect().width)).toBe(initialWidth + 16)

  await page.reload()
  await expect.poll(() => tree.evaluate((element) => element.getBoundingClientRect().width)).toBe(initialWidth + 16)
})

test("applies an editor palette while preserving dark rendering", async ({ page }) => {
  await page.goto("/")

  await page.getByRole("button", { name: "Theme: System" }).click()
  await page.getByRole("menuitemradio", { name: "Gruvbox Dark" }).click()

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
  await page.goto("http://127.0.0.1:18080/view?path=guides%2Fdiagrams.md")
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
  await page.goto("http://127.0.0.1:18081/view?path=guides%2Fdiagrams.md")
  await revealMermaidCases(page)

  await expect(page.locator("main .mermaid-diagram.ready")).toHaveCount(mermaidCaseCount)
  await expect(page.locator("main .tl-container")).toHaveCount(mermaidCaseCount)
  await expect(page.locator("main .mermaid-excalidraw")).toHaveCount(0)
})
