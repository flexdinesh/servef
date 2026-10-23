import { existsSync, readFileSync, writeFileSync } from "node:fs"
import path from "node:path"

import { expect, test, type Page } from "@playwright/test"

import { rootParam } from "./fixture-root.ts"
import { featureSubject, gitFixtureChanges, gitFixtureRoot } from "./git-fixture.ts"
import { gitApiOrigin } from "./ports.ts"

// The fixture repository is built by the git server's webServer command.
test.skip(() => !existsSync(path.join(gitFixtureRoot, ".git")), "git is not installed")

const project = encodeURIComponent(gitFixtureRoot)

// Uncommitted changes in review order: folders first, then root files.
const uncommittedFiles = [
  "assets/logo.bin", "guides/start.md", "notes/removed.md", "src/app.ts", "added.md", "large.txt", "README.md", "untracked.txt",
]

function sourceControl(page: Page) {
  return page.getByRole("region", { name: "Source Control" })
}

function compareList(page: Page) {
  return sourceControl(page).getByRole("listbox", { name: "Compare" })
}

function uncommittedRow(page: Page) {
  return compareList(page).getByRole("option", { name: /^Uncommitted changes/ })
}

function branchRow(page: Page) {
  return compareList(page).getByRole("option", { name: /^Branch vs / })
}

function commitRow(page: Page, subject: string) {
  return compareList(page).getByRole("option", { name: new RegExp(`^${subject}, servef, `) })
}

function filesPane(page: Page) {
  return sourceControl(page).getByRole("region", { name: /^Files: / })
}

function fileRow(page: Page, name: string) {
  return filesPane(page).getByRole("treeitem", { name, exact: true })
}

function reviewFile(page: Page, file: string) {
  return page.getByRole("main").getByRole("region", { name: file, exact: true })
}

// fileToggle opens or closes a review section's diff.
function fileToggle(page: Page, file: string) {
  return reviewFile(page, file).getByRole("button", { name: new RegExp(`^${file.replaceAll(".", "\\.")}`) })
}

function diffLines(page: Page, type: "change-addition" | "change-deletion", file?: string) {
  const scope = file ? reviewFile(page, file) : page.getByRole("main")
  return scope.locator(`.diff-view [data-line-type="${type}"]`)
}

async function openReview(page: Page, search: string) {
  await page.goto(`${gitApiOrigin}/view?project=${project}&${search}`)
}

async function openSourceControl(page: Page) {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
}

// topOffset is how far a review section sits below the pane's top edge.
async function topOffset(page: Page, file: string): Promise<number> {
  const [section, pane] = await Promise.all([reviewFile(page, file).boundingBox(), page.getByRole("main").boundingBox()])
  return section && pane ? section.y - pane.y : Number.NaN
}

