import { expect, test, type Page } from "@playwright/test"

// openColorTheme runs Preferences: Color Theme from the command palette.
async function openColorTheme(page: Page) {
  // The palette shortcut listens once the workspace has rendered.
  await expect(page.getByRole("button", { name: "Settings", exact: true })).toBeVisible()
  await page.keyboard.press("ControlOrMeta+Shift+P")
  const palette = page.getByRole("dialog", { name: "Command palette" })
  await palette.getByRole("combobox", { name: "Command" }).fill("color theme")
  await page.keyboard.press("Enter")
  const picker = page.getByRole("dialog", { name: "Color Theme" })
  await expect(picker).toBeVisible()
  return picker
}

function storedTheme(page: Page) {
  return page.evaluate(() => window.localStorage.getItem("servef-theme"))
}

test("the Color Theme pick previews on arrows, reverts on Escape, and keeps on Enter", async ({ page }) => {
  await page.goto("/")
  const html = page.locator("html")

  const picker = await openColorTheme(page)
  const input = picker.getByRole("combobox", { name: "Color theme" })
  await expect(input).toBeFocused()
  await expect(input).toHaveAttribute("placeholder", "Select Color Theme (Up/Down Keys to Preview)")
  await expect(picker.getByRole("group", { name: "light themes" })).toBeVisible()
  await expect(picker.getByRole("group", { name: "dark themes" })).toBeVisible()
  await expect(picker.getByRole("option", { name: "Dark", exact: true })).toHaveAttribute("aria-selected", "true")

  await page.keyboard.press("ArrowUp")
  await expect(html).toHaveAttribute("data-theme", "white")
  await expect(html).toHaveClass(/light/)
  await page.keyboard.press("ArrowUp")
  await expect(html).toHaveAttribute("data-theme", "solarized-light")
  expect(await storedTheme(page)).toBeNull()

  await page.keyboard.press("Escape")
  await expect(picker).toBeHidden()
  await expect(html).toHaveAttribute("data-theme", "dark")
  expect(await storedTheme(page)).toBeNull()

  await openColorTheme(page)
  await input.fill("tokyo")
  await expect(html).toHaveAttribute("data-theme", "tokyo-night")
  await expect(html).toHaveClass(/dark/)
  await page.keyboard.press("Enter")
  await expect(picker).toBeHidden()
  await expect(html).toHaveAttribute("data-theme", "tokyo-night")
  expect(await storedTheme(page)).toBe("tokyo-night")

  await page.reload()
  await expect(html).toHaveAttribute("data-theme", "tokyo-night")
  await page.keyboard.press("ControlOrMeta+Shift+P")
  await expect(page.getByRole("option", { name: "Preferences: Color Theme, Tokyo Night" })).toBeVisible()
  await page.keyboard.press("Escape")
})

test("the Color Theme pick previews only the keyboard row and reverts when nothing matches", async ({ page }) => {
  await page.goto("/")
  const html = page.locator("html")
  const picker = await openColorTheme(page)

  await picker.getByRole("option", { name: "Nord", exact: true }).hover()
  await expect(html).toHaveAttribute("data-theme", "dark")
  await expect(picker.getByRole("option", { name: "Dark", exact: true })).toHaveAttribute("aria-selected", "true")

  const input = picker.getByRole("combobox", { name: "Color theme" })
  await input.fill("rose")
  await expect(html).toHaveAttribute("data-theme", "rose-pine")
  await input.fill("nothing matches")
  await expect(picker.getByText("No matching themes")).toBeVisible()
  await expect(html).toHaveAttribute("data-theme", "dark")

  await input.fill("")
  await picker.getByRole("option", { name: "Nord", exact: true }).click()
  await expect(picker).toBeHidden()
  await expect(html).toHaveAttribute("data-theme", "nord")
  expect(await storedTheme(page)).toBe("nord")
})

test("renamed stored themes keep working", async ({ page }) => {
  await page.goto("/")
  await page.evaluate(() => window.localStorage.setItem("servef-theme", "gruvbox-dark"))
  await page.reload()
  await expect(page.locator("html")).toHaveAttribute("data-theme", "gruvbox")
  await page.getByRole("button", { name: "Settings" }).click()
  const theme = page.getByRole("dialog", { name: "Settings" }).getByRole("combobox", { name: "Appearance: Theme" })
  await expect(theme).toHaveValue("gruvbox")
})

test("the Color Theme pick returns focus where it was", async ({ page }) => {
  await page.goto("/")
  const settings = page.getByRole("button", { name: "Settings", exact: true })
  await settings.focus()
  const picker = await openColorTheme(page)
  await page.keyboard.press("Escape")
  await expect(picker).toBeHidden()
  await expect(settings).toBeFocused()

  await openColorTheme(page)
  await page.keyboard.press("Enter")
  await expect(picker).toBeHidden()
  await expect(settings).toBeFocused()
})
