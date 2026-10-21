import { existsSync } from "node:fs"
import path from "node:path"

import { expect, test, type Page } from "@playwright/test"

import { rootParam } from "./fixture-root.ts"
import { featureSubject, gitFixtureChanges, gitFixtureRoot } from "./git-fixture.ts"
import { gitApiOrigin } from "./ports.ts"

// The fixture repository is built by the git server's webServer command.
test.skip(() => !existsSync(path.join(gitFixtureRoot, ".git")), "git is not installed")

const project = encodeURIComponent(gitFixtureRoot)

function sourceControl(page: Page) {
  return page.getByRole("region", { name: "Source Control" })
}

function changeRow(page: Page, name: string) {
  return sourceControl(page).getByRole("treeitem", { name, exact: true })
}

function diffLines(page: Page, type: "change-addition" | "change-deletion") {
  return page.locator(`.diff-view [data-line-type="${type}"]`)
}

async function openDiff(page: Page, file: string) {
  await page.goto(`${gitApiOrigin}/view?project=${project}&path=${encodeURIComponent(file)}&diff=1`)
}

test("Source Control lists every pending change and opens diffs in their own tabs", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  const activity = page.getByRole("button", { name: `Source Control, ${gitFixtureChanges} pending changes` })
  await expect(activity).toHaveAttribute("aria-pressed", "false")
  await expect(page.locator(".activity-badge")).toHaveText(String(gitFixtureChanges))

  // Explorer rows carry git status; gitignored files are not changes.
  const explorer = page.getByRole("complementary", { name: "Files" })
  await expect(explorer.locator('[data-item-path="README.md"]')).toHaveAttribute("data-item-git-status", "modified")
  await expect(explorer.locator('[data-item-path="debug.log"]')).not.toHaveAttribute("data-item-git-status", /./)

  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(activity).toHaveAttribute("aria-pressed", "true")
  await expect(sourceControl(page).getByRole("heading", { name: `Changes, ${gitFixtureChanges} changes` })).toBeVisible()
  for (const name of ["README.md", "added.md", "logo.bin", "start.md", "removed.md", "app.ts", "untracked.txt", "large.txt"]) {
    await expect(changeRow(page, name)).toBeVisible()
  }
  await expect(changeRow(page, "debug.log")).toHaveCount(0)
  await expect(changeRow(page, "clean.md")).toHaveCount(0)
  await expect(sourceControl(page).locator('[data-item-path="guides/start.md"]')).toHaveAttribute("data-item-git-status", "renamed")

  await changeRow(page, "README.md").click()
  const tab = page.getByRole("tab", { name: "README.md (Working Tree)" })
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/[?&]diff=1/)
  await expect(diffLines(page, "change-addition").filter({ hasText: "Four" })).toBeVisible()
  await expect(diffLines(page, "change-deletion").filter({ hasText: "Two" })).toBeVisible()
  await expect(page.locator(".status-diff-stats")).toHaveText("+2 −1")
  await expect(page).toHaveTitle(/README\.md \(Working Tree\)/)

  // Open File opens the document beside the diff.
  await page.getByRole("button", { name: "Open File" }).click()
  await expect(page.getByRole("tab", { name: "README.md", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(tab).toBeVisible()

  // Diff tabs survive reloads.
  await tab.click()
  await page.reload()
  await expect(page.getByRole("tab", { name: "README.md (Working Tree)" })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("tab", { name: "README.md", exact: true })).toBeVisible()
  await expect(diffLines(page, "change-addition").filter({ hasText: "Four" })).toBeVisible()
})

test("diff options apply at once and persist", async ({ page }) => {
  await openDiff(page, "src/app.ts")
  const split = page.getByRole("button", { name: "Split" })
  const unified = page.getByRole("button", { name: "Unified" })
  const pre = page.locator(".diff-view pre[data-diff-type]")
  await expect(split).toHaveAttribute("aria-pressed", "true")
  await expect(pre).toHaveAttribute("data-diff-type", "split")
  await expect(page.locator(".status-diff-stats")).toHaveText("+2 −2")

  await unified.click()
  await expect(unified).toHaveAttribute("aria-pressed", "true")
  await expect(pre).toHaveAttribute("data-diff-type", "single")

  // Ignoring leading whitespace leaves only the value change.
  const whitespace = page.getByRole("button", { name: "Ignore whitespace" })
  await whitespace.click()
  await expect(whitespace).toHaveAttribute("aria-pressed", "true")
  await expect(page.locator(".status-diff-stats")).toHaveText("+1 −1")
  await expect(diffLines(page, "change-addition").filter({ hasText: "return" })).toHaveCount(0)

  const wrap = page.getByRole("button", { name: "Wrap lines" })
  await wrap.click()
  await expect(wrap).toHaveAttribute("aria-pressed", "true")
  await expect(page.locator(".diff-view [data-overflow]").first()).toHaveAttribute("data-overflow", "wrap")

  await page.getByRole("button", { name: "Inline changes: Word" }).click()
  await page.getByRole("menuitemradio", { name: "Character" }).click()
  await expect(page.getByRole("button", { name: "Inline changes: Character" })).toBeVisible()

  const expand = page.getByRole("button", { name: "Expand unchanged lines" })
  await expand.click()
  await expect(expand).toHaveAttribute("aria-pressed", "true")

  await page.reload()
  await expect(unified).toHaveAttribute("aria-pressed", "true")
  await expect(whitespace).toHaveAttribute("aria-pressed", "true")
  await expect(wrap).toHaveAttribute("aria-pressed", "true")
  await expect(expand).toHaveAttribute("aria-pressed", "true")
  await expect(page.getByRole("button", { name: "Inline changes: Character" })).toBeVisible()
  await expect(page.locator(".status-diff-stats")).toHaveText("+1 −1")
})

test("large diffs keep the page responsive and render only rows near the viewport", async ({ page }) => {
  test.setTimeout(60_000)
  await openDiff(page, "large.txt")
  // The worker's parse can take seconds on a busy machine.
  await expect(diffLines(page, "change-addition").filter({ hasText: "Line 0: the slow brown fox" })).toBeVisible({ timeout: 20_000 })
  await expect(page.locator(".status-diff-stats")).toHaveText("+1,200 −1,200")
  const rows = page.locator(".diff-view [data-line]")
  expect(await rows.count()).toBeLessThan(1_000)

  // Reparsing and re-rendering run off the main thread or only near the
  // viewport, so the next control answers at once.
  const whitespace = page.getByRole("button", { name: "Ignore whitespace" })
  const wrap = page.getByRole("button", { name: "Wrap lines" })
  const started = Date.now()
  await whitespace.click()
  await wrap.click()
  await expect(wrap).toHaveAttribute("aria-pressed", "true")
  expect(Date.now() - started).toBeLessThan(2_000)
  await expect(whitespace).toHaveAttribute("aria-pressed", "true")
  await expect(page.locator(".status-diff-stats")).toHaveText("+1,200 −1,200")

  // The document pane scrolls the diff; rows render where it lands.
  await page.locator("main").evaluate((pane) => { pane.scrollTop = pane.scrollHeight })
  await expect(diffLines(page, "change-addition").filter({ hasText: "Line 23980: the slow brown fox" })).toBeVisible({ timeout: 10_000 })
  expect(await rows.count()).toBeLessThan(1_000)

  // Row estimates follow the Font Size setting.
  await wrap.click()
  await page.getByRole("button", { name: "Settings", exact: true }).click()
  const settings = page.getByRole("dialog", { name: "Settings", exact: true })
  await settings.getByRole("spinbutton", { name: "Appearance: Font Size" }).fill("16")
  await settings.getByRole("button", { name: "Close settings" }).click()
  const row = diffLines(page, "change-addition").first()
  await expect.poll(async () => (await row.boundingBox())?.height).toBeCloseTo(22 * 16 / 13, 1)
  expect(await rows.count()).toBeLessThan(1_000)
})

test("diff line numbers select lines, update the URL, and copy path:line", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
  await openDiff(page, "README.md")
  // Split: HEAD's lines on the left (L), the working copy's on the right (R).
  const gutter = (side: "deletions" | "additions", line: number) =>
    page.locator(`.diff-view [data-${side}] [data-column-number="${line}"]`)
  const status = page.locator(".status-lines")
  await expect(gutter("additions", 4)).toBeVisible()
  await expect(status).toHaveCount(0)

  await gutter("additions", 4).click()
  await expect(page).toHaveURL(/#R4$/)
  await expect(status).toHaveText("Ln 4")
  await expect(page.locator(".status-diff-stats")).toHaveText("+2 −1")
  await page.getByRole("button", { name: "Copy README.md:4" }).click()
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("README.md:4")

  await gutter("additions", 6).click({ modifiers: ["Shift"] })
  await expect(page).toHaveURL(/#R4-R6$/)
  await expect(status).toHaveText("Ln 4–6")
  await page.getByRole("button", { name: "Copy README.md:4-6" }).click()
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("README.md:4-6")

  // HEAD's lines keep HEAD's numbers.
  await gutter("deletions", 4).click()
  await expect(page).toHaveURL(/#L4$/)
  await expect(status).toHaveText("Old Ln 4")
  await expect(page.getByRole("button", { name: "Copy README.md:4" })).toBeVisible()

  await page.keyboard.press("Escape")
  await expect(page).not.toHaveURL(/#/)
  await expect(status).toHaveCount(0)
  await expect(page.getByRole("button", { name: /^Copy README\.md:/ })).toHaveCount(0)

  // A shared range across sides selects the working copy's lines it covers.
  await page.goto(`${gitApiOrigin}/view?project=${project}&path=README.md&diff=1#L4-R6`)
  await expect(status).toHaveText("Ln 4–6")
  await expect(page.locator(".diff-view [data-line][data-selected-line]").first()).toBeVisible()
  await expect(page.getByRole("button", { name: "Copy README.md:4-6" })).toBeVisible()

  // Lines past the end select nothing.
  await page.goto(`${gitApiOrigin}/view?project=${project}&path=README.md&diff=1#R40`)
  await expect(gutter("additions", 4)).toBeVisible()
  await expect(status).toHaveCount(0)
})

test("deleted, renamed, binary, and unchanged files explain themselves", async ({ page }) => {
  await openDiff(page, "notes/removed.md")
  await expect(page.getByRole("tab", { name: "removed.md (Deleted)" })).toHaveAttribute("aria-selected", "true")
  await expect(diffLines(page, "change-deletion").filter({ hasText: "This note goes away." })).toBeVisible()
  await expect(page.getByRole("button", { name: "Open File" })).toHaveAttribute("aria-disabled", "true")

  await openDiff(page, "guides/start.md")
  await expect(page.getByRole("note").filter({ hasText: "Renamed from guides/intro.md" })).toBeVisible()
  await expect(page.getByRole("note").filter({ hasText: "File contents are unchanged." })).toBeVisible()

  await openDiff(page, "assets/logo.bin")
  await expect(page.getByRole("note")).toHaveText(/^Binary file changed · \d+ B → \d+ B$/)
  await expect(page.locator(".diff-view")).toHaveCount(0)

  await openDiff(page, "clean.md")
  await expect(page.getByRole("heading", { name: "No changes" })).toBeVisible()
  await page.getByRole("main").getByRole("button", { name: "Open File" }).click()
  await expect(page.getByRole("tab", { name: "clean.md", exact: true })).toHaveAttribute("aria-selected", "true")
})

test("diffs follow every theme", async ({ page }) => {
  await openDiff(page, "README.md")
  for (const [theme, scheme] of [
    ["light", "light"], ["dark", "dark"], ["catppuccin-mocha", "dark"], ["dracula", "dark"],
    ["gruvbox-dark", "dark"], ["nord", "dark"], ["solarized-light", "light"],
  ]) {
    await page.evaluate((value) => window.localStorage.setItem("servef-theme", value), theme)
    await page.reload()
    await expect(page.locator(".diff-view pre[data-diff-type]"), theme).toHaveCSS("color-scheme", scheme)
    const colors = await page.locator(".diff-view").evaluate((host) => {
      const added = host.shadowRoot?.querySelector('[data-line-type="change-addition"]')
      return { added: added ? getComputedStyle(added).backgroundColor : "", host: getComputedStyle(host).backgroundColor }
    })
    expect(colors.added, theme).not.toBe("")
    expect(colors.added, theme).not.toBe(colors.host)
  }
})

test("phones force Unified and open changes from the drawer", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.getByRole("button", { name: "Open files" }).click()
  await page.getByRole("button", { name: /^Source Control, / }).click()
  const drawer = page.getByRole("dialog", { name: "Source Control" })
  await drawer.getByRole("treeitem", { name: "app.ts", exact: true }).click()
  await expect(drawer).toBeHidden()
  await expect(page.getByRole("tab", { name: "app.ts (Working Tree)" })).toHaveAttribute("aria-selected", "true")
  await expect(page.locator(".diff-view pre[data-diff-type]")).toHaveAttribute("data-diff-type", "single")
  // Phones give the layout group's room to tabs; tablets explain it.
  await expect(page.getByRole("button", { name: "Split" })).toBeHidden()
  await page.setViewportSize({ width: 700, height: 844 })
  await expect(page.getByRole("button", { name: "Unified" })).toHaveAttribute("aria-pressed", "true")
  await expect(page.getByRole("button", { name: "Split" })).toHaveAttribute("aria-disabled", "true")
})

test("status failures keep the view usable and recover on Retry", async ({ page }) => {
  let fail = true
  await page.route("**/api/changes?*", async (route) => {
    if (!fail) return route.continue()
    await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "Couldn't read git status: index is corrupt" }) })
  })
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(sourceControl(page).getByRole("status")).toContainText("Couldn't read git status: index is corrupt.")
  await expect(page.locator(".activity-badge")).toHaveCount(0)
  fail = false
  await sourceControl(page).getByRole("button", { name: "Retry" }).click()
  await expect(changeRow(page, "README.md")).toBeVisible()
  await expect(page.locator(".activity-badge")).toHaveText(String(gitFixtureChanges))
})

