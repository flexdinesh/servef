import { existsSync, readFileSync, writeFileSync } from "node:fs"
import path from "node:path"

import { expect, test, type Page } from "@playwright/test"

import { rootParam } from "./fixture-root.ts"
import {
  featureSubject, gitFixtureBaseWorktree, gitFixtureChanges, gitFixtureRoot, lowerBranch, lowerSubject, upperBranch,
} from "./git-fixture.ts"
import { gitApiOrigin } from "./ports.ts"

// The fixture repository is built by the git server's webServer command.
test.skip(() => !existsSync(path.join(gitFixtureRoot, ".git")), "git is not installed")

const project = encodeURIComponent(gitFixtureRoot)
// baseProject is a worktree with the base branch, main, checked out.
const baseProject = encodeURIComponent(gitFixtureBaseWorktree)

// Uncommitted changes in review order: folders first, then root files.
const uncommittedFiles = [
  "assets/logo.bin", "guides/start.md", "notes/removed.md", "src/app.ts", "added.md", "large.txt", "README.md", "untracked.txt",
]

type PaneTitle = "Uncommitted" | "Commits" | "Branches"

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

function sourceControl(page: Page) {
  return page.getByRole("region", { name: "Source Control" })
}

function pane(page: Page, title: PaneTitle) {
  return sourceControl(page).getByRole("region", { name: title, exact: true })
}

// paneToggle is the pane header's open/close button.
function paneToggle(page: Page, title: PaneTitle) {
  return pane(page, title).getByRole("heading", { level: 3 }).getByRole("button", { name: title, exact: true })
}

function changesList(page: Page) {
  return pane(page, "Uncommitted").getByRole("listbox", { name: "Uncommitted changes" })
}

function changeRow(page: Page, file: string) {
  return changesList(page).getByRole("option", { name: new RegExp(`^${escapeRegExp(file)}, `) })
}

// changesTree is the Uncommitted pane's tree view.
function changesTree(page: Page, name: string) {
  return pane(page, "Uncommitted").getByRole("treeitem", { name, exact: true })
}

function commitsList(page: Page) {
  return pane(page, "Commits").getByRole("listbox", { name: "Commits" })
}

function commitRow(page: Page, subject: string) {
  return commitsList(page).getByRole("option", { name: new RegExp(`^${escapeRegExp(subject)}, servef, `) })
}

// commitDetails is a commit's expanded details: hidden from the accessibility
// tree, so found through the row's aria-describedby.
async function commitDetails(page: Page, subject: string) {
  const id = await commitRow(page, subject).getAttribute("aria-describedby")
  if (!id) throw new Error(`${subject} shows no details`)
  return page.locator(`[id="${id}"]`)
}

function branchesList(page: Page) {
  return pane(page, "Branches").getByRole("listbox", { name: "Branches" })
}

function branchRow(page: Page, name: string) {
  return branchesList(page).getByRole("option", { name: new RegExp(`^${escapeRegExp(name)}, `) })
}

// rail is a commit, range, or branch review's Files rail.
function rail(page: Page) {
  return page.getByRole("complementary", { name: "Review files" })
}

function railFile(page: Page, name: string) {
  return rail(page).getByRole("treeitem", { name, exact: true })
}

