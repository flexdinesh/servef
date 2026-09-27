import { expect, test } from "@playwright/test"

for (const entry of [
  { name: "development", url: "/" },
  { name: "embedded binary", url: "http://127.0.0.1:18080/" },
]) {
  test(`search retains query and results across document navigation (${entry.name})`, async ({ page }) => {
    let indexRequests = 0
    page.on("request", (request) => {
      if (new URL(request.url()).pathname === "/api/search-documents") indexRequests++
    })
    await page.goto(entry.url)
    await page.getByRole("button", { name: /Search/ }).click()
    const input = page.getByPlaceholder("Search files and content…")
    await input.fill("getting-started")
    const result = page.getByRole("option", { exact: true, name: "getting-started.md guides/getting-started.md" })
    await result.click()
    await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
    await expect(page.locator("main")).toBeFocused()

    await page.getByRole("button", { name: /Search/ }).click()
    await expect(input).toHaveValue("getting-started")
    await expect(input).toBeFocused()
    await expect(result).toBeVisible()
    expect(indexRequests).toBe(1)
    await input.press("Escape")
    await expect(input).toBeHidden()
  })
}

test("search retries transient failure without losing the query", async ({ page }) => {
  let requests = 0
  await page.route("**/api/search-documents", async (route) => {
    if (++requests === 1) {
      await route.fulfill({ status: 503, contentType: "application/json", body: "{}" })
    } else {
      await route.continue()
    }
  })
  await page.goto("/")
  await page.getByRole("button", { name: /Search/ }).click()
  const input = page.getByPlaceholder("Search files and content…")
  await input.fill("getting-started")
  await expect(page.locator(".search-status")).toContainText("Search is unavailable.")
  await page.getByRole("button", { name: "Retry search" }).click()
  await expect(input).toHaveValue("getting-started")
  await expect(input).toBeFocused()
  await expect(page.getByRole("option", { exact: true, name: "getting-started.md guides/getting-started.md" })).toBeVisible()
  expect(requests).toBe(2)
})

test("search survives unavailable Worker and recovers when support returns", async ({ page }) => {
  await page.addInitScript(() => {
    const originalWorker = window.Worker
    Object.defineProperty(window, "Worker", { configurable: true, get: () => {
      if (document.documentElement.dataset.restoreWorker === "true") return originalWorker
      throw new Error("Worker unavailable")
    } })
  })
  await page.goto("/")
  await page.getByRole("button", { name: /Search/ }).click()
  await expect(page.locator(".search-status")).toContainText("Search is unavailable.")
  await page.evaluate(() => { document.documentElement.dataset.restoreWorker = "true" })
  await page.getByPlaceholder("Search files and content…").fill("getting-started")
  await page.getByRole("button", { name: "Retry search" }).click()
  await expect(page.getByRole("option").first()).toBeVisible()
})

