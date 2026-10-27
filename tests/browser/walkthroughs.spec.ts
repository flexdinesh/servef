import { execFileSync } from "node:child_process"
import { existsSync } from "node:fs"
import path from "node:path"

import { expect, test } from "@playwright/test"

import { gitFixtureBaseWorktree, gitFixtureRoot } from "./git-fixture.ts"
import { gitApiOrigin } from "./ports.ts"

// The fixture repository is built by the git server's webServer command.
test.skip(() => !existsSync(path.join(gitFixtureRoot, ".git")), "git is not installed")

const title = "How run finds its answer"
// Tests run in parallel against one server, so each publishes its own title.
const presentTitle = "Present how run finds its answer"

function walkthrough(id: string, head: string, name = title) {
  return {
    project: gitFixtureRoot,
    id,
    title: name,
    summary: "A short tour of `src/app.ts`.",
    steps: [
      { title: "What this covers", body: "The constant, the function, and the **uncommitted** change." },
      { title: "run returns the constant", body: "`run` hands back `answer`.", file: { path: "src/app.ts", lines: [3, 5] } },
      { title: "The README at HEAD", body: "Pinned to a commit.", file: { path: "README.md", lines: [1, 1], at: head.slice(0, 7) } },
      { title: "The answer went from 1 to 2", body: "The working tree bumps it.", diff: { compare: { kind: "working" }, path: "src/app.ts", lines: [1, 1] } },
      { title: "Written against older code", body: "This line moved on.", file: { path: "src/app.ts", lines: [1, 1], expect: "export const answer = 99" } },
    ],
  }
}