// filesToggle is the diff toolbar's Files rail toggle.
function filesToggle(page: Page) {
  return page.getByRole("button", { name: "Files", exact: true })
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

function commitTab(page: Page, subject: string) {
  return page.getByRole("tab", { name: new RegExp(`^[0-9a-f]{7} ${escapeRegExp(subject)}$`) })
}

async function openReview(page: Page, search: string) {
  await page.goto(`${gitApiOrigin}/view?project=${project}&${search}`)
}

async function openSourceControl(page: Page, which = project) {
  await page.goto(`${gitApiOrigin}/?project=${which}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
}

// reviewSearch reads the page URL's query but the project, ignoring order.
function reviewSearch(page: Page): Record<string, string> {
  const params = new URL(page.url()).searchParams
  params.delete("project")
  return Object.fromEntries(params)
}

// pinnedOffset is the review gutter a file opened by path stops below the
// pane's top edge, where its header sticks.
const pinnedOffset = 8

// topOffset is how far a review section sits below the pane's top edge.
async function topOffset(page: Page, file: string): Promise<number> {
  const [section, main] = await Promise.all([reviewFile(page, file).boundingBox(), page.getByRole("main").boundingBox()])
  return section && main ? section.y - main.y : Number.NaN
}

test("Source Control lists every pending change and reviews them in one tab", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  const activity = page.getByRole("button", { name: `Source Control, ${gitFixtureChanges} pending changes` })
  await expect(activity).toHaveAttribute("aria-pressed", "false")
  await expect(page.locator(".activity-badge")).toHaveText(String(gitFixtureChanges))

  // Explorer rows carry git status; gitignored files are not changes.
  const explorer = page.getByRole("complementary", { name: "Files", exact: true })
  await expect(explorer.locator('[data-item-path="README.md"]')).toHaveAttribute("data-item-git-status", "modified")
  await expect(explorer.locator('[data-item-path="debug.log"]')).not.toHaveAttribute("data-item-git-status", /./)

  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(activity).toHaveAttribute("aria-pressed", "true")
  // The Uncommitted pane counts and lists every change by path.
  await expect(pane(page, "Uncommitted").getByLabel(`${gitFixtureChanges} files`, { exact: true })).toHaveText(String(gitFixtureChanges))
  await expect(changesList(page).getByRole("option")).toHaveCount(gitFixtureChanges)
  for (const file of uncommittedFiles) await expect(changeRow(page, file)).toBeVisible()
  await expect(changeRow(page, "debug.log")).toHaveCount(0)
  await expect(changeRow(page, "clean.md")).toHaveCount(0)
  // Rows name their status and carry line stats.
  await expect(changeRow(page, "guides/start.md")).toHaveAccessibleName(/^guides\/start\.md, Renamed(, |$)/)
  await expect(changeRow(page, "untracked.txt")).toHaveAccessibleName(/^untracked\.txt, Untracked(, |$)/)
  await expect(changeRow(page, "README.md")).toHaveAccessibleName("README.md, Modified, +2 −1")

  await changeRow(page, "README.md").click()
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
  await expect(changeRow(page, "README.md")).toHaveAttribute("aria-selected", "true")
  // The sidebar lists Uncommitted's files, so its review has no Files rail.
  await expect(rail(page)).toHaveCount(0)
  await expect(filesToggle(page)).toHaveCount(0)

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

test("Source Control stacks Uncommitted, Commits, and Branches, open when they have content", async ({ page }) => {
  await openSourceControl(page)
  await expect(commitRow(page, featureSubject)).toBeVisible()
  const titles = await sourceControl(page).getByRole("region").evaluateAll((regions) => regions.map((region) => region.getAttribute("aria-label")))
  expect(titles).toEqual(["Uncommitted", "Commits", "Branches"])
  for (const title of ["Uncommitted", "Commits", "Branches"] as const) {
    await expect(paneToggle(page, title)).toHaveAttribute("aria-expanded", "true")
  }
  await expect(changeRow(page, "README.md")).toBeVisible()
  await expect(branchRow(page, "feature")).toBeVisible()

  // A pane closes from its header, and its separator goes with it.
  await expect(sourceControl(page).getByRole("separator", { name: "Resize Commits and Branches" })).toBeVisible()
  await paneToggle(page, "Commits").click()
  await expect(paneToggle(page, "Commits")).toHaveAttribute("aria-expanded", "false")
  await expect(commitsList(page)).toHaveCount(0)
  await expect(sourceControl(page).getByRole("separator")).toHaveCount(1)
  await expect(sourceControl(page).getByRole("separator", { name: "Resize Uncommitted and Branches" })).toBeVisible()

  // With only Uncommitted open, it fills the view and closed panes sit at the bottom.
  await paneToggle(page, "Branches").click()
  await expect(paneToggle(page, "Branches")).toHaveAttribute("aria-expanded", "false")
  const panes = sourceControl(page).locator(".scm-panes")
  const bottom = async () => {
    const [container, last] = await Promise.all([panes.boundingBox(), sourceControl(page).getByRole("region", { name: "Branches" }).boundingBox()])
    return container && last ? Math.abs(container.y + container.height - (last.y + last.height)) : Infinity
  }
  await expect.poll(bottom).toBeLessThan(1)
})

test("empty panes start collapsed, and a pane's toggle is remembered", async ({ page }) => {
  await openSourceControl(page, baseProject)
  await expect(commitRow(page, "Initial")).toBeVisible()
  // On main nothing is uncommitted and no branch is stacked.
  await expect(paneToggle(page, "Uncommitted")).toHaveAttribute("aria-expanded", "false")
  await expect(paneToggle(page, "Commits")).toHaveAttribute("aria-expanded", "true")
  await expect(paneToggle(page, "Branches")).toHaveAttribute("aria-expanded", "false")
  await expect(pane(page, "Uncommitted").getByLabel(/^\d+ files?$/)).toHaveCount(0)
  await expect(sourceControl(page).getByRole("separator")).toHaveCount(0)

  await paneToggle(page, "Branches").click()
  await expect(paneToggle(page, "Branches")).toHaveAttribute("aria-expanded", "true")
  await expect(pane(page, "Branches").getByText("On main.")).toBeVisible()
  await paneToggle(page, "Commits").click()
  await expect(commitsList(page)).toHaveCount(0)

  await page.reload()
  await expect(paneToggle(page, "Branches")).toHaveAttribute("aria-expanded", "true")
  await expect(pane(page, "Branches").getByText("On main.")).toBeVisible()
  await expect(paneToggle(page, "Commits")).toHaveAttribute("aria-expanded", "false")
  await expect(paneToggle(page, "Uncommitted")).toHaveAttribute("aria-expanded", "false")
})

test("Branch and All are disabled when HEAD is the base branch, which lists all history", async ({ page }) => {
  await openSourceControl(page, baseProject)
  const branch = pane(page, "Commits").getByRole("button", { name: "Branch", exact: true })
  const all = pane(page, "Commits").getByRole("button", { name: "All", exact: true })
  await expect(branch).toBeDisabled()
  await expect(all).toBeDisabled()
  await expect(all).toHaveAttribute("aria-pressed", "true")
  await expect(commitRow(page, "Initial")).toBeVisible()
  await expect(commitsList(page).getByRole("option")).toHaveCount(1)
  await expect(pane(page, "Commits").getByText(/ahead of/)).toHaveCount(0)
  await expect(pane(page, "Branches").getByText("base main")).toBeVisible()
})

test("Uncommitted switches between a list and a tree, and remembers it", async ({ page }) => {
  await openSourceControl(page)
  const uncommitted = pane(page, "Uncommitted")
  await expect(changeRow(page, "src/app.ts")).toBeVisible()
  // Folders collapse only in the tree.
  await expect(uncommitted.getByRole("button", { name: "Collapse All Folders" })).toHaveCount(0)

  await uncommitted.getByRole("button", { name: "View as Tree" }).click()
  await expect(changesList(page)).toHaveCount(0)
  await expect(uncommitted.getByLabel("Uncommitted changes")).toBeVisible()
  await expect(changesTree(page, "app.ts")).toBeVisible()
  await expect(uncommitted.locator('[data-item-path="guides/start.md"]')).toHaveAttribute("data-item-git-status", "renamed")
  await expect(changesTree(page, "README.md")).toContainText("+2 −1")

  await page.reload()
  await expect(changesTree(page, "app.ts")).toBeVisible()
  await expect(uncommitted.getByRole("button", { name: "View as List" })).toBeVisible()

  // Collapse All Folders and Expand All Folders toggle every folder.
  const folders = ["assets", "guides", "notes", "src"]
  for (const name of folders) await expect(changesTree(page, name)).toHaveAttribute("aria-expanded", "true")
  await uncommitted.getByRole("button", { name: "Collapse All Folders" }).click()
  for (const name of folders) await expect(changesTree(page, name)).toHaveAttribute("aria-expanded", "false")
  await expect(changesTree(page, "app.ts")).toHaveCount(0)
  await expect(changesTree(page, "README.md")).toBeVisible()
  await uncommitted.getByRole("button", { name: "Expand All Folders" }).click()
  for (const name of folders) await expect(changesTree(page, name)).toHaveAttribute("aria-expanded", "true")

  // A tree file opens the Uncommitted review there too.
  await changesTree(page, "app.ts").click()
  await expect(page).toHaveURL(/[?&]review=working#f=src\/app\.ts$/)

  await uncommitted.getByRole("button", { name: "View as List" }).click()
  await expect(changeRow(page, "src/app.ts")).toBeVisible()
  await expect(uncommitted.getByRole("button", { name: "Collapse All Folders" })).toHaveCount(0)
  await page.reload()
  await expect(changeRow(page, "src/app.ts")).toBeVisible()
})

test("an uncommitted file opens the Uncommitted review scrolled to it", async ({ page }) => {
  await openSourceControl(page)
  await pane(page, "Uncommitted").getByRole("button", { name: "Open Changes" }).click()
  await expect(page.getByRole("tab", { name: "Uncommitted", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/[?&]review=working$/)
  await expect(page.getByRole("main")).toBeFocused()
  await expect(reviewFile(page, uncommittedFiles[0] ?? "")).toBeVisible()
  expect(await topOffset(page, "src/app.ts")).toBeGreaterThan(pinnedOffset + 2)

  // A click opens the file and keeps focus in the list.
  await changeRow(page, "src/app.ts").click()
  await expect(page).toHaveURL(/#f=src\/app\.ts$/)
  await expect.poll(() => topOffset(page, "src/app.ts")).toBeCloseTo(pinnedOffset, 0)
  await expect(page.locator(".status-path")).toHaveText("src/app.ts")
  await expect(changeRow(page, "src/app.ts")).toHaveAttribute("aria-selected", "true")
  await expect(changeRow(page, "src/app.ts")).toBeFocused()
  // The same tab stays open beside Welcome; no per-file tabs and no rail.
  await expect(page.getByRole("tab")).toHaveText(["Welcome", "Uncommitted"])
  await expect(rail(page)).toHaveCount(0)

  // Rows run in path order; Enter opens and moves focus to the review.
  await page.keyboard.press("ArrowDown")
  await expect(changeRow(page, "untracked.txt")).toBeFocused()
  await page.keyboard.press("Enter")
  await expect(page).toHaveURL(/#f=untracked\.txt$/)
  await expect(page.getByRole("main")).toBeFocused()
})

test("a file too near the end to reach the top stays the file in view", async ({ page }) => {
  await openSourceControl(page)
  await pane(page, "Uncommitted").getByRole("button", { name: "Open Changes" }).click()
  await expect(reviewFile(page, "README.md")).toBeVisible()
  // README.md is next to last, so the pane scrolls to its end to show it.
  await changeRow(page, "README.md").click()
  await expect(page).toHaveURL(/#f=README\.md$/)
  await expect(diffLines(page, "change-addition", "README.md").filter({ hasText: "Four" })).toBeInViewport()
  await expect(page.locator(".status-path")).toHaveText("README.md")
  await expect(page.locator(".status-diff-stats")).toHaveText("+2 −1")
  await expect(changeRow(page, "README.md")).toHaveAttribute("aria-selected", "true")
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
  // Its lines are revealed; the file in view is selected in Uncommitted.
  await expect(reviewFile(page, "src/app.ts").locator(".diff-view [data-additions] [data-column-number=\"2\"]")).toBeInViewport()
  await expect(changeRow(page, "src/app.ts")).toHaveAttribute("aria-selected", "true")
  await expect(changeRow(page, "README.md")).toHaveAttribute("aria-selected", "false")
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
    ["light", "light"], ["dark", "dark"], ["catppuccin", "dark"], ["dracula", "dark"],
    ["gruvbox", "dark"], ["nord", "dark"], ["solarized-light", "light"],
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

test("phones stack the panes, open diffs from a tapped commit, and force Unified", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.getByRole("button", { name: "Open files" }).click()
  await page.getByRole("button", { name: /^Source Control, / }).click()
  const drawer = page.getByRole("dialog", { name: "Source Control" })
  // Phones stack the panes and scroll; nothing splits their height.
  for (const title of ["Uncommitted", "Commits", "Branches"]) {
    await expect(drawer.getByRole("region", { name: title, exact: true })).toBeVisible()
  }
  await expect(drawer.getByRole("separator")).toHaveCount(0)
  const commits = drawer.getByRole("listbox", { name: "Commits" })
  await expect(commits).not.toHaveAttribute("aria-multiselectable", "true")

  // A tap selects a commit and shows its diff button; the diff closes the drawer.
  const commit = commits.getByRole("option", { name: new RegExp(`^${featureSubject}, `) })
  await commit.click()
  await expect(commit).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("tab")).toHaveText(["Welcome"])
  await commit.getByRole("button", { name: "Open changes" }).click()
  await expect(drawer).toBeHidden()
  await expect(commitTab(page, featureSubject)).toHaveAttribute("aria-selected", "true")
  // Phones keep the rail in a sheet the toolbar opens.
  await expect(rail(page)).toHaveCount(0)
  await filesToggle(page).click()
  const sheet = page.getByRole("dialog", { name: "Review files" })
  await expect(sheet).toBeVisible()
  await expect(sheet.getByRole("treeitem", { name: "history.md", exact: true })).toBeVisible()
  await sheet.getByRole("treeitem", { name: "clean.md", exact: true }).click()
  await expect(sheet).toBeHidden()
  await expect(page).toHaveURL(/#f=clean\.md$/)
  await expect(diffLines(page, "change-addition", "clean.md").filter({ hasText: "Edited on the feature branch." })).toBeInViewport()

  await page.getByRole("button", { name: "Open files" }).click()
  await page.getByRole("button", { name: /^Source Control, / }).click()
  await expect(drawer).toBeVisible()
  await drawer.getByRole("listbox", { name: "Uncommitted changes" }).getByRole("option", { name: /^src\/app\.ts, / }).click()
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

test("Split follows the diff area's width, not just the window's", async ({ page }) => {
  await openReview(page, "review=branch#f=clean.md")
  await expect(rail(page)).toBeVisible()
  await rail(page).getByRole("button", { name: "Hide Files" }).click()
  await expect(rail(page)).toHaveCount(0)
  // Size the window so the diff area is just over 48rem without the rail.
  const main = page.getByRole("main")
  const viewport = page.viewportSize()
  const width = (await main.boundingBox())?.width ?? 0
  await page.setViewportSize({ width: Math.round((viewport?.width ?? 0) - width + 16 * 52), height: viewport?.height ?? 720 })
  const split = page.getByRole("button", { name: "Split" })
  const pre = reviewFile(page, "clean.md").locator(".diff-view pre[data-diff-type]")
  await expect(split).not.toHaveAttribute("aria-disabled", "true")
  await expect(split).toHaveAttribute("aria-pressed", "true")
  await expect(pre).toHaveAttribute("data-diff-type", "split")

  // The rail narrows the diff area below 48rem, which forces Unified.
  await filesToggle(page).click()
  await expect(rail(page)).toBeVisible()
  await expect(split).toHaveAttribute("aria-disabled", "true")
  await expect(page.getByRole("button", { name: "Unified" })).toHaveAttribute("aria-pressed", "true")
  await expect(pre).toHaveAttribute("data-diff-type", "single")
})

test("status failures keep the view usable and recover on Retry", async ({ page }) => {
  let fail = true
  await page.route("**/api/changes?*", async (route) => {
    if (!fail) return route.continue()
    await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "Couldn't read git status: index is corrupt" }) })
  })
  await openSourceControl(page)
  await expect(pane(page, "Uncommitted").getByRole("alert")).toContainText("Couldn't read git status: index is corrupt.")
  await expect(page.locator(".activity-badge")).toHaveCount(0)
  // History and the stack still list.
  await expect(commitRow(page, featureSubject)).toBeVisible()
  await expect(branchRow(page, "feature")).toBeVisible()
  fail = false
  await pane(page, "Uncommitted").getByRole("button", { name: "Retry" }).click()
  await expect(changeRow(page, "README.md")).toBeVisible()
  await expect(page.locator(".activity-badge")).toHaveText(String(gitFixtureChanges))
})

test("folders outside git say so", async ({ page }) => {
  await page.goto(`/?project=${rootParam}`)
  await page.keyboard.press("ControlOrMeta+Shift+g")
  await expect(sourceControl(page).getByRole("status")).toHaveText("This folder isn't a git repository.")
  await expect(page.getByRole("button", { name: "Source Control", exact: true })).toBeVisible()
})

test("clicking a commit selects it; its diff opens the commit's review with a Files rail", async ({ page }) => {
  await openSourceControl(page)
  await expect(pane(page, "Commits").getByText("2 ahead of main")).toBeVisible()
  await expect(pane(page, "Commits").getByRole("button", { name: "Branch", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(commitsList(page).getByRole("option")).toHaveCount(2)

  const commit = commitRow(page, featureSubject)
  const lower = commitRow(page, lowerSubject)
  await page.mouse.move(0, 0)
  await expect(commit.getByRole("button", { name: "Open changes" })).toHaveCount(0)
  // A click selects without opening anything.
  await commit.click()
  await expect(commit).toHaveAttribute("aria-selected", "true")
  await expect(lower).toHaveAttribute("aria-selected", "false")
  await expect(page.getByRole("tab")).toHaveText(["Welcome"])
  await expect(page).not.toHaveURL(/[?&]commit=/)
  // The selection keeps its diff button; other rows show theirs on hover.
  await lower.hover()
  await expect(lower.getByRole("button", { name: "Open changes" })).toBeVisible()
  await expect(commit.getByRole("button", { name: "Open changes" })).toBeVisible()
  await commit.getByRole("button", { name: "Open changes" }).click()

  const tab = commitTab(page, featureSubject)
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/[?&]commit=[0-9a-f]{40}$/)
  const main = page.getByRole("main")
  await expect(main.getByRole("heading", { level: 1, name: featureSubject })).toBeVisible()
  await expect(main).toContainText("Explain how the fixture's branch history is built.")
  await expect(main.locator(".review-meta")).toContainText("servef")

  // The rail lists the commit's files with line stats.
  await expect(rail(page).getByRole("heading", { name: "Files" })).toBeVisible()
  await expect(rail(page).getByLabel("2 files", { exact: true })).toHaveText("2")
  await expect(filesToggle(page)).toHaveAttribute("aria-pressed", "true")
  await expect(railFile(page, "clean.md")).toContainText("+2 −0")
  await expect(railFile(page, "history.md")).toContainText("+3 −0")
  // The first file is in view, so it is selected.
  await expect(railFile(page, "history.md")).toHaveAttribute("aria-selected", "true")
  // Folders collapse one at a time too.
  await railFile(page, "docs").click()
  await expect(railFile(page, "history.md")).toHaveCount(0)
  await railFile(page, "docs").click()

  await railFile(page, "clean.md").click()
  await expect(page).toHaveURL(/[?&]commit=[0-9a-f]{40}#f=clean\.md$/)
  await expect(railFile(page, "clean.md")).toHaveAttribute("aria-selected", "true")
  await railFile(page, "history.md").click()
  await expect(page).toHaveURL(/[?&]commit=[0-9a-f]{40}#f=docs\/history\.md$/)
  await expect(diffLines(page, "change-addition", "docs/history.md").filter({ hasText: "Written on the feature branch." })).toBeVisible()
  await expect(page.locator(".status-diff-stats")).toHaveText("+3 −0")
  await expect(railFile(page, "history.md")).toHaveAttribute("aria-selected", "true")

  // A commit compares with its parent, not the working tree.
  await expect(diffLines(page, "change-addition", "clean.md").filter({ hasText: "Edited on the feature branch." })).toBeVisible()

  // Commit tabs survive reloads; Open File opens the working copy.
  await page.reload()
  await expect(commitTab(page, featureSubject)).toHaveAttribute("aria-selected", "true")
  await expect(diffLines(page, "change-addition", "docs/history.md").filter({ hasText: "Written on the feature branch." })).toBeVisible()
  await reviewFile(page, "docs/history.md").getByRole("button", { name: "Open File" }).click()
  await expect(page.getByRole("tab", { name: "history.md", exact: true })).toHaveAttribute("aria-selected", "true")
})

test("commits that tip a stack layer name its branches in their details", async ({ page }) => {
  await openSourceControl(page)
  await expect(commitRow(page, featureSubject)).toHaveAccessibleName(new RegExp(`^${featureSubject}, servef, .+, feature$`))
  await expect(commitRow(page, lowerSubject)).toHaveAccessibleName(new RegExp(`^${lowerSubject}, servef, .+, ${lowerBranch}$`))
  // Branch names stay off the commit line.
  await expect(commitsList(page).locator(".ref-chip")).toHaveCount(0)

  // The chevron shows details.
  const toggle = commitRow(page, featureSubject).getByRole("button", { name: "Show commit details" })
  await toggle.click()
  await expect(commitRow(page, featureSubject).getByRole("button", { name: "Hide commit details" })).toHaveAttribute("aria-expanded", "true")
  const details = await commitDetails(page, featureSubject)
  await expect(details).toContainText("servef <servef@example.invalid>")
  await expect(details.locator(".commit-compare-hash")).toHaveText(/^[0-9a-f]{7}$/)
  await expect(details.locator(".ref-chip")).toHaveText("feature")
  await expect(details.locator(".ref-chip")).toHaveAttribute("data-current")

  // Right shows a commit's details and Left hides them.
  await commitRow(page, lowerSubject).focus()
  await page.keyboard.press("ArrowRight")
  await expect(commitRow(page, lowerSubject)).toHaveAttribute("aria-describedby", /.+/)
  const lower = await commitDetails(page, lowerSubject)
  await expect(lower.locator(".ref-chip")).toHaveText(lowerBranch)
  await expect(lower.locator(".ref-chip")).not.toHaveAttribute("data-current")
  await page.keyboard.press("ArrowLeft")
  await expect(commitRow(page, lowerSubject)).not.toHaveAttribute("aria-describedby")
  await expect(lower).toHaveCount(0)
  // Other commits keep their details open.
  await expect(details).toBeVisible()

  // The details copy the full hash.
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
  await details.getByRole("button", { name: "Copy commit hash", includeHidden: true }).click()
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toMatch(/^[0-9a-f]{40}$/)

  // The base's commit tips no layer.
  await pane(page, "Commits").getByRole("button", { name: "All", exact: true }).click()
  await expect(commitRow(page, "Initial")).toHaveAccessibleName(/^Initial, servef, [^,]+$/)
  await commitRow(page, "Initial").getByRole("button", { name: "Show commit details" }).click()
  await expect(commitRow(page, "Initial")).toHaveAttribute("aria-describedby", /.+/)
  await expect((await commitDetails(page, "Initial")).locator(".ref-chip")).toHaveCount(0)
})

test("a commit's rail scrolls its review to a file and selects the file in view", async ({ page }) => {
  await openSourceControl(page)
  await pane(page, "Commits").getByRole("button", { name: "All", exact: true }).click()
  // Double-click opens a commit's review and focuses it.
  await commitRow(page, "Initial").dblclick()
  await expect(commitTab(page, "Initial")).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("main")).toBeFocused()
  await expect(railFile(page, "app.ts")).toBeVisible()
  // The root commit adds every file.
  await expect(rail(page).locator('[data-item-path="README.md"]')).toHaveAttribute("data-item-git-status", "added")
  await expect(reviewFile(page, "assets/logo.bin")).toBeVisible()
  expect(await topOffset(page, "src/app.ts")).toBeGreaterThan(pinnedOffset + 2)

  await railFile(page, "app.ts").click()
  await expect(page).toHaveURL(/#f=src\/app\.ts$/)
  await expect.poll(() => topOffset(page, "src/app.ts")).toBeCloseTo(pinnedOffset, 0)
  await expect(page.locator(".status-path")).toHaveText("src/app.ts")
  await expect(railFile(page, "app.ts")).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("tab")).toHaveCount(2)

  // Scrolling the review selects the file in view.
  await page.locator("main").evaluate((pane) => {
    pane.dispatchEvent(new WheelEvent("wheel"))
    pane.scrollTop = 0
  })
  await expect(railFile(page, "logo.bin")).toHaveAttribute("aria-selected", "true")
  await expect(railFile(page, "app.ts")).toHaveAttribute("aria-selected", "false")
})

test("a review tab keeps its scroll, files, and rail behind a document tab, without rereading diffs", async ({ page }) => {
  await openSourceControl(page)
  await pane(page, "Commits").getByRole("button", { name: "All", exact: true }).click()
  await commitRow(page, "Initial").dblclick()
  await fileToggle(page, "assets/logo.bin").click()
  await expect(fileToggle(page, "assets/logo.bin")).toHaveAttribute("aria-expanded", "false")
  await railFile(page, "app.ts").click()
  await expect(page).toHaveURL(/#f=src\/app\.ts$/)
  await expect(diffLines(page, "change-addition", "src/app.ts").first()).toBeVisible()
  await expect.poll(() => topOffset(page, "src/app.ts")).toBeCloseTo(pinnedOffset, 0)
  // Scrolling releases the pin; the review is left mid-file.
  const main = page.locator("main")
  const top = await main.evaluate((pane) => {
    pane.dispatchEvent(new WheelEvent("wheel"))
    pane.scrollTop += 24
    return pane.scrollTop
  })
  await expect(railFile(page, "app.ts")).toHaveAttribute("aria-selected", "true")

  const diffReads: string[] = []
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/diff") diffReads.push(request.url())
  })
  await reviewFile(page, "src/app.ts").getByRole("button", { name: "Open File" }).click()
  await expect(page.getByRole("tab", { name: "app.ts" })).toHaveAttribute("aria-selected", "true")
  await expect(rail(page)).toBeHidden()

  await commitTab(page, "Initial").click()
  await expect(commitTab(page, "Initial")).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/#f=src\/app\.ts$/)
  await expect(diffLines(page, "change-addition", "src/app.ts").first()).toBeVisible()
  await expect.poll(() => main.evaluate((pane) => pane.scrollTop)).toBeCloseTo(top, 0)
  await expect(fileToggle(page, "assets/logo.bin")).toHaveAttribute("aria-expanded", "false")
  await expect(railFile(page, "app.ts")).toHaveAttribute("aria-selected", "true")
  await expect(page.locator(".status-path")).toHaveText("src/app.ts")
  expect(diffReads).toEqual([])
})

test("a pinned review file stays the file in view behind a document tab", async ({ page }) => {
  await openSourceControl(page)
  const current = branchRow(page, "feature")
  await current.hover()
  await current.getByRole("button", { name: "Open changes on feature since main" }).click()
  const tab = page.getByRole("tab", { name: "vs main", exact: true })
  // clean.md is last, too near the end to reach the top.
  await railFile(page, "clean.md").click()
  await expect(page).toHaveURL(/#f=clean\.md$/)
  await expect(railFile(page, "clean.md")).toHaveAttribute("aria-selected", "true")
  await reviewFile(page, "clean.md").getByRole("button", { name: "Open File" }).click()
  await expect(page.getByRole("tab", { name: "clean.md" })).toHaveAttribute("aria-selected", "true")

  await page.goBack()
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/#f=clean\.md$/)
  await expect(railFile(page, "clean.md")).toHaveAttribute("aria-selected", "true")
  await expect(page.locator(".status-path")).toHaveText("clean.md")
})

test("Collapse All Folders and Expand All Folders toggle every rail folder", async ({ page }) => {
  await openSourceControl(page)
  await pane(page, "Commits").getByRole("button", { name: "All", exact: true }).click()
  await commitRow(page, "Initial").dblclick()
  const folders = ["assets", "guides", "notes", "src"]
  for (const name of folders) await expect(railFile(page, name)).toHaveAttribute("aria-expanded", "true")
  await rail(page).getByRole("button", { name: "Collapse All Folders" }).click()
  for (const name of folders) await expect(railFile(page, name)).toHaveAttribute("aria-expanded", "false")
  await expect(railFile(page, "app.ts")).toHaveCount(0)
  // Root files stay listed.
  await expect(railFile(page, "README.md")).toBeVisible()

  await rail(page).getByRole("button", { name: "Expand All Folders" }).click()
  for (const name of folders) await expect(railFile(page, name)).toHaveAttribute("aria-expanded", "true")
  await expect(railFile(page, "app.ts")).toBeVisible()
  await expect(rail(page).getByRole("button", { name: "Collapse All Folders" })).toBeVisible()
})

test("the review rail hides and resizes, and remembers both", async ({ page }) => {
  await openReview(page, "review=branch")
  await expect(railFile(page, "clean.md")).toBeVisible()
  const separator = rail(page).getByRole("separator", { name: "Resize review files" })
  await expect(separator).toHaveAttribute("aria-valuenow", "16")
  const width = async () => (await rail(page).boundingBox())?.width ?? 0
  const before = await width()
  await separator.focus()
  await page.keyboard.press("ArrowLeft")
  await page.keyboard.press("ArrowLeft")
  await expect(separator).toHaveAttribute("aria-valuenow", "18")
  await expect.poll(width).toBeGreaterThan(before)
  await page.keyboard.press("ArrowRight")
  await expect(separator).toHaveAttribute("aria-valuenow", "17")

  await page.reload()
  await expect(separator).toHaveAttribute("aria-valuenow", "17")
  await separator.dblclick()
  await expect(separator).toHaveAttribute("aria-valuenow", "16")

  await rail(page).getByRole("button", { name: "Hide Files" }).click()
  await expect(rail(page)).toHaveCount(0)
  await expect(filesToggle(page)).toHaveAttribute("aria-pressed", "false")
  await page.reload()
  await expect(reviewFile(page, "clean.md")).toBeVisible()
  await expect(rail(page)).toHaveCount(0)
  await filesToggle(page).click()
  await expect(railFile(page, "clean.md")).toBeVisible()
  await expect(filesToggle(page)).toHaveAttribute("aria-pressed", "true")
  await page.reload()
  await expect(railFile(page, "clean.md")).toBeVisible()
})

test("the commit range switches to all history and is remembered", async ({ page }) => {
  await openSourceControl(page)
  await expect(commitRow(page, featureSubject)).toBeVisible()
  await pane(page, "Commits").getByRole("button", { name: "All", exact: true }).click()
  await expect(commitRow(page, "Initial")).toBeVisible()
  await expect(commitsList(page).getByRole("option")).toHaveCount(3)
  await expect(pane(page, "Commits").getByText("2 ahead of main")).toHaveCount(0)

  await page.reload()
  await expect(pane(page, "Commits").getByRole("button", { name: "All", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(commitRow(page, "Initial")).toBeVisible()
  await pane(page, "Commits").getByRole("button", { name: "Branch", exact: true }).click()
  await expect(commitsList(page).getByRole("option")).toHaveCount(2)
  await expect(pane(page, "Commits").getByText("2 ahead of main")).toBeVisible()
})

test("Branches lists the stack top to bottom, HEAD's layer between, then the base", async ({ page }) => {
  await openSourceControl(page)
  const branches = pane(page, "Branches")
  await expect(branches.getByText("base main", { exact: true })).toBeVisible()
  const options = branchesList(page).getByRole("option")
  await expect(options).toHaveCount(3)
  const names = await options.evaluateAll((rows) => rows.map((row) => row.getAttribute("aria-label")))
  expect(names).toEqual([
    `${upperBranch}, 1 commit on feature`,
    `feature, checked out, 1 commit on ${lowerBranch}`,
    `${lowerBranch}, 1 commit on main`,
  ])
  // The base closes the stack at the merge-base; it opens nothing.
  const base = branches.locator(".branch-row-base")
  await expect(base).toContainText("main")
  await expect(base).toContainText(/[0-9a-f]{7}/)
  await expect(base).toContainText("merge-base")
  await expect(base.getByRole("button")).toHaveCount(0)
})

test("a layer opens its own commits or everything since the base", async ({ page }) => {
  await openSourceControl(page)
  const upper = branchRow(page, upperBranch)
  const lower = branchRow(page, lowerBranch)
  // The lowest layer's own commits are its base diff, so it offers only that.
  await lower.hover()
  await expect(lower.getByRole("button", { name: `Open changes on ${lowerBranch} since main` })).toBeVisible()
  await expect(lower.getByRole("button", { name: /since (?!main)/ })).toHaveCount(0)

  // A layer above HEAD against the layer below.
  await upper.hover()
  await upper.getByRole("button", { name: `Open changes on ${upperBranch} since feature` }).click()
  await expect(page.getByRole("tab", { name: `${upperBranch} vs feature`, exact: true })).toHaveAttribute("aria-selected", "true")
  expect(reviewSearch(page)).toEqual({ review: "branch", base: "feature", head: upperBranch })
  await expect(page.getByRole("main").getByText(`1 commit on ${upperBranch} ahead of feature`)).toBeVisible()
  await expect(rail(page).getByLabel("1 file", { exact: true })).toBeVisible()
  await expect(railFile(page, "changelog.md")).toBeVisible()
  await expect(railFile(page, "history.md")).toHaveCount(0)
  await expect(diffLines(page, "change-addition", "docs/changelog.md").filter({ hasText: "Written on the upper layer." })).toBeVisible()

  // The same layer against the base.
  await upper.hover()
  await upper.getByRole("button", { name: `Open changes on ${upperBranch} since main` }).click()
  await expect(page.getByRole("tab", { name: `${upperBranch} vs main`, exact: true })).toHaveAttribute("aria-selected", "true")
  expect(reviewSearch(page)).toEqual({ review: "branch", head: upperBranch })
  await expect(page.getByRole("main").getByText(`3 commits on ${upperBranch} ahead of main`)).toBeVisible()
  await expect(rail(page).getByLabel("4 files", { exact: true })).toBeVisible()
  await expect(railFile(page, "changelog.md")).toBeVisible()
  await expect(railFile(page, "index.md")).toBeVisible()
  await expect(railFile(page, "history.md")).toBeVisible()
  await expect(railFile(page, "clean.md")).toBeVisible()

  // HEAD's layer against the layer below compares HEAD.
  const current = branchRow(page, "feature")
  await current.hover()
  await current.getByRole("button", { name: `Open changes on feature since ${lowerBranch}` }).click()
  await expect(page.getByRole("tab", { name: `vs ${lowerBranch}`, exact: true })).toHaveAttribute("aria-selected", "true")
  expect(reviewSearch(page)).toEqual({ review: "branch", base: lowerBranch })
  await expect(rail(page).getByLabel("2 files", { exact: true })).toBeVisible()
  await expect(railFile(page, "history.md")).toBeVisible()
  await expect(railFile(page, "clean.md")).toBeVisible()
  await expect(railFile(page, "index.md")).toHaveCount(0)

  // HEAD's layer against the base is the branch review as before.
  await current.hover()
  await current.getByRole("button", { name: "Open changes on feature since main" }).click()
  await expect(page.getByRole("tab", { name: "vs main", exact: true })).toHaveAttribute("aria-selected", "true")
  expect(reviewSearch(page)).toEqual({ review: "branch" })
  await expect(page.getByRole("main").getByText("2 commits ahead of main")).toBeVisible()
  await expect(rail(page).getByLabel("3 files", { exact: true })).toBeVisible()
  await expect(railFile(page, "index.md")).toBeVisible()
  await expect(railFile(page, "changelog.md")).toHaveCount(0)

  // Double-click opens a layer's base diff.
  await lower.dblclick()
  await expect(page.getByRole("tab", { name: `${lowerBranch} vs main`, exact: true })).toHaveAttribute("aria-selected", "true")
  expect(reviewSearch(page)).toEqual({ review: "branch", head: lowerBranch })
  await expect(rail(page).getByLabel("1 file", { exact: true })).toBeVisible()
  await expect(railFile(page, "index.md")).toBeVisible()
  await expect(page.getByRole("main")).toBeFocused()

  // Layer reviews survive reloads.
  await page.reload()
  await expect(page.getByRole("tab", { name: `${lowerBranch} vs main`, exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(diffLines(page, "change-addition", "docs/index.md").filter({ hasText: "Written on the lower layer." })).toBeVisible()
})

test("HEAD's branch review follows HEAD and survives reloads", async ({ page }) => {
  await openSourceControl(page)
  const current = branchRow(page, "feature")
  await current.hover()
  await current.getByRole("button", { name: "Open changes on feature since main" }).click()
  const tab = page.getByRole("tab", { name: "vs main", exact: true })
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("main").getByText("merge-base")).toBeVisible()
  await railFile(page, "clean.md").click()
  await expect(page).toHaveURL(/[?&]review=branch#f=clean\.md$/)
  await railFile(page, "history.md").click()
  await expect(page).toHaveURL(/[?&]review=branch#f=docs\/history\.md$/)
  await expect(diffLines(page, "change-addition", "docs/history.md").filter({ hasText: "Written on the feature branch." })).toBeVisible()
  await expect(railFile(page, "history.md")).toHaveAttribute("aria-selected", "true")

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

test("HEAD's branch review adds uncommitted changes in place and marks their files", async ({ page }) => {
  await openReview(page, "review=branch#f=clean.md")
  const branchTab = page.getByRole("tab", { name: "vs main", exact: true })
  await expect(branchTab).toHaveAttribute("aria-selected", "true")
  const scope = page.getByRole("main").getByRole("group", { name: "Changes to include" })
  const committed = scope.getByRole("button", { name: "Committed", exact: true })
  const uncommitted = scope.getByRole("button", { name: `Include uncommitted changes, ${gitFixtureChanges} files` })
  await expect(committed).toHaveAttribute("aria-pressed", "true")
  await expect(uncommitted).toHaveAttribute("aria-pressed", "false")
  await expect(reviewFile(page, "docs/history.md")).toBeVisible()
  await expect(reviewFile(page, "README.md")).toHaveCount(0)
  await expect(page.getByRole("main").getByRole("img", { name: "Has uncommitted changes" })).toHaveCount(0)

  // The tab switches in place and keeps the file in view.
  await uncommitted.click()
  const combinedTab = page.getByRole("tab", { name: "vs main + uncommitted", exact: true })
  await expect(combinedTab).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("tab")).toHaveCount(1)
  expect(reviewSearch(page)).toEqual({ review: "branch", uncommitted: "1" })
  await expect(page).toHaveURL(/#f=clean\.md$/)
  await expect(uncommitted).toHaveAttribute("aria-pressed", "true")

  // Committed and uncommitted changes read from the merge base to the
  // working tree; only files with uncommitted edits carry the dot.
  await expect(reviewFile(page, "docs/history.md")).toBeVisible()
  await expect(reviewFile(page, "docs/index.md")).toBeVisible()
  await expect(reviewFile(page, "untracked.txt")).toBeVisible()
  await expect(reviewFile(page, "README.md").getByRole("img", { name: "Has uncommitted changes" })).toBeVisible()
  await expect(reviewFile(page, "clean.md").getByRole("img", { name: "Has uncommitted changes" })).toHaveCount(0)
  await expect(diffLines(page, "change-addition", "clean.md").filter({ hasText: "Edited on the feature branch." })).toBeVisible()
  await expect(rail(page).locator('[data-item-path="README.md"] [title^="Has uncommitted changes"]')).toBeVisible()
  await expect(rail(page).locator('[data-item-path="clean.md"] [title^="Has uncommitted changes"]')).toHaveCount(0)
  await reviewFile(page, "README.md").scrollIntoViewIfNeeded()
  await expect(diffLines(page, "change-addition", "README.md").filter({ hasText: "Two, changed" })).toBeVisible()

  // The scope survives a reload, and switching back restores the tab.
  await page.reload()
  await expect(combinedTab).toHaveAttribute("aria-selected", "true")
  await committed.click()
  await expect(branchTab).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("tab")).toHaveCount(1)
  expect(reviewSearch(page)).toEqual({ review: "branch" })
  await expect(reviewFile(page, "README.md")).toHaveCount(0)
})

test("a clean checkout's branch review keeps + Uncommitted, disabled with its reason", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/view?project=${baseProject}&review=branch`)
  const scope = page.getByRole("main").getByRole("group", { name: "Changes to include" })
  const uncommitted = scope.getByRole("button", { name: "Include uncommitted changes", exact: true })
  await expect(uncommitted).toHaveAttribute("aria-disabled", "true")
  await expect(uncommitted).toHaveAttribute("title", "No uncommitted changes")
  // aria-disabled keeps it focusable; a click does nothing.
  await uncommitted.dispatchEvent("click")
  await expect(scope.getByRole("button", { name: "Committed", exact: true })).toHaveAttribute("aria-pressed", "true")
  expect(reviewSearch(page)).toEqual({ review: "branch" })
})