for (const entry of [
  { name: "development", url: "/view?path=guides%2Fgetting-started.md" },
  { name: "embedded binary", url: "http://127.0.0.1:18080/view?path=guides%2Fgetting-started.md" },
]) {
  test(`document scroll survives tabs, Home, Back, and Forward (${entry.name})`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 400 })
    await page.goto(entry.url)
    const pane = page.locator("#document-pane")
    await expect(page.getByRole("heading", { level: 1, name: "Getting started" })).toBeVisible()
    const firstScroll = await pane.evaluate((element) => {
      element.scrollTop = 220
      return element.scrollTop
    })
    expect(firstScroll).toBeGreaterThan(0)
    const tree = page.getByRole("complementary", { name: "Markdown files" })
    await tree.getByRole("link", { name: "code-blocks.md" }).click()
    await expect(page.getByRole("tab", { name: "code-blocks.md" })).toHaveAttribute("aria-selected", "true")
    await expect.poll(() => pane.evaluate((element) => element.scrollTop)).toBe(0)
    const secondScroll = await pane.evaluate((element) => {
      element.scrollTop = 150
      return element.scrollTop
    })
    expect(secondScroll).toBeGreaterThan(0)
    await page.getByRole("tab", { name: "getting-started.md" }).click()
    await expect.poll(() => pane.evaluate((element) => element.scrollTop)).toBe(firstScroll)

    await page.goBack()
    await expect(page.getByRole("tab", { name: "code-blocks.md" })).toHaveAttribute("aria-selected", "true")
    await expect.poll(() => pane.evaluate((element) => element.scrollTop)).toBe(secondScroll)
    await page.goForward()
    await expect.poll(() => pane.evaluate((element) => element.scrollTop)).toBe(firstScroll)

    await page.getByRole("link", { name: "servef home" }).click()
    await expect(page.getByRole("heading", { name: "Choose a Markdown file" })).toBeVisible()
    await page.goBack()
    await expect.poll(() => pane.evaluate((element) => element.scrollTop)).toBe(firstScroll)

    await page.getByRole("button", { name: "Source", exact: true }).click()
    await expect.poll(() => pane.evaluate((element) => element.scrollTop)).toBe(0)
    await page.getByRole("button", { name: "Close guides/getting-started.md" }).click()
    await expect(page.getByRole("tab", { name: "code-blocks.md" })).toHaveAttribute("aria-selected", "true")
    await tree.getByRole("link", { name: "getting-started.md" }).click()
    await expect(page.getByRole("button", { name: "Preview", exact: true })).toHaveAttribute("aria-pressed", "true")
    await expect.poll(() => pane.evaluate((element) => element.scrollTop)).toBe(0)
  })
}

test("metrics polling stays single-flight and retains samples during failure", async ({ page }) => {
  await page.clock.install()
  let releaseFirst: (() => void) | undefined
  const firstResponse = new Promise<void>((resolve) => { releaseFirst = resolve })
  let requests = 0
  await page.route("**/api/metrics", async (route) => {
    const request = ++requests
    if (request === 1) await firstResponse
    await route.fulfill({ status: request === 2 ? 503 : 200, contentType: "application/json",
      body: JSON.stringify({ cpuUsage: request === 1 ? 11 : 22, goroutines: 2, memoryBytes: 200, memorySource: "rss" }) })
  })
  await page.goto("/")
  await expect.poll(() => requests).toBe(1)
  await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")))
  await page.clock.runFor(5_000)
  expect(requests).toBe(1)
  if (!releaseFirst) throw new Error("Metrics request not started")
  releaseFirst()
  await expect(page.locator(".status-metrics")).toContainText("11.0%")
  await page.clock.runFor(2_500)
  await expect.poll(() => requests).toBe(2)
  await expect(page.locator(".status-metrics")).toContainText("11.0%")
  await page.clock.runFor(2_500)
  await expect(page.locator(".status-metrics")).toContainText("22.0%")
})

test("diagram module failure offers reload and recovers", async ({ page }) => {
  let blocked = true
  await page.route("**/src/ExcalidrawMermaidCanvas.tsx", async (route) => {
    if (blocked) await route.abort("failed")
    else await route.continue()
  })
  await page.goto("/view?path=guides%2Fdiagrams.md")
  const reload = page.getByRole("button", { name: "Reload diagrams" }).first()
  await expect(reload).toBeVisible()
  await expect(page.locator(".mermaid-error").first()).toContainText("Reload to try again.")
  await expect(page.getByRole("link", { name: "getting-started.md" })).toBeVisible()
  blocked = false
  await reload.click()
  await expect(page.locator(".mermaid-diagram.ready").first()).toBeVisible()
  await expect(page.locator(".mermaid-error")).toHaveCount(0)
})

