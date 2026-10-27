import { expect, test } from "@playwright/test"

// Headless Chromium hides scrollbars unless asked to lay them out.
test.use({ launchOptions: { ignoreDefaultArgs: ["--hide-scrollbars"] } })

test("document content keeps its width as overflow comes and goes", async ({ page }) => {
  await page.goto("/view?path=README.md")
  await page.getByRole("button", { name: "Preview", exact: true }).click()
  const main = page.locator("main")
  await expect(page.locator("article")).toBeVisible()
  const width = () => main.evaluate((element) => ({ client: element.clientWidth, overflows: element.scrollHeight > element.clientHeight }))
  const short = await width()
  expect(short.overflows).toBe(false)

  await page.goto("/view?path=guides%2Fdiagrams.md")
  await page.getByRole("button", { name: "Preview", exact: true }).click()
  await expect(page.locator("article")).toBeVisible()
  await expect.poll(async () => (await width()).overflows).toBe(true)
  expect((await width()).client).toBe(short.client)
})
