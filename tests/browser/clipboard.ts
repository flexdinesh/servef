import type { Page } from "@playwright/test"

// Playwright's WebKit has no clipboard permissions to grant, so a test's setup
// would abort there before its body runs.
export async function grantClipboard(page: Page): Promise<void> {
  if (page.context().browser()?.browserType().name() === "webkit") return
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
}
