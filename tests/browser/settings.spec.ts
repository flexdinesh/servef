import { expect, test } from "@playwright/test"

test("settings categories, global search, and modified filter compose and recover", async ({ page }) => {
  await page.goto("/")
  await page.getByRole("button", { name: "Settings", exact: true }).click()
  const dialog = page.getByRole("dialog", { name: "Settings", exact: true })
  const categories = dialog.getByRole("navigation", { name: "Settings categories" })
  const search = dialog.getByRole("searchbox", { name: "Search settings" })
  const modified = dialog.getByRole("button", { name: "Show modified settings" })
  await expect(search).toBeFocused()
  await expect(dialog.getByRole("status")).toHaveText("3 settings")
  await categories.getByRole("button", { name: "Code Navigation" }).click()
  await expect(dialog.getByRole("combobox", { name: "Code Navigation: Language Servers" })).toBeVisible()
  await expect(dialog.getByRole("spinbutton")).toHaveCount(0)
  await search.fill("nord")
  await expect(dialog.getByRole("status")).toHaveText("1 setting")
  const theme = dialog.getByRole("combobox", { name: "Appearance: Theme" })
  await theme.selectOption("nord")
  await modified.click()
  await expect(modified).toHaveAttribute("aria-pressed", "true")
  await expect(theme).toHaveValue("nord")
  await dialog.getByRole("button", { name: "Reset Theme", exact: true }).click()
  await expect(modified).toBeFocused()
  await expect(dialog.getByRole("status")).toHaveText("0 settings")
  await dialog.getByRole("button", { name: "Clear filters" }).click()
  await expect(search).toBeFocused()
  await expect(dialog.getByRole("status")).toHaveText("3 settings")
  await search.fill("nothing matches")
  await dialog.getByRole("button", { name: "Clear settings search" }).click()
  await expect(search).toHaveValue("")
  await expect(search).toBeFocused()
  await search.fill("font")
  await dialog.getByRole("spinbutton").focus()
  await page.keyboard.press("Escape")
  await expect(dialog).toBeVisible()
  await expect(search).toHaveValue("")
  await expect(search).toBeFocused()
  await page.keyboard.press("Escape")
  await expect(dialog).toBeHidden()
  await expect(page.getByRole("button", { name: "Settings", exact: true })).toBeFocused()
})

test("settings footer matches the header and categories align with search", async ({ page }) => {
  await page.goto("/")
  await page.getByRole("button", { name: "Settings", exact: true }).click()
  const dialog = page.getByRole("dialog", { name: "Settings", exact: true })
  const box = async (selector: string) => {
    const bounds = await dialog.locator(selector).first().boundingBox()
    expect(bounds).not.toBeNull()
    return bounds ?? { x: 0, y: 0, width: 0, height: 0 }
  }
  expect((await box(".settings-footer")).height).toBeCloseTo((await box(".settings-header")).height, 0)
  const search = await box(".settings-search")
  const rail = await box(".settings-categories")
  const selected = await box(".settings-categories [aria-current]")
  // The selection starts at the search field's edge and fills to the rail's border.
  expect(selected.x).toBeCloseTo(search.x, 0)
  expect(selected.x + selected.width).toBeCloseTo(rail.x + rail.width - 1, 0)
  // The select's chevron ink is inset from the right as far as its text is from the left.
  const inset = await dialog.getByRole("combobox", { name: "Appearance: Theme" }).evaluate((select) => {
    const icon = select.parentElement?.querySelector("svg")
    const path = icon?.querySelector("path")
    if (!icon || !(path instanceof SVGGraphicsElement)) return null
    const box = icon.getBoundingClientRect()
    const scale = box.width / icon.viewBox.baseVal.width
    const ink = path.getBBox()
    const inkRight = box.left + (ink.x + ink.width + parseFloat(getComputedStyle(path).strokeWidth) / 2) * scale
    const style = getComputedStyle(select)
    return {
      left: parseFloat(style.borderLeftWidth) + parseFloat(style.paddingLeft),
      right: select.getBoundingClientRect().right - inkRight,
    }
  })
  expect(inset).not.toBeNull()
  if (inset) expect(Math.abs(inset.right - inset.left)).toBeLessThan(0.5)
})

test("settings keeps controls and footer reachable on short desktop and mobile", async ({ page }) => {
  for (const viewport of [{ width: 1280, height: 480 }, { width: 390, height: 667 }]) {
    await page.setViewportSize(viewport)
    await page.goto("/")
    await page.getByRole("button", { name: "Settings", exact: true }).click()
    const dialog = page.getByRole("dialog", { name: "Settings", exact: true })
    const font = dialog.getByRole("spinbutton", { name: "Appearance: Font Size" })
    await font.fill("20")
    const bounds = await dialog.boundingBox()
    expect(bounds).not.toBeNull()
    if (bounds) {
      expect(bounds.y).toBeGreaterThanOrEqual(0)
      expect(bounds.y + bounds.height).toBeLessThanOrEqual(viewport.height)
    }
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true)
    const categories = dialog.getByRole("navigation", { name: "Settings categories" })
    await categories.getByRole("button", { name: "Code Navigation" }).click()
    const servers = dialog.getByRole("combobox", { name: "Code Navigation: Language Servers" })
    await servers.scrollIntoViewIfNeeded()
    await expect(servers).toBeInViewport()
    const reset = dialog.getByRole("button", { name: "Reset all", exact: true })
    await expect(reset).toBeInViewport()
    await reset.click()
    await expect(dialog.getByRole("searchbox")).toBeFocused()
    await dialog.getByRole("button", { name: "Close settings" }).click()
  }
})