test("a published walkthrough steps through a note, a file, a snapshot, a diff, and a changed step", async ({ page, request }) => {
  const head = execFileSync("git", ["-C", gitFixtureRoot, "rev-parse", "HEAD"], { encoding: "utf8" }).trim()
  const id = "browser-tour"
  const published = await request.post(`${gitApiOrigin}/api/walkthroughs`, { data: walkthrough(id, head) })
  expect(published.status()).toBe(201)
  const { url }: { url: string } = await published.json()

  try {
    await page.goto(url)
    await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible()
    const steps = page.getByRole("listbox", { name: "Steps" })
    await expect(steps.getByRole("option")).toHaveCount(6)
    await expect(page.getByRole("tab", { name: title })).toHaveAttribute("aria-selected", "true")

    await page.getByRole("button", { name: "Start walkthrough" }).click()
    await expect(page.getByRole("region", { name: "Step 1 of 5: What this covers" })).toContainText("uncommitted")
    await expect(steps.getByRole("option", { selected: true })).toHaveAccessibleName(/^Step 1 of 5: What this covers/)

    await page.keyboard.press("ControlOrMeta+ArrowRight")
    await expect(page).toHaveURL(/path=src%2Fapp\.ts.*step=2#L3-L5$/)
    const source = page.getByRole("textbox", { name: "TypeScript source" })
    await expect(source.locator(".source-document-line.selected")).toHaveCount(3)
    await expect(source.getByRole("region", { name: "Step 2 of 5: run returns the constant" })).toContainText("hands back")

    await page.keyboard.press("ControlOrMeta+ArrowRight")
    await expect(page.getByRole("tab", { name: `README.md @ ${head.slice(0, 7)}` })).toHaveAttribute("aria-selected", "true")
    await expect(page.getByRole("region", { name: "Step 3 of 5: The README at HEAD" })).toBeVisible()

    await page.getByRole("button", { name: "Next Step" }).click()
    await expect(page).toHaveURL(/review=working.*step=4#f=src\/app\.ts&R1$/)
    const diffNarration = page.getByRole("region", { name: "Step 4 of 5: The answer went from 1 to 2" })
    await expect(diffNarration).toContainText("bumps it")
    await expect(diffNarration.getByRole("note")).toHaveCount(0)

    await page.keyboard.press("ControlOrMeta+ArrowRight")
    const changed = page.getByRole("region", { name: "Step 5 of 5: Written against older code" })
    await expect(changed.getByRole("note")).toHaveText(/changed after the step was written/)
    await expect(changed.getByRole("button", { name: "Next Step" })).toHaveAttribute("aria-disabled", "true")

    await changed.getByRole("button", { name: "Previous Step" }).click()
    await expect(page).toHaveURL(/step=4/)

    await page.getByRole("region", { name: /^Step 4 of 5/ }).getByRole("button", { name: "Close Walkthrough" }).click()
    await expect(page).not.toHaveURL(/wt=/)
    await expect(page.getByRole("region", { name: /^Step \d of 5/ })).toHaveCount(0)
    await expect(page.getByRole("listbox", { name: "Walkthroughs" }).getByRole("option", { name: new RegExp(`^${title}`) })).toBeVisible()
  } finally {
    await request.delete(`${gitApiOrigin}/api/walkthroughs?project=${encodeURIComponent(gitFixtureRoot)}&id=${id}`)
  }
})

test("present mode hides the chrome until Escape, and phones step from a bar", async ({ page, request }) => {
  const head = execFileSync("git", ["-C", gitFixtureRoot, "rev-parse", "HEAD"], { encoding: "utf8" }).trim()
  const id = "browser-present"
  const published = await request.post(`${gitApiOrigin}/api/walkthroughs`, { data: walkthrough(id, head, presentTitle) })
  expect(published.status()).toBe(201)
  const { url }: { url: string } = await published.json()

  try {
    await page.goto(`${url}&step=2#L3-L5`)
    await page.getByRole("button", { name: "Present", exact: true }).click()
    const bar = page.getByRole("toolbar", { name: `Walkthrough: ${presentTitle}` })
    await expect(bar).toContainText("2 / 5")
    await expect(page.getByRole("navigation", { name: "Views" })).toBeHidden()
    await expect(page.getByRole("tablist", { name: "Open documents" })).toBeHidden()
    await expect(page.locator(".status-bar")).toBeHidden()

    await bar.getByRole("button", { name: "Next Step" }).click()
    await expect(bar).toContainText("3 / 5")
    await page.keyboard.press("Escape")
    await expect(bar).toHaveCount(0)
    await expect(page.getByRole("navigation", { name: "Views" })).toBeVisible()
    await expect(page).toHaveURL(/step=3/)

    await page.setViewportSize({ width: 390, height: 844 })
    const phoneBar = page.getByRole("toolbar", { name: `Walkthrough: ${presentTitle}` })
    await expect(phoneBar).toContainText("3 / 5")
    await expect(page.getByRole("region", { name: /^Step 3 of 5/ })).toBeVisible()
    await phoneBar.getByRole("button", { name: "Previous Step" }).click()
    await expect(phoneBar).toContainText("2 / 5")
    await phoneBar.getByRole("button", { name: "Steps" }).click()
    const steps = page.getByRole("listbox", { name: "Steps" })
    await expect(steps).toBeVisible()
    await steps.getByRole("option", { name: /^Step 4 of 5/ }).click()
    await expect(steps).toBeHidden()
    await expect(phoneBar).toContainText("4 / 5")
  } finally {
    await request.delete(`${gitApiOrigin}/api/walkthroughs?project=${encodeURIComponent(gitFixtureRoot)}&id=${id}`)
  }
})

test("the view copies instructions that tell an agent where to publish", async ({ page }) => {
  // The base worktree has no walkthroughs, so the list shows its empty state.
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
  await page.goto(`${gitApiOrigin}/?project=${encodeURIComponent(gitFixtureBaseWorktree)}`)
  await page.getByRole("navigation", { name: "Views" }).getByRole("button", { name: /^Walkthroughs/ }).click()
  const view = page.locator(".walkthroughs-view")
  await expect(view).toContainText("No walkthroughs for this project yet.")

  const clipboard = () => page.evaluate(() => navigator.clipboard.readText())
  await view.getByRole("button", { name: "Copy agent instructions", exact: true }).click()
  await expect(view.getByRole("button", { name: "Copied", exact: true })).toBeVisible()
  const copied = await clipboard()
  expect(copied).toContain(`Project: ${gitFixtureBaseWorktree}\n`)
  expect(copied).toContain(`POST ${gitApiOrigin}/api/walkthroughs`)
  expect(copied.endsWith("Walk me through: ")).toBe(true)

  await page.evaluate(() => navigator.clipboard.writeText(""))
  const header = view.getByRole("button", { name: "Copy Agent Instructions", exact: true })
  await header.click()
  await expect(header).toHaveAttribute("data-status", "copied")
  await expect.poll(clipboard).toBe(copied)
})