test("folders outside git say so", async ({ page }) => {
  await page.goto(`/?project=${rootParam}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(sourceControl(page).getByRole("status")).toHaveText("This folder isn't a git repository.")
  await expect(page.getByRole("button", { name: "Source Control", exact: true })).toBeVisible()
})

function commits(page: Page) {
  return sourceControl(page).getByRole("tree", { name: "Commits" })
}

function commitRow(page: Page, subject: string) {
  return commits(page).getByRole("treeitem", { name: new RegExp(`^${subject}, servef, `) })
}

test("Commits lists the branch's commits and opens their files as diff tabs", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(sourceControl(page).getByText("1 ahead of main")).toBeVisible()
  await expect(sourceControl(page).getByRole("button", { name: "Branch", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(commits(page).getByRole("treeitem", { level: 1 })).toHaveCount(1)

  const commit = commitRow(page, featureSubject)
  await expect(commit).toHaveAttribute("aria-expanded", "false")
  await commit.click()
  await expect(commit).toHaveAttribute("aria-expanded", "true")
  const details = commits(page).getByRole("treeitem", { level: 2 }).filter({ hasText: "Explain how the fixture's branch history is built." })
  await expect(details).toContainText("servef")
  await expect(details.locator(".commit-hash")).toHaveText(/^[0-9a-f]{7}$/)
  await expect(commits(page).getByRole("treeitem", { name: "clean.md, Modified" })).toBeVisible()
  await commits(page).getByRole("treeitem", { name: "docs", exact: true }).click()
  await expect(commits(page).getByRole("treeitem", { name: "history.md, Added" })).toHaveCount(0)
  await commits(page).getByRole("treeitem", { name: "docs", exact: true }).click()

  await commits(page).getByRole("treeitem", { name: "history.md, Added" }).click()
  const tab = page.getByRole("tab", { name: /^history\.md \([0-9a-f]{7}\)$/ })
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/[?&]commit=[0-9a-f]{40}/)
  await expect(diffLines(page, "change-addition").filter({ hasText: "Written on the feature branch." })).toBeVisible()
  await expect(page.locator(".status-diff-stats")).toHaveText("+3 −0")
  await expect(commits(page).getByRole("treeitem", { name: "history.md, Added" })).toHaveAttribute("aria-selected", "true")

  // A commit diff compares the commit with its parent, not the working tree.
  await commits(page).getByRole("treeitem", { name: "clean.md, Modified" }).click()
  await expect(diffLines(page, "change-addition").filter({ hasText: "Edited on the feature branch." })).toBeVisible()
  await expect(page.getByRole("tab", { name: /^clean\.md \([0-9a-f]{7}\)$/ })).toHaveAttribute("aria-selected", "true")

  // Commit tabs survive reloads; Open File opens the working copy.
  await tab.click()
  await page.reload()
  await expect(page.getByRole("tab", { name: /^history\.md \([0-9a-f]{7}\)$/ })).toHaveAttribute("aria-selected", "true")
  await expect(diffLines(page, "change-addition").filter({ hasText: "Written on the feature branch." })).toBeVisible()
  await page.getByRole("button", { name: "Open File" }).click()
  await expect(page.getByRole("tab", { name: "history.md", exact: true })).toHaveAttribute("aria-selected", "true")
})

test("the commit range switches to all history and is remembered", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(commitRow(page, featureSubject)).toBeVisible()
  await sourceControl(page).getByRole("button", { name: "All", exact: true }).click()
  await expect(commitRow(page, "Initial")).toBeVisible()
  await expect(commits(page).getByRole("treeitem", { level: 1 })).toHaveCount(2)
  await expect(sourceControl(page).getByText("1 ahead of main")).toHaveCount(0)

  // Expanding the root commit lists every file it added.
  await commitRow(page, "Initial").click()
  await expect(commits(page).getByRole("treeitem", { name: "README.md, Added" })).toBeVisible()

  await page.reload()
  await expect(sourceControl(page).getByRole("button", { name: "All", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(commitRow(page, "Initial")).toBeVisible()
  await sourceControl(page).getByRole("button", { name: "Branch", exact: true }).click()
  await expect(commits(page).getByRole("treeitem", { level: 1 })).toHaveCount(1)
})

test("a branch with nothing ahead offers all history", async ({ page }) => {
  await page.route("**/api/commits?*", async (route) => {
    if (new URL(route.request().url()).searchParams.get("range") !== "branch") return route.continue()
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
      repository: true, branch: "main", base: "main", range: "branch", head: "a".repeat(40), ahead: 0, commits: [], more: false,
    }) })
  })
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(sourceControl(page).getByText("No commits ahead of main.")).toBeVisible()
  await sourceControl(page).getByRole("button", { name: "Show all history" }).click()
  await expect(sourceControl(page).getByRole("button", { name: "All", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(commitRow(page, "Initial")).toBeVisible()
})

test("history failures alert in Commits and recover on Retry", async ({ page }) => {
  let fail = true
  await page.route("**/api/commits?*", async (route) => {
    if (!fail) return route.continue()
    await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "Couldn't read git history: object missing" }) })
  })
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(sourceControl(page).getByRole("alert")).toHaveText("Couldn't read git history: object missing.")
  await expect(changeRow(page, "README.md")).toBeVisible()
  fail = false
  await sourceControl(page).getByRole("button", { name: "Retry" }).click()
  await expect(commitRow(page, featureSubject)).toBeVisible()
})

test("Commits works from the keyboard", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  const commit = commitRow(page, featureSubject)
  await commit.focus()
  await page.keyboard.press("ArrowRight")
  await expect(commit).toHaveAttribute("aria-expanded", "true")
  await page.keyboard.press("ArrowRight")
  await expect(commits(page).getByRole("treeitem", { level: 2 }).first()).toBeFocused()
  await page.keyboard.press("ArrowDown")
  await page.keyboard.press("ArrowDown")
  await expect(commits(page).getByRole("treeitem", { name: "history.md, Added" })).toBeFocused()
  await page.keyboard.press("ArrowLeft")
  await expect(commits(page).getByRole("treeitem", { name: "docs", exact: true })).toBeFocused()
  await page.keyboard.press("ArrowDown")
  await page.keyboard.press("Enter")
  await expect(page.getByRole("tab", { name: /^history\.md \([0-9a-f]{7}\)$/ })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("main")).toBeFocused()
})

test("Collapse All Commits closes every open commit", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  const collapse = sourceControl(page).getByRole("button", { name: "Collapse All Commits" })
  const commit = commitRow(page, featureSubject)
  await expect(commit).toBeVisible()
  await expect(collapse).toBeDisabled()
  await commit.click()
  await expect(commit).toHaveAttribute("aria-expanded", "true")
  await collapse.click()
  await expect(commit).toHaveAttribute("aria-expanded", "false")
  await expect(commits(page).getByRole("treeitem", { level: 2 })).toHaveCount(0)
  await expect(collapse).toBeDisabled()
})

test("sidebar and section headers center their contents", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(commitRow(page, featureSubject)).toBeVisible()
  const middle = (selector: string) => sourceControl(page).locator(selector).first().evaluate((element) => {
    const bounds = element.getBoundingClientRect()
    return bounds.top + bounds.height / 2
  })
  const header = await middle(".sidebar-header")
  expect(await middle(".sidebar-title")).toBeCloseTo(header, 0)
  expect(await middle(".sidebar-header button")).toBeCloseTo(header, 0)
  for (const name of ["Changes", "Commits"]) {
    const toggle = sourceControl(page).getByRole("button", { name, exact: true })
    const offsets = await toggle.evaluate((element) => {
      const section = element.closest(".source-control-section")
      const row = element.getBoundingClientRect()
      const top = section ? section.getBoundingClientRect().top + parseFloat(getComputedStyle(section).borderTopWidth) : row.top
      return row.top - top
    })
    expect(offsets).toBeCloseTo(0, 0)
  }
})

test("Changes and Commits collapse and resize, and the layout is remembered", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  const separator = sourceControl(page).getByRole("separator", { name: "Resize Changes and Commits" })
  await expect(separator).toHaveAttribute("aria-valuenow", "50")
  const changesBox = () => sourceControl(page).locator("#source-control-changes").boundingBox()
  const before = (await changesBox())?.height ?? 0
  await separator.focus()
  await page.keyboard.press("ArrowDown")
  await page.keyboard.press("ArrowDown")
  await expect(separator).toHaveAttribute("aria-valuenow", "60")
  await expect.poll(async () => (await changesBox())?.height ?? 0).toBeGreaterThan(before)

  const changes = sourceControl(page).getByRole("button", { name: "Changes", exact: true })
  await changes.click()
  await expect(changes).toHaveAttribute("aria-expanded", "false")
  await expect(changeRow(page, "README.md")).toBeHidden()
  await expect(separator).toHaveCount(0)

  await page.reload()
  await expect(sourceControl(page).getByRole("button", { name: "Changes", exact: true })).toHaveAttribute("aria-expanded", "false")
  await sourceControl(page).getByRole("button", { name: "Changes", exact: true }).click()
  await expect(changeRow(page, "README.md")).toBeVisible()
  await expect(separator).toHaveAttribute("aria-valuenow", "60")
  await separator.dblclick()
  await expect(separator).toHaveAttribute("aria-valuenow", "50")
})

test("phones stack both sections and open commit files from the drawer", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.getByRole("button", { name: "Open files" }).click()
  await page.getByRole("button", { name: /^Source Control, / }).click()
  const drawer = page.getByRole("dialog", { name: "Source Control" })
  await expect(drawer.getByRole("separator", { name: "Resize Changes and Commits" })).toHaveCount(0)
  await expect(drawer.getByRole("treeitem", { name: "app.ts", exact: true })).toBeVisible()
  await drawer.getByRole("treeitem", { name: new RegExp(`^${featureSubject}, `) }).click()
  await drawer.getByRole("treeitem", { name: "clean.md, Modified" }).click()
  await expect(drawer).toBeHidden()
  await expect(page.getByRole("tab", { name: /^clean\.md \([0-9a-f]{7}\)$/ })).toHaveAttribute("aria-selected", "true")
})

test("closing the last diff tab ends every diff worker", async ({ page }) => {
  await openDiff(page, "src/app.ts")
  await expect(diffLines(page, "change-addition").first()).toBeVisible()
  await expect.poll(() => page.workers().length).toBeGreaterThan(0)
  const tab = page.getByRole("tab", { name: "app.ts (Working Tree)" })
  await tab.click({ button: "right" })
  await page.getByRole("menuitem", { name: "Close", exact: true }).click()
  await expect(tab).toHaveCount(0)
  await expect.poll(() => page.workers().length).toBe(0)
})