test("the base picker changes the stack's base and is remembered", async ({ page }) => {
  await openSourceControl(page)
  const branches = pane(page, "Branches")
  await expect(branches.getByText("base main", { exact: true })).toBeVisible()
  await branches.getByRole("button", { name: "Change base branch" }).click()
  const filter = page.getByRole("combobox", { name: "Filter branches" })
  await expect(filter).toBeFocused()
  const choices = page.getByRole("dialog", { name: "Base branch" }).getByRole("listbox", { name: "Branches" })
  await expect(choices.getByRole("option", { name: /^Default/ })).toHaveAttribute("aria-selected", "true")
  await filter.fill("rel")
  await expect(choices.getByRole("option")).toHaveCount(1)
  await choices.getByRole("option", { name: "release" }).click()
  await expect(choices).toBeHidden()

  await expect(branches.getByText("base release", { exact: true })).toBeVisible()
  await expect(branchRow(page, lowerBranch)).toHaveAccessibleName(`${lowerBranch}, 1 commit on release`)
  await expect(pane(page, "Commits").getByText("2 ahead of release")).toBeVisible()
  const current = branchRow(page, "feature")
  await current.hover()
  await current.getByRole("button", { name: "Open changes on feature since release" }).click()
  expect(reviewSearch(page)).toEqual({ review: "branch", base: "release" })
  await expect(page.getByRole("tab", { name: "vs release", exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("main").getByText("2 commits ahead of release")).toBeVisible()
  await expect(railFile(page, "history.md")).toBeVisible()
  // A layer's diff keeps the chosen base.
  const lower = branchRow(page, lowerBranch)
  await lower.hover()
  await lower.getByRole("button", { name: `Open changes on ${lowerBranch} since release` }).click()
  expect(reviewSearch(page)).toEqual({ review: "branch", base: "release", head: lowerBranch })
  await expect(page.getByRole("tab", { name: `${lowerBranch} vs release`, exact: true })).toHaveAttribute("aria-selected", "true")

  // The choice is remembered for the project.
  await page.reload()
  await expect(branches.getByText("base release", { exact: true })).toBeVisible()
  await branches.getByRole("button", { name: "Change base branch" }).click()
  await expect(choices.getByRole("option", { name: "release" })).toHaveAttribute("aria-selected", "true")
})

test("Shift-selecting commits opens their range from the newest commit's diff", async ({ page }) => {
  await openSourceControl(page)
  await pane(page, "Commits").getByRole("button", { name: "All", exact: true }).click()
  const feature = commitRow(page, featureSubject)
  const lower = commitRow(page, lowerSubject)
  const initial = commitRow(page, "Initial")
  await expect(initial).toBeVisible()
  await expect(commitsList(page)).toHaveAttribute("aria-multiselectable", "true")

  await feature.click()
  await initial.click({ modifiers: ["Shift"] })
  for (const row of [feature, lower, initial]) await expect(row).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("tab")).toHaveText(["Welcome"])
  // Only the newest selected commit offers the range's diff.
  await lower.hover()
  await expect(lower.getByRole("button", { name: /^Open changes/ })).toHaveCount(0)
  await initial.hover()
  await expect(initial.getByRole("button", { name: /^Open changes/ })).toHaveCount(0)
  await expect(feature.getByRole("button", { name: "Open changes in the selected commits" })).toBeVisible()
  await feature.getByRole("button", { name: "Open changes in the selected commits" }).click()

  const tab = page.getByRole("tab", { name: /^[0-9a-f]{7}\.\.[0-9a-f]{7}$/ })
  await expect(tab).toHaveAttribute("aria-selected", "true")
  await expect(page).toHaveURL(/[?&]from=[0-9a-f]{40}&to=[0-9a-f]{40}$/)
  await expect(page.getByRole("main").getByText(/^3 commits$/)).toBeVisible()
  // The oldest selected commit is the root, so every file is added.
  await expect(railFile(page, "history.md")).toBeVisible()
  await expect(railFile(page, "index.md")).toBeVisible()
  await expect(rail(page).locator('[data-item-path="clean.md"]')).toHaveAttribute("data-item-git-status", "added")
  await railFile(page, "clean.md").click()
  await expect(page).toHaveURL(/[?&]from=[0-9a-f]{40}&to=[0-9a-f]{40}#f=clean\.md$/)
  await expect(diffLines(page, "change-addition", "clean.md").filter({ hasText: "Edited on the feature branch." })).toBeVisible()

  // Escape narrows to the commit the range started from; the tab stays open.
  await feature.focus()
  await page.keyboard.press("Escape")
  await expect(feature).toHaveAttribute("aria-selected", "true")
  await expect(lower).toHaveAttribute("aria-selected", "false")
  await expect(initial).toHaveAttribute("aria-selected", "false")
  await expect(feature.getByRole("button", { name: "Open changes" })).toBeVisible()
  await expect(tab).toBeVisible()

  // Shift+Arrow extends from the focused commit; Escape narrows it again.
  await page.keyboard.press("Shift+ArrowDown")
  await expect(lower).toBeFocused()
  await expect(lower).toHaveAttribute("aria-selected", "true")
  await expect(feature).toHaveAttribute("aria-selected", "true")
  await page.keyboard.press("Escape")
  await expect(lower).toHaveAttribute("aria-selected", "false")
  await expect(feature).toHaveAttribute("aria-selected", "true")

  // Switching the list to the branch narrows a range to its newest commit.
  await initial.click({ modifiers: ["Shift"] })
  await expect(initial).toHaveAttribute("aria-selected", "true")
  await pane(page, "Commits").getByRole("button", { name: "Branch", exact: true }).click()
  await expect(initial).toHaveCount(0)
  await expect(feature).toHaveAttribute("aria-selected", "true")
  await expect(lower).toHaveAttribute("aria-selected", "false")

  await page.reload()
  await expect(page.getByRole("tab", { name: /^[0-9a-f]{7}\.\.[0-9a-f]{7}$/ })).toHaveAttribute("aria-selected", "true")
  await expect(diffLines(page, "change-addition", "clean.md").filter({ hasText: "Edited on the feature branch." })).toBeVisible()
})

test("a branch with nothing ahead offers all history", async ({ page }) => {
  await page.route("**/api/commits?*", async (route) => {
    if (new URL(route.request().url()).searchParams.get("range") !== "branch") return route.continue()
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
      repository: true, branch: "feature", base: "main", range: "branch", head: "a".repeat(40), ahead: 0, commits: [], more: false,
    }) })
  })
  await openSourceControl(page)
  // An empty pane starts collapsed.
  await expect(paneToggle(page, "Commits")).toHaveAttribute("aria-expanded", "false")
  await paneToggle(page, "Commits").click()
  await expect(pane(page, "Commits").getByText("No commits ahead of main.")).toBeVisible()
  await pane(page, "Commits").getByRole("button", { name: "Show all history" }).click()
  await expect(pane(page, "Commits").getByRole("button", { name: "All", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(commitRow(page, "Initial")).toBeVisible()
})

test("Load more ends the commit list as a row and recovers on Retry", async ({ page }) => {
  let failMore = true
  await page.route("**/api/commits?*", async (route) => {
    const params = new URL(route.request().url()).searchParams
    if (params.get("range") !== "all") return route.continue()
    if (params.has("skip")) {
      if (!failMore) return route.continue()
      return route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "Couldn't read git history: object missing" }) })
    }
    // The first page holds only the newest commit.
    const response = await route.fetch()
    const body: unknown = await response.json()
    if (typeof body !== "object" || body === null || !("commits" in body) || !Array.isArray(body.commits)) throw new Error("unexpected commits response")
    await route.fulfill({ response, json: { ...body, commits: body.commits.slice(0, 1), more: true } })
  })
  await openSourceControl(page)
  await pane(page, "Commits").getByRole("button", { name: "All", exact: true }).click()
  const more = pane(page, "Commits").getByRole("button", { name: "Load more" })
  await expect(more).toBeVisible()
  await expect(commitsList(page).getByRole("option")).toHaveCount(1)

  // It is a 22px list row directly under the last commit, not a raised button.
  const [row, last] = await Promise.all([more.boundingBox(), commitsList(page).getByRole("option").last().boundingBox()])
  expect(row?.height).toBe(22)
  expect(row && last ? Math.abs(row.y - (last.y + last.height)) : Number.NaN).toBeLessThan(1)

  await more.click()
  await expect(pane(page, "Commits").getByRole("alert")).toContainText("object missing")
  failMore = false
  await pane(page, "Commits").getByRole("button", { name: "Retry" }).click()
  await expect(commitRow(page, "Initial")).toBeVisible()
  await expect(pane(page, "Commits").getByRole("button", { name: /^(Load more|Retry)$/ })).toHaveCount(0)
})