test("Source Control lists every pending change and reviews them in one tab", async ({ page }) => {
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
  // A dirty working tree selects its uncommitted changes first.
  await expect(uncommittedRow(page)).toHaveAccessibleName(`Uncommitted changes, ${gitFixtureChanges} files`)
  await expect(uncommittedRow(page)).toHaveAttribute("aria-selected", "true")
  await expect(filesPane(page)).toHaveAccessibleName("Files: Uncommitted")
  await expect(page.getByLabel("Files in Uncommitted")).toBeVisible()
  for (const name of ["README.md", "added.md", "logo.bin", "start.md", "removed.md", "app.ts", "untracked.txt", "large.txt"]) {
    await expect(fileRow(page, name)).toBeVisible()
  }
  await expect(fileRow(page, "debug.log")).toHaveCount(0)
  await expect(fileRow(page, "clean.md")).toHaveCount(0)
  await expect(filesPane(page).locator('[data-item-path="guides/start.md"]')).toHaveAttribute("data-item-git-status", "renamed")
  // Rows carry line stats.
  await expect(fileRow(page, "README.md")).toContainText("+2 −1")

  await fileRow(page, "README.md").click()
  const tab = page.getByRole("tab", { name: "Uncommitted", exact: true })
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/[?&]review=working#f=README\.md$/)
  await expect(diffLines(page, "change-addition", "README.md").filter({ hasText: "Four" })).toBeVisible()
  await expect(diffLines(page, "change-deletion", "README.md").filter({ hasText: "Two" })).toBeVisible()
  await expect(page).toHaveTitle(/Uncommitted changes/)
  // One tab reviews every change.
  await expect(page.getByRole("main").getByRole("heading", { level: 1, name: "Uncommitted changes" })).toBeVisible()
  await expect(page.getByRole("main").getByRole("region")).toHaveCount(gitFixtureChanges)
  await expect(page.getByRole("main").getByText(`0 of ${gitFixtureChanges} viewed`)).toBeVisible()

  // Open File opens the document beside the review.
  await reviewFile(page, "README.md").getByRole("button", { name: "Open File" }).click()
  await expect(page.getByRole("tab", { name: "README.md", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(tab).toBeVisible()

  // Review tabs survive reloads.
  await tab.click()
  await page.reload()
  await expect(page.getByRole("tab", { name: "Uncommitted", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("tab", { name: "README.md", exact: true })).toBeVisible()
  await expect(diffLines(page, "change-addition", "README.md").filter({ hasText: "Four" })).toBeVisible()
})

test("diff options apply at once and persist", async ({ page }) => {
  await openReview(page, "review=working#f=src/app.ts")
  const split = page.getByRole("button", { name: "Split" })
  const unified = page.getByRole("button", { name: "Unified" })
  const pre = reviewFile(page, "src/app.ts").locator(".diff-view pre[data-diff-type]")
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
  await expect(diffLines(page, "change-addition", "src/app.ts").filter({ hasText: "answer = 2" })).toBeVisible()
  await expect(diffLines(page, "change-addition", "src/app.ts").filter({ hasText: "return" })).toHaveCount(0)

  const wrap = page.getByRole("button", { name: "Wrap lines" })
  await wrap.click()
  await expect(wrap).toHaveAttribute("aria-pressed", "true")
  await expect(reviewFile(page, "src/app.ts").locator(".diff-view [data-overflow]").first()).toHaveAttribute("data-overflow", "wrap")

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
  await expect(diffLines(page, "change-addition", "src/app.ts").filter({ hasText: "return" })).toHaveCount(0)
})

test("large diffs keep the page responsive and render only rows near the viewport", async ({ page }) => {
  test.setTimeout(60_000)
  await openReview(page, "review=working#f=large.txt")
  // The worker's parse can take seconds on a busy machine.
  await expect(diffLines(page, "change-addition", "large.txt").filter({ hasText: "Line 0: the slow brown fox" })).toBeVisible({ timeout: 20_000 })
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

  // The document pane scrolls the review; rows render where it lands. A
  // wheel turn lets go of the linked file first.
  await page.locator("main").evaluate((pane) => {
    pane.dispatchEvent(new WheelEvent("wheel"))
    pane.scrollTop = pane.scrollHeight
  })
  await expect(diffLines(page, "change-addition", "large.txt").filter({ hasText: "Line 23980: the slow brown fox" })).toBeVisible({ timeout: 10_000 })
  expect(await rows.count()).toBeLessThan(1_000)

  // Row estimates follow the Font Size setting.
  await wrap.click()
  await page.getByRole("button", { name: "Settings", exact: true }).click()
  const settings = page.getByRole("dialog", { name: "Settings", exact: true })
  await settings.getByRole("spinbutton", { name: "Appearance: Font Size" }).fill("16")
  await settings.getByRole("button", { name: "Close settings" }).click()
  const row = diffLines(page, "change-addition", "large.txt").last()
  await expect.poll(async () => (await row.boundingBox())?.height).toBeCloseTo(22 * 16 / 13, 1)
  expect(await rows.count()).toBeLessThan(1_000)
})

test("diff line numbers select lines, update the URL, and copy path:line", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
  await openReview(page, "review=working#f=README.md")
  // Split: HEAD's lines on the left (L), the working copy's on the right (R).
  const gutter = (side: "deletions" | "additions", line: number) =>
    reviewFile(page, "README.md").locator(`.diff-view [data-${side}] [data-column-number="${line}"]`)
  const status = page.locator(".status-lines")
  // The review keeps README.md in view while large.txt above it loads; a
  // click lets go of it.
  await expect(reviewFile(page, "large.txt").locator(".diff-view")).toBeAttached({ timeout: 20_000 })
  await expect(gutter("additions", 4)).toBeInViewport()
  await expect(status).toHaveCount(0)

  await gutter("additions", 4).click()
  await expect(page).toHaveURL(/#f=README\.md&R4$/)
  await expect(status).toHaveText("Ln 4")
  await expect(page.locator(".status-diff-stats")).toHaveText("+2 −1")
  await page.getByRole("button", { name: "Copy README.md:4" }).click()
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("README.md:4")

  await gutter("additions", 6).click({ modifiers: ["Shift"] })
  await expect(page).toHaveURL(/#f=README\.md&R4-R6$/)
  await expect(status).toHaveText("Ln 4–6")
  await page.getByRole("button", { name: "Copy README.md:4-6" }).click()
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("README.md:4-6")

  // HEAD's lines keep HEAD's numbers.
  await gutter("deletions", 4).click()
  await expect(page).toHaveURL(/#f=README\.md&L4$/)
  await expect(status).toHaveText("Old Ln 4")
  await expect(page.getByRole("button", { name: "Copy README.md:4" })).toBeVisible()

  // Escape clears the lines and keeps the file.
  await page.keyboard.press("Escape")
  await expect(page).toHaveURL(/#f=README\.md$/)
  await expect(status).toHaveCount(0)
  await expect(page.getByRole("button", { name: /^Copy README\.md:/ })).toHaveCount(0)

  // A shared range across sides selects the working copy's lines it covers.
  await openReview(page, "review=working#f=README.md&L4-R6")
  await expect(status).toHaveText("Ln 4–6")
  await expect(reviewFile(page, "README.md").locator(".diff-view [data-line][data-selected-line]").first()).toBeVisible()
  await expect(page.getByRole("button", { name: "Copy README.md:4-6" })).toBeVisible()

  // Lines past the end select nothing.
  await openReview(page, "review=working#f=README.md&R40")
  await expect(gutter("additions", 4)).toBeVisible()
  await expect(status).toHaveCount(0)
})

test("a deep link scrolls its review to the file and selects its lines", async ({ page }) => {
  await openReview(page, "review=working#f=src/app.ts&R2-R4")
  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(reviewFile(page, "src/app.ts").locator(".diff-view [data-line][data-selected-line]").first()).toBeVisible()
  await expect(page.locator(".status-lines")).toHaveText("Ln 2–4")
  await expect(page.locator(".status-path")).toHaveText("src/app.ts")
  await expect(page.getByRole("button", { name: "Copy src/app.ts:2-4" })).toBeVisible()
  // Its lines are revealed; the file in view is selected in Files.
  await expect(reviewFile(page, "src/app.ts").locator(".diff-view [data-additions] [data-column-number=\"2\"]")).toBeInViewport()
  await expect(uncommittedRow(page)).toHaveAttribute("aria-selected", "true")
  await expect(fileRow(page, "app.ts")).toHaveAttribute("aria-selected", "true")
})

test("review anchors scroll their lines into view and open the unchanged lines they name", async ({ page }) => {
  test.setTimeout(60_000)
  const gutter = (line: number) => reviewFile(page, "large.txt").locator(`.diff-view [data-additions] [data-column-number="${line}"]`)
  const status = page.locator(".status-lines")
  // large.txt changes every twentieth line, so line 12001 is a change.
  await openReview(page, "review=working#f=large.txt&R12001")
  await expect(gutter(12001)).toBeInViewport({ timeout: 20_000 })
  await expect(status).toHaveText("Ln 12001")

  // Line 12010 sits in the collapsed gap between two changes.
  await openReview(page, "review=working#f=large.txt&R12010-R12011")
  await expect(gutter(12010)).toBeInViewport({ timeout: 20_000 })
  await expect(reviewFile(page, "large.txt").locator(".diff-view [data-line][data-selected-line]").first()).toBeVisible()
  await expect(status).toHaveText("Ln 12010–12011")

  // Selecting from the gutter leaves the pane where it is.
  const pane = page.locator("main")
  const before = await pane.evaluate((element) => element.scrollTop)
  await gutter(12012).click()
  await expect(page).toHaveURL(/#f=large\.txt&R12012$/)
  await expect(status).toHaveText("Ln 12012")
  expect(await pane.evaluate((element) => element.scrollTop)).toBe(before)
})

test("F7 and the toolbar step through changes across the review's files", async ({ page }) => {
  test.setTimeout(60_000)
  const gutter = (line: number) => reviewFile(page, "large.txt").locator(`.diff-view [data-additions] [data-column-number="${line}"]`)
  await openReview(page, "review=working#f=large.txt")
  await expect(gutter(1)).toBeVisible({ timeout: 20_000 })
  const pane = page.locator("main")
  await pane.focus()

  // Each hunk is its change with four lines of context either side.
  await page.keyboard.press("F7")
  await expect(page).toHaveURL(/#f=large\.txt&R1-R5$/)
  await page.keyboard.press("F7")
  await expect(page).toHaveURL(/#f=large\.txt&R17-R25$/)
  await expect(page.getByRole("button", { name: "Copy large.txt:17-25" })).toBeVisible()
  await page.keyboard.press("Shift+F7")
  await expect(page).toHaveURL(/#f=large\.txt&R1-R5$/)

  // Past a file's first change, steps cross into the file before it.
  await page.keyboard.press("Shift+F7")
  await expect(page).toHaveURL(/#f=added\.md&R1-R3$/)
  await expect(reviewFile(page, "added.md").locator(".diff-view [data-line][data-selected-line]").first()).toBeInViewport()
  await page.getByRole("button", { name: "Previous change" }).click()
  await expect(page).toHaveURL(/#f=src\/app\.ts&R1-R5$/)
  await page.getByRole("button", { name: "Next change" }).click()
  await expect(page).toHaveURL(/#f=added\.md&R1-R3$/)
  await page.getByRole("button", { name: "Next change" }).click()
  await expect(page).toHaveURL(/#f=large\.txt&R1-R5$/)
  await expect(gutter(1)).toBeInViewport()

  // Files without hunks are skipped, and steps wrap past the review's ends.
  await openReview(page, "review=working#f=notes/removed.md")
  await expect(diffLines(page, "change-deletion", "notes/removed.md").first()).toBeVisible()
  await pane.focus()
  await page.keyboard.press("F7")
  await expect(page).toHaveURL(/#f=notes\/removed\.md&L1-L3$/)
  await page.keyboard.press("Shift+F7")
  await expect(page).toHaveURL(/#f=untracked\.txt&R1$/)
  await expect(reviewFile(page, "untracked.txt").locator(".diff-view [data-line][data-selected-line]").first()).toBeInViewport()
})

test("legacy per-file diff links open the matching review at that file", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/view?project=${project}&path=README.md&diff=1#R4`)
  await expect(page).toHaveURL(/[?&]review=working#f=README\.md&R4$/)
  await expect(page).not.toHaveURL(/[?&](path|diff)=/)
  await expect(page.getByRole("tab", { name: "Uncommitted", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(page.locator(".status-lines")).toHaveText("Ln 4")

  await page.goto(`${gitApiOrigin}/view?project=${project}&path=${encodeURIComponent("docs/history.md")}&branch=1`)
  await expect(page).toHaveURL(/[?&]review=branch#f=docs\/history\.md$/)
  await expect(page.getByRole("tab", { name: "vs main", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(diffLines(page, "change-addition", "docs/history.md").filter({ hasText: "Written on the feature branch." })).toBeVisible()
})

test("deleted, renamed, and binary files explain themselves", async ({ page }) => {
  await openReview(page, "review=working#f=notes/removed.md")
  await expect(diffLines(page, "change-deletion", "notes/removed.md").filter({ hasText: "This note goes away." })).toBeVisible()
  await expect(reviewFile(page, "notes/removed.md").getByRole("button", { name: "Open File" })).toHaveAttribute("aria-disabled", "true")

  const renamed = reviewFile(page, "guides/start.md")
  await expect(renamed.getByRole("button", { name: "guides/start.md from guides/intro.md" })).toBeVisible()
  await expect(renamed.getByRole("note").filter({ hasText: "File contents are unchanged." })).toBeVisible()

  const binary = reviewFile(page, "assets/logo.bin")
  await expect(binary.getByRole("note")).toHaveText(/^Binary file changed · \d+ B → \d+ B$/)
  await expect(binary.locator(".diff-view")).toHaveCount(0)
})

test("diffs follow every theme", async ({ page }) => {
  await openReview(page, "review=working#f=README.md")
  const view = reviewFile(page, "README.md").locator(".diff-view")
  for (const [theme, scheme] of [
    ["light", "light"], ["dark", "dark"], ["catppuccin-mocha", "dark"], ["dracula", "dark"],
    ["gruvbox-dark", "dark"], ["nord", "dark"], ["solarized-light", "light"],
  ]) {
    await page.evaluate((value) => window.localStorage.setItem("servef-theme", value), theme)
    await page.reload()
    await expect(view.locator("pre[data-diff-type]"), theme).toHaveCSS("color-scheme", scheme)
    const colors = await view.evaluate((host) => {
      const added = host.shadowRoot?.querySelector('[data-line-type="change-addition"]')
      return { added: added ? getComputedStyle(added).backgroundColor : "", host: getComputedStyle(host).backgroundColor }
    })
    expect(colors.added, theme).not.toBe("")
    expect(colors.added, theme).not.toBe(colors.host)
  }
})

test("phones drill from the list into files and force Unified", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.getByRole("button", { name: "Open files" }).click()
  await page.getByRole("button", { name: /^Source Control, / }).click()
  const drawer = page.getByRole("dialog", { name: "Source Control" })
  // Phones show the list or the files, never both, and select no ranges.
  await expect(drawer.getByRole("separator", { name: "Resize commits and files" })).toHaveCount(0)
  await expect(drawer.getByRole("listbox", { name: "Compare" })).not.toHaveAttribute("aria-multiselectable", "true")
  await expect(drawer.getByRole("region", { name: /^Files: / })).toHaveCount(0)

  await drawer.getByRole("option", { name: new RegExp(`^${featureSubject}, `) }).click()
  const files = drawer.getByRole("region", { name: `Files: ${featureSubject}` })
  await expect(files.getByRole("treeitem", { name: "clean.md", exact: true })).toBeVisible()
  await expect(drawer.getByRole("listbox", { name: "Compare" })).toHaveCount(0)
  await files.getByRole("button", { name: "Back to list" }).click()
  await expect(drawer.getByRole("listbox", { name: "Compare" })).toBeVisible()

  await drawer.getByRole("option", { name: /^Uncommitted changes/ }).click()
  await drawer.getByRole("region", { name: "Files: Uncommitted" }).getByRole("treeitem", { name: "app.ts", exact: true }).click()
  await expect(drawer).toBeHidden()
  await expect(page.getByRole("tab", { name: "Uncommitted", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/#f=src\/app\.ts$/)
  await expect(reviewFile(page, "src/app.ts").locator(".diff-view pre[data-diff-type]")).toHaveAttribute("data-diff-type", "single")
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
  await openSourceControl(page)
  await expect(filesPane(page).getByRole("status")).toContainText("Couldn't read git status: index is corrupt.")
  await expect(page.locator(".activity-badge")).toHaveCount(0)
  // History still lists.
  await expect(commitRow(page, featureSubject)).toBeVisible()
  fail = false
  await filesPane(page).getByRole("button", { name: "Retry" }).click()
  await expect(fileRow(page, "README.md")).toBeVisible()
  await expect(page.locator(".activity-badge")).toHaveText(String(gitFixtureChanges))
})

test("folders outside git say so", async ({ page }) => {
  await page.goto(`/?project=${rootParam}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(sourceControl(page).getByRole("status")).toHaveText("This folder isn't a git repository.")
  await expect(page.getByRole("button", { name: "Source Control", exact: true })).toBeVisible()
})

test("selecting a commit shows its files, and a file opens the commit's review there", async ({ page }) => {
  await openSourceControl(page)
  await expect(sourceControl(page).getByText("1 ahead of main")).toBeVisible()
  await expect(sourceControl(page).getByRole("button", { name: "Branch", exact: true })).toHaveAttribute("aria-pressed", "true")
  // Uncommitted and the branch lead the one commit.
  await expect(compareList(page).getByRole("option")).toHaveCount(3)

  const commit = commitRow(page, featureSubject)
  await commit.click()
  await expect(commit).toHaveAttribute("aria-selected", "true")
  await expect(uncommittedRow(page)).toHaveAttribute("aria-selected", "false")
  await expect(filesPane(page)).toHaveAccessibleName(`Files: ${featureSubject}`)
  await expect(filesPane(page)).toContainText("Explain how the fixture's branch history is built.")
  await expect(filesPane(page)).toContainText("servef")
  await expect(filesPane(page).getByRole("button", { name: /^Copy commit hash [0-9a-f]{40}$/ })).toHaveText(/^[0-9a-f]{7}$/)
  await expect(fileRow(page, "clean.md")).toContainText("+2 −0")
  await expect(fileRow(page, "history.md")).toContainText("+3 −0")
  // Folders collapse one at a time too.
  await fileRow(page, "docs").click()
  await expect(fileRow(page, "history.md")).toHaveCount(0)
  await fileRow(page, "docs").click()

  await fileRow(page, "history.md").click()
  const tab = page.getByRole("tab", { name: new RegExp(`^[0-9a-f]{7} ${featureSubject}$`) })
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/[?&]commit=[0-9a-f]{40}#f=docs\/history\.md$/)
  await expect(page.getByRole("main").getByRole("heading", { level: 1, name: featureSubject })).toBeVisible()
  await expect(diffLines(page, "change-addition", "docs/history.md").filter({ hasText: "Written on the feature branch." })).toBeVisible()
  await expect(page.locator(".status-diff-stats")).toHaveText("+3 −0")
  await expect(fileRow(page, "history.md")).toHaveAttribute("aria-selected", "true")

  // A commit compares with its parent, not the working tree.
  await expect(diffLines(page, "change-addition", "clean.md").filter({ hasText: "Edited on the feature branch." })).toBeVisible()

  // Commit tabs survive reloads; Open File opens the working copy.
  await page.reload()
  await expect(page.getByRole("tab", { name: new RegExp(`^[0-9a-f]{7} ${featureSubject}$`) })).toHaveAttribute("aria-selected", "true")
  await expect(diffLines(page, "change-addition", "docs/history.md").filter({ hasText: "Written on the feature branch." })).toBeVisible()
  await reviewFile(page, "docs/history.md").getByRole("button", { name: "Open File" }).click()
  await expect(page.getByRole("tab", { name: "history.md", exact: true })).toHaveAttribute("aria-selected", "true")
})

test("clicking a file in Files scrolls the open review to it", async ({ page }) => {
  await openSourceControl(page)
  await uncommittedRow(page).dblclick()
  await expect(page.getByRole("tab", { name: "Uncommitted", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/[?&]review=working$/)
  await expect(reviewFile(page, uncommittedFiles[0] ?? "")).toBeVisible()
  expect(await topOffset(page, "src/app.ts")).toBeGreaterThan(2)

  await fileRow(page, "app.ts").click()
  await expect(page).toHaveURL(/#f=src\/app\.ts$/)
  await expect.poll(() => topOffset(page, "src/app.ts")).toBeLessThan(2)
  await expect(page.locator(".status-path")).toHaveText("src/app.ts")
  await expect(fileRow(page, "app.ts")).toHaveAttribute("aria-selected", "true")
  // The same tab stays open; no per-file tabs.
  await expect(page.getByRole("tab")).toHaveCount(1)
})

test("a file too near the end to reach the top stays the file in view", async ({ page }) => {
  await openSourceControl(page)
  await uncommittedRow(page).dblclick()
  await expect(reviewFile(page, "README.md")).toBeVisible()
  // README.md is next to last, so the pane scrolls to its end to show it.
  await fileRow(page, "README.md").click()
  await expect(page).toHaveURL(/#f=README\.md$/)
  await expect(diffLines(page, "change-addition", "README.md").filter({ hasText: "Four" })).toBeInViewport()
  await expect(page.locator(".status-path")).toHaveText("README.md")
  await expect(page.locator(".status-diff-stats")).toHaveText("+2 −1")
  await expect(fileRow(page, "README.md")).toHaveAttribute("aria-selected", "true")
})

test("the commit range switches to all history and is remembered", async ({ page }) => {
  await openSourceControl(page)
  await expect(commitRow(page, featureSubject)).toBeVisible()
  await sourceControl(page).getByRole("button", { name: "All", exact: true }).click()
  await expect(commitRow(page, "Initial")).toBeVisible()
  await expect(compareList(page).getByRole("option")).toHaveCount(4)
  await expect(sourceControl(page).getByText("1 ahead of main")).toHaveCount(0)

  // The root commit adds every file.
  await commitRow(page, "Initial").click()
  await expect(filesPane(page)).toHaveAccessibleName("Files: Initial")
  await expect(fileRow(page, "README.md")).toBeVisible()
  await expect(filesPane(page).locator('[data-item-path="README.md"]')).toHaveAttribute("data-item-git-status", "added")

  await page.reload()
  await expect(sourceControl(page).getByRole("button", { name: "All", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(commitRow(page, "Initial")).toBeVisible()
  await sourceControl(page).getByRole("button", { name: "Branch", exact: true }).click()
  await expect(compareList(page).getByRole("option")).toHaveCount(3)
})

test("the branch row compares with its base and opens a live review", async ({ page }) => {
  await openSourceControl(page)
  const row = branchRow(page)
  await expect(row).toHaveAccessibleName("Branch vs main, 2 files")
  await row.click()
  await expect(row).toHaveAttribute("aria-selected", "true")
  await expect(filesPane(page)).toHaveAccessibleName("Files: vs main")
  await expect(filesPane(page).getByText(/^1 commit ahead$/)).toBeVisible()
  await expect(filesPane(page).getByText("merge-base")).toBeVisible()
  await expect(fileRow(page, "clean.md")).toBeVisible()
  await fileRow(page, "history.md").click()

  const tab = page.getByRole("tab", { name: "vs main", exact: true })
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/[?&]review=branch#f=docs\/history\.md$/)
  await expect(diffLines(page, "change-addition", "docs/history.md").filter({ hasText: "Written on the feature branch." })).toBeVisible()
  await expect(fileRow(page, "history.md")).toHaveAttribute("aria-selected", "true")

  // The branch follows HEAD: returning to the window rereads it.
  let reads = 0
  await page.route("**/api/branch-files?*", async (route) => {
    reads++
    await route.continue()
  })
  await page.waitForTimeout(2_100)
  await page.evaluate(() => window.dispatchEvent(new Event("focus")))
  await expect.poll(() => reads).toBeGreaterThan(0)

  await page.reload()
  await expect(page.getByRole("tab", { name: "vs main", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(diffLines(page, "change-addition", "docs/history.md").filter({ hasText: "Written on the feature branch." })).toBeVisible()
})

test("the base picker changes the branch compare and is remembered", async ({ page }) => {
  await openSourceControl(page)
  await expect(branchRow(page)).toHaveAccessibleName("Branch vs main, 2 files")
  await branchRow(page).getByRole("button", { name: "Change base branch" }).click()
  const filter = page.getByRole("combobox", { name: "Filter branches" })
  await expect(filter).toBeFocused()
  const branches = page.getByRole("listbox", { name: "Branches" })
  await expect(branches.getByRole("option", { name: /^Default/ })).toHaveAttribute("aria-selected", "true")
  await filter.fill("rel")
  await expect(branches.getByRole("option")).toHaveCount(1)
  await branches.getByRole("option", { name: "release" }).click()
  await expect(branches).toBeHidden()

  await expect(branchRow(page)).toHaveAccessibleName("Branch vs release, 2 files")
  await expect(branchRow(page)).toHaveAttribute("aria-selected", "true")
  await expect(filesPane(page)).toHaveAccessibleName("Files: vs release")
  await expect(fileRow(page, "history.md")).toBeVisible()
  await filesPane(page).getByRole("button", { name: "Open Changes" }).click()
  await expect(page).toHaveURL(/[?&]review=branch&base=release$/)
  await expect(page.getByRole("tab", { name: "vs release", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("main").getByText("1 commit ahead of release")).toBeVisible()

  // The choice is remembered for the project.
  await page.reload()
  await expect(branchRow(page)).toHaveAccessibleName("Branch vs release, 2 files")
  await branchRow(page).getByRole("button", { name: "Change base branch" }).click()
  await expect(page.getByRole("listbox", { name: "Branches" }).getByRole("option", { name: "release" })).toHaveAttribute("aria-selected", "true")
})

test("Shift-selecting commits compares their range until cleared", async ({ page }) => {
  await openSourceControl(page)
  await sourceControl(page).getByRole("button", { name: "All", exact: true }).click()
  const feature = commitRow(page, featureSubject)
  const initial = commitRow(page, "Initial")
  await expect(initial).toBeVisible()
  await expect(compareList(page)).toHaveAttribute("aria-multiselectable", "true")

  await feature.click()
  await initial.click({ modifiers: ["Shift"] })
  await expect(feature).toHaveAttribute("aria-selected", "true")
  await expect(initial).toHaveAttribute("aria-selected", "true")
  await expect(filesPane(page)).toHaveAccessibleName("Files: 2 commits")
  await expect(filesPane(page)).toContainText(/[0-9a-f]{7}\.\.[0-9a-f]{7}/)

  // The oldest selected commit is the root, so every file is added.
  await expect(fileRow(page, "history.md")).toBeVisible()
  await expect(filesPane(page).locator('[data-item-path="clean.md"]')).toHaveAttribute("data-item-git-status", "added")
  await fileRow(page, "clean.md").click()
  const tab = page.getByRole("tab", { name: /^[0-9a-f]{7}\.\.[0-9a-f]{7}$/ })
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/[?&]from=[0-9a-f]{40}&to=[0-9a-f]{40}#f=clean\.md$/)
  await expect(diffLines(page, "change-addition", "clean.md").filter({ hasText: "Edited on the feature branch." })).toBeVisible()

  // Clear range narrows to the newest commit; the tab stays open.
  await filesPane(page).getByRole("button", { name: "Clear range" }).click()
  await expect(feature).toHaveAttribute("aria-selected", "true")
  await expect(initial).toHaveAttribute("aria-selected", "false")
  await expect(filesPane(page)).toHaveAccessibleName(`Files: ${featureSubject}`)
  await expect(filesPane(page).getByRole("button", { name: "Clear range" })).toHaveCount(0)
  await expect(tab).toBeVisible()

  // Shift+Arrow extends from the focused commit; Escape narrows it again.
  await feature.focus()
  await page.keyboard.press("Shift+ArrowDown")
  await expect(initial).toBeFocused()
  await expect(initial).toHaveAttribute("aria-selected", "true")
  await expect(filesPane(page)).toHaveAccessibleName("Files: 2 commits")
  await page.keyboard.press("Escape")
  await expect(initial).toHaveAttribute("aria-selected", "false")
  await expect(feature).toHaveAttribute("aria-selected", "true")

  // Switching the list to the branch drops a range for the branch compare.
  await initial.click({ modifiers: ["Shift"] })
  await expect(filesPane(page)).toHaveAccessibleName("Files: 2 commits")
  await sourceControl(page).getByRole("button", { name: "Branch", exact: true }).click()
  await expect(branchRow(page)).toHaveAttribute("aria-selected", "true")
  await expect(filesPane(page)).toHaveAccessibleName("Files: vs main")

  await page.reload()
  await expect(page.getByRole("tab", { name: /^[0-9a-f]{7}\.\.[0-9a-f]{7}$/ })).toHaveAttribute("aria-selected", "true")
  await expect(diffLines(page, "change-addition", "clean.md").filter({ hasText: "Edited on the feature branch." })).toBeVisible()
})

test("a branch with nothing ahead offers all history", async ({ page }) => {
  await page.route("**/api/commits?*", async (route) => {
    if (new URL(route.request().url()).searchParams.get("range") !== "branch") return route.continue()
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
      repository: true, branch: "main", base: "main", range: "branch", head: "a".repeat(40), ahead: 0, commits: [], more: false,
    }) })
  })
  await openSourceControl(page)
  await expect(sourceControl(page).getByText("No commits ahead of main.")).toBeVisible()
  await sourceControl(page).getByRole("button", { name: "Show all history" }).click()
  await expect(sourceControl(page).getByRole("button", { name: "All", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(commitRow(page, "Initial")).toBeVisible()
})

test("history failures alert in the list and recover on Retry", async ({ page }) => {
  let fail = true
  await page.route("**/api/commits?*", async (route) => {
    if (!fail) return route.continue()
    await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "Couldn't read git history: object missing" }) })
  })
  await openSourceControl(page)
  await expect(sourceControl(page).getByRole("alert")).toHaveText("Couldn't read git history: object missing.")
  await expect(fileRow(page, "README.md")).toBeVisible()
  fail = false
  await sourceControl(page).getByRole("button", { name: "Retry" }).click()
  await expect(commitRow(page, featureSubject)).toBeVisible()
})

test("a failed switch to all history never lists the branch as all history", async ({ page }) => {
  let fail = true
  await page.route("**/api/commits?*", async (route) => {
    if (!fail || new URL(route.request().url()).searchParams.get("range") !== "all") return route.continue()
    await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "Couldn't read git history: object missing" }) })
  })
  await openSourceControl(page)
  await expect(commitRow(page, featureSubject)).toBeVisible()
  await sourceControl(page).getByRole("button", { name: "All", exact: true }).click()
  await expect(sourceControl(page).getByRole("alert")).toHaveText("Couldn't read git history: object missing.")
  // Only the pinned rows remain.
  await expect(compareList(page).getByRole("option")).toHaveCount(2)
  fail = false
  await sourceControl(page).getByRole("button", { name: "Retry" }).click()
  await expect(commitRow(page, "Initial")).toBeVisible()
  await expect(compareList(page).getByRole("option")).toHaveCount(4)
})

test("the compare list works from the keyboard", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
  await openSourceControl(page)
  const commit = commitRow(page, featureSubject)
  await expect(commit).toBeVisible()
  await uncommittedRow(page).focus()
  await page.keyboard.press("ArrowDown")
  await expect(branchRow(page)).toBeFocused()
  await expect(branchRow(page)).toHaveAttribute("aria-selected", "true")
  await expect(filesPane(page)).toHaveAccessibleName("Files: vs main")
  await page.keyboard.press("ArrowDown")
  await expect(commit).toBeFocused()
  await expect(commit).toHaveAttribute("aria-selected", "true")

  // Ctrl/Cmd+C copies the focused commit's hash.
  await page.keyboard.press("ControlOrMeta+c")
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toMatch(/^[0-9a-f]{40}$/)

  await page.keyboard.press("Enter")
  await expect(page.getByRole("tab", { name: new RegExp(`^[0-9a-f]{7} ${featureSubject}$`) })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("main")).toBeFocused()

  await commit.focus()
  await page.keyboard.press("Home")
  await expect(uncommittedRow(page)).toBeFocused()
  await expect(uncommittedRow(page)).toHaveAttribute("aria-selected", "true")
})

test("Collapse All Folders and Expand All Folders toggle every Files folder", async ({ page }) => {
  await openSourceControl(page)
  const folders = ["assets", "guides", "notes", "src"]
  for (const name of folders) await expect(fileRow(page, name)).toHaveAttribute("aria-expanded", "true")
  await filesPane(page).getByRole("button", { name: "Collapse All Folders" }).click()
  for (const name of folders) await expect(fileRow(page, name)).toHaveAttribute("aria-expanded", "false")
  await expect(fileRow(page, "app.ts")).toHaveCount(0)
  // Root files stay listed.
  await expect(fileRow(page, "README.md")).toBeVisible()

  await filesPane(page).getByRole("button", { name: "Expand All Folders" }).click()
  for (const name of folders) await expect(fileRow(page, name)).toHaveAttribute("aria-expanded", "true")
  await expect(fileRow(page, "app.ts")).toBeVisible()
  await expect(filesPane(page).getByRole("button", { name: "Collapse All Folders" })).toBeVisible()
})

test("Collapse All Files and Expand All Files toggle every review diff", async ({ page }) => {
  await openReview(page, "review=working")
  for (const file of uncommittedFiles) await expect(fileToggle(page, file)).toHaveAttribute("aria-expanded", "true")
  await expect(diffLines(page, "change-addition", "README.md").first()).toBeVisible()

  await page.getByRole("button", { name: "Collapse All Files" }).click()
  for (const file of uncommittedFiles) await expect(fileToggle(page, file)).toHaveAttribute("aria-expanded", "false")
  await expect(diffLines(page, "change-addition", "README.md")).toHaveCount(0)

  await page.getByRole("button", { name: "Expand All Files" }).click()
  for (const file of uncommittedFiles) await expect(fileToggle(page, file)).toHaveAttribute("aria-expanded", "true")
  await expect(page.getByRole("button", { name: "Collapse All Files" })).toBeVisible()

  // One file's toggle closes only that file.
  await fileToggle(page, "README.md").click()
  await expect(fileToggle(page, "README.md")).toHaveAttribute("aria-expanded", "false")
  await expect(fileToggle(page, "src/app.ts")).toHaveAttribute("aria-expanded", "true")
})

test("Viewed marks collapse files, survive reloads, and clear when the file changes", async ({ page }) => {
  const file = path.join(gitFixtureRoot, "untracked.txt")
  const original = readFileSync(file, "utf8")
  try {
    await openReview(page, "review=working#f=untracked.txt")
    const viewed = reviewFile(page, "untracked.txt").getByRole("button", { name: "Viewed" })
    // Marks wait for the file's content hash.
    await expect(viewed).toBeEnabled()
    await expect(viewed).toHaveAttribute("aria-pressed", "false")
    await viewed.click()
    await expect(viewed).toHaveAttribute("aria-pressed", "true")
    await expect(fileToggle(page, "untracked.txt")).toHaveAttribute("aria-expanded", "false")
    await expect(page.getByRole("main").getByText(`1 of ${gitFixtureChanges} viewed`)).toBeVisible()

    await page.reload()
    await expect(viewed).toHaveAttribute("aria-pressed", "true")
    await expect(fileToggle(page, "untracked.txt")).toHaveAttribute("aria-expanded", "false")
    await expect(page.getByRole("main").getByText(`1 of ${gitFixtureChanges} viewed`)).toBeVisible()

    // New content is a new blob, so the mark no longer applies.
    writeFileSync(file, `${original}Edited after review.\n`)
    await page.reload()
    await expect(reviewFile(page, "untracked.txt")).toContainText("+2 −0")
    await expect(viewed).toHaveAttribute("aria-pressed", "false")
    await expect(fileToggle(page, "untracked.txt")).toHaveAttribute("aria-expanded", "true")
    await expect(page.getByRole("main").getByText(`0 of ${gitFixtureChanges} viewed`)).toBeVisible()
  } finally {
    writeFileSync(file, original)
  }
})

test("sidebar and section headers center their contents", async ({ page }) => {
  await openSourceControl(page)
  await expect(commitRow(page, featureSubject)).toBeVisible()
  await expect(fileRow(page, "README.md")).toBeVisible()
  const middle = (selector: string) => sourceControl(page).locator(selector).first().evaluate((element) => {
    const bounds = element.getBoundingClientRect()
    return bounds.top + bounds.height / 2
  })
  const header = await middle(".sidebar-header")
  expect(await middle(".sidebar-title")).toBeCloseTo(header, 0)
  expect(await middle(".sidebar-header button")).toBeCloseTo(header, 0)
  // Each section's header row sits at its top, its title centered in it.
  for (const title of ["Commits", "Uncommitted"]) {
    const heading = sourceControl(page).getByRole("heading", { level: 3, name: title, exact: true })
    const offsets = await heading.evaluate((element) => {
      const row = element.closest(".source-control-group")
      const section = element.closest(".source-control-section")
      if (!row || !section) return { top: Number.NaN, center: Number.NaN }
      const rowBounds = row.getBoundingClientRect()
      const bounds = element.getBoundingClientRect()
      const top = section.getBoundingClientRect().top + parseFloat(getComputedStyle(section).borderTopWidth)
      return { top: rowBounds.top - top, center: bounds.top + bounds.height / 2 - (rowBounds.top + rowBounds.height / 2) }
    })
    expect(offsets.top, title).toBeCloseTo(0, 0)
    expect(offsets.center, title).toBeCloseTo(0, 0)
  }
})

test("the list and Files resize, and the split is remembered", async ({ page }) => {
  await openSourceControl(page)
  const separator = sourceControl(page).getByRole("separator", { name: "Resize commits and files" })
  await expect(separator).toHaveAttribute("aria-valuenow", "50")
  const listBox = () => compareList(page).locator("xpath=ancestor::div[contains(@class, 'source-control-section')][1]").boundingBox()
  const before = (await listBox())?.height ?? 0
  await separator.focus()
  await page.keyboard.press("ArrowDown")
  await page.keyboard.press("ArrowDown")
  await expect(separator).toHaveAttribute("aria-valuenow", "60")
  await expect.poll(async () => (await listBox())?.height ?? 0).toBeGreaterThan(before)

  await page.reload()
  await expect(separator).toHaveAttribute("aria-valuenow", "60")
  await separator.dblclick()
  await expect(separator).toHaveAttribute("aria-valuenow", "50")
})

test("closing the last review tab ends every diff worker", async ({ page }) => {
  await openReview(page, "review=working#f=src/app.ts")
  await expect(diffLines(page, "change-addition", "src/app.ts").first()).toBeVisible()
  await expect.poll(() => page.workers().length).toBeGreaterThan(0)
  const tab = page.getByRole("tab", { name: "Uncommitted", exact: true })
  await tab.click({ button: "right" })
  await page.getByRole("menuitem", { name: "Close", exact: true }).click()
  await expect(tab).toHaveCount(0)
  await expect.poll(() => page.workers().length).toBe(0)
})
