import { existsSync } from "node:fs"
import path from "node:path"

import { expect, test } from "@playwright/test"

import { gitFixtureRoot } from "./git-fixture.ts"
import { gitApiOrigin } from "./ports.ts"

// The fixture repository is built by the git server's webServer command.
test.skip(() => !existsSync(path.join(gitFixtureRoot, ".git")), "git is not installed")

const project = encodeURIComponent(gitFixtureRoot)

// Builds ship only Highlights and servef's theme (decision 0029); this guards
// that a mapped family still highlights with servef's syntax tokens.
test("diffs highlight mapped families with servef syntax colors", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/view?project=${project}&path=${encodeURIComponent("src/app.ts")}&diff=1`)
  const diff = page.locator(".diff-view")
  await expect(diff.locator('[data-line-type="change-addition"]').first()).toBeVisible()
  await expect.poll(() => diff.evaluate((host) => {
    const keyword = getComputedStyle(document.documentElement).getPropertyValue("--color-syntax-keyword").trim()
    const probe = document.createElement("span")
    probe.style.color = keyword
    document.body.append(probe)
    const expected = getComputedStyle(probe).color
    probe.remove()
    const tokens = [...(host.shadowRoot?.querySelectorAll('[data-line-type="change-addition"] span') ?? [])]
    // Highlights keeps trailing whitespace in a token.
    return tokens.some((token) => token.textContent?.trim() === "export" && getComputedStyle(token).color === expected)
  })).toBe(true)
})