test("history failures alert in the Commits pane and recover on Retry", async ({ page }) => {
  let fail = true
  await page.route("**/api/commits?*", async (route) => {
    if (!fail) return route.continue()
    await route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "Couldn't read git history: object missing" }) })
  })
  await openSourceControl(page)
  await expect(pane(page, "Commits").getByRole("alert")).toHaveText("Couldn't read git history: object missing.")
  await expect(changeRow(page, "README.md")).toBeVisible()
  fail = false
  await pane(page, "Commits").getByRole("button", { name: "Retry" }).click()
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
  await pane(page, "Commits").getByRole("button", { name: "All", exact: true }).click()
  await expect(pane(page, "Commits").getByRole("alert")).toHaveText("Couldn't read git history: object missing.")
  await expect(commitsList(page).getByRole("option")).toHaveCount(0)
  fail = false
  await pane(page, "Commits").getByRole("button", { name: "Retry" }).click()
  await expect(commitRow(page, "Initial")).toBeVisible()
  await expect(commitsList(page).getByRole("option")).toHaveCount(3)
})

test("the panes work from the keyboard", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
  await openSourceControl(page)
  const feature = commitRow(page, featureSubject)
  const lower = commitRow(page, lowerSubject)
  await expect(feature).toBeVisible()
  await feature.focus()
  await page.keyboard.press("ArrowDown")
  await expect(lower).toBeFocused()
  await expect(lower).toHaveAttribute("aria-selected", "true")
  await page.keyboard.press("Home")
  await expect(feature).toBeFocused()
  await expect(feature).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("tab")).toHaveText(["Welcome"])

  // Ctrl/Cmd+C copies the focused commit's hash.
  await page.keyboard.press("ControlOrMeta+c")
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toMatch(/^[0-9a-f]{40}$/)

  await page.keyboard.press("Enter")
  await expect(commitTab(page, featureSubject)).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("main")).toBeFocused()

  // Space opens an uncommitted file and keeps focus in the list.
  await changeRow(page, "assets/logo.bin").focus()
  await page.keyboard.press("End")
  await expect(changeRow(page, "untracked.txt")).toBeFocused()
  await page.keyboard.press("Space")
  await expect(page).toHaveURL(/[?&]review=working#f=untracked\.txt$/)
  await expect(changeRow(page, "untracked.txt")).toBeFocused()

  // Branches start from HEAD's layer; Enter opens a layer's base diff.
  await branchesList(page).getByRole("option", { name: /, checked out, / }).focus()
  await page.keyboard.press("ArrowDown")
  await expect(branchRow(page, lowerBranch)).toBeFocused()
  await page.keyboard.press("Enter")
  await expect(page.getByRole("tab", { name: `${lowerBranch} vs main`, exact: true })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("main")).toBeFocused()
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

  // A file header's hover cue is the pointer alone; no fill.
  const header = fileToggle(page, "src/app.ts")
  await header.hover()
  await expect(header).toHaveCSS("cursor", "pointer")
  await expect(header).toHaveCSS("background-image", "none")
  await expect(header).toHaveCSS("background-color", "rgba(0, 0, 0, 0)")
})