test("metrics aborts pending requests when its view unmounts", async ({ page }) => {
  let release: (() => void) | undefined
  const response = new Promise<void>((resolve) => { release = resolve })
  let pendingRequests = 0
  await page.route("**/api/metrics", async (route) => {
    pendingRequests++
    await response
    await route.fulfill({ contentType: "application/json", body: "{}" }).catch(() => {})
  })
  await page.goto("/test-fixtures/markdown-document.html")
  await expect(page.locator(".status-bar")).toBeVisible()
  await expect.poll(() => pendingRequests).toBeGreaterThan(0)
  const aborted = page.waitForEvent("requestfailed", {
    predicate: (request) => new URL(request.url()).pathname === "/api/metrics",
  })
  await page.getByRole("button", { name: "Toggle document" }).click()
  await aborted
  if (!release) throw new Error("Metrics request not started")
  release()
  await expect(page.locator(".status-bar")).toHaveCount(0)
})

test("Markdown portals survive Strict Mode replay, replacement, and remount", async ({ page }) => {
  const errors: string[] = []
  page.on("pageerror", (error) => errors.push(error.message))
  await page.goto("/test-fixtures/markdown-document.html")
  await expect(page.locator(".mermaid-lazy")).toHaveCount(1)
  await expect(page.locator(".code-block")).toHaveCount(1)
  await expect(page.getByRole("button", { name: "Copy code", exact: true })).toHaveCount(1)

  await page.getByRole("button", { name: "Update Markdown" }).click()
  await expect(page.getByRole("heading", { name: "Replacement" })).toBeVisible()
  await expect(page.locator(".mermaid-lazy")).toHaveCount(0)
  await expect(page.locator(".code-block")).toHaveCount(1)
  await expect(page.getByRole("button", { name: "Copy json code" })).toHaveCount(1)
  await expect(page.locator(".code-block code")).toHaveText('{"updated":true}')
  await page.getByRole("button", { name: "Copy json code" }).click()
  await expect(page.locator(".code-block").getByRole("status")).toHaveText("Copied")

  await page.getByRole("button", { name: "Toggle document" }).click()
  await expect(page.locator("article")).toHaveCount(0)
  await page.getByRole("button", { name: "Toggle document" }).click()
  await expect(page.locator(".code-block")).toHaveCount(1)
  await expect(page.getByRole("button", { name: "Copy json code" })).toHaveCount(1)
  expect(errors).toEqual([])
})

test("search keeps its session while callbacks change", async ({ page }) => {
  let indexRequests = 0
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/search-documents") indexRequests++
  })
  await page.goto("/test-fixtures/markdown-document.html")
  const input = page.getByPlaceholder("Search files and content…")
  await page.getByRole("button", { name: "Open search" }).click()
  await input.fill("getting-started")
  const result = page.getByRole("option", { exact: true, name: "getting-started.md guides/getting-started.md" })
  await expect(result).toBeVisible()
  await input.press("Escape")
  await page.getByRole("button", { name: "Update navigation" }).click()
  await page.getByRole("button", { name: "Open search" }).click()
  await expect(input).toHaveValue("getting-started")
  await result.click()
  await expect(page.getByLabel("Navigation")).toHaveText("updated:guides/getting-started.md")
  expect(indexRequests).toBe(1)
})

test("sidebar pointer resize clamps and double click resets width", async ({ page }) => {
  await page.goto("/")
  const separator = page.getByRole("separator", { name: "Resize file tree" })
  const bounds = await separator.boundingBox()
  if (!bounds) throw new Error("Sidebar separator not found")
  await page.mouse.move(bounds.x, bounds.y + 50)
  await page.mouse.down()
  await page.mouse.move(bounds.x + 500, bounds.y + 50)
  await page.mouse.up()
  await expect(separator).toHaveAttribute("aria-valuenow", "480")
  await separator.dblclick()
  await expect(separator).toHaveAttribute("aria-valuenow", "288")
  expect(await page.evaluate(() => localStorage.getItem("servef-sidebar-width"))).toBe("288")
})