test("the review header and rail are islands one gutter in from the tab strip", async ({ page }) => {
  await openReview(page, "review=branch")
  await expect(railFile(page, "clean.md")).toBeVisible()
  const [main, header, column, card] = await Promise.all([
    page.getByRole("main").boundingBox(),
    page.getByRole("main").locator(".review-header").boundingBox(),
    rail(page).boundingBox(),
    rail(page).locator(".review-rail-panel").boundingBox(),
  ])
  if (!main || !header || !column || !card) throw new Error("review layout is not rendered")
  expect(header.y - main.y).toBeCloseTo(pinnedOffset, 0)
  expect(card.y - column.y).toBeCloseTo(pinnedOffset, 0)
  expect(card.y).toBeCloseTo(header.y, 0)
  expect(column.x + column.width - (card.x + card.width)).toBeCloseTo(pinnedOffset, 0)
  expect(column.y + column.height - (card.y + card.height)).toBeCloseTo(pinnedOffset, 0)
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

test("sidebar and pane headers center their contents", async ({ page }) => {
  await openSourceControl(page)
  await expect(commitRow(page, featureSubject)).toBeVisible()
  await expect(changeRow(page, "README.md")).toBeVisible()
  const middle = (selector: string) => sourceControl(page).locator(selector).first().evaluate((element) => {
    const bounds = element.getBoundingClientRect()
    return bounds.top + bounds.height / 2
  })
  const header = await middle(".sidebar-header")
  expect(await middle(".sidebar-title")).toBeCloseTo(header, 0)
  expect(await middle(".sidebar-header button")).toBeCloseTo(header, 0)
  // Each pane's header row sits at its top, its title centered in it.
  for (const title of ["Uncommitted", "Commits", "Branches"] as const) {
    const offsets = await paneToggle(page, title).evaluate((element) => {
      const section = element.closest("section")
      const row = element.closest("h3")?.parentElement
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

test("open panes resize, and the split is remembered", async ({ page }) => {
  await openSourceControl(page)
  const separator = sourceControl(page).getByRole("separator", { name: "Resize Uncommitted and Commits" })
  await expect(separator).toHaveAttribute("aria-valuenow", "44")
  const height = async () => (await pane(page, "Uncommitted").boundingBox())?.height ?? 0
  const before = await height()
  await separator.focus()
  await page.keyboard.press("ArrowDown")
  await page.keyboard.press("ArrowDown")
  await expect(separator).toHaveAttribute("aria-valuenow", "56")
  await expect.poll(height).toBeGreaterThan(before)

  await page.reload()
  await expect(separator).toHaveAttribute("aria-valuenow", "56")
  await separator.dblclick()
  await expect(separator).toHaveAttribute("aria-valuenow", "44")
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
