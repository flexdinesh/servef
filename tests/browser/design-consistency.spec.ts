import { existsSync } from "node:fs"
import path from "node:path"

import { expect, test, type Locator, type Page } from "@playwright/test"

import { gitFixtureRoot } from "./git-fixture.ts"
import { gitApiOrigin } from "./ports.ts"

// Decision 0033: views share one design language through the UI primitives.
// These checks compare equivalent elements across views rather than pinning
// each view separately.

test.skip(() => !existsSync(path.join(gitFixtureRoot, ".git")), "git is not installed")

const project = encodeURIComponent(gitFixtureRoot)

function sourceControl(page: Page) {
  return page.getByRole("region", { name: "Source Control" })
}

async function height(locator: Locator): Promise<number> {
  const box = await locator.boundingBox()
  expect(box).not.toBeNull()
  return box?.height ?? 0
}

async function styles(locator: Locator, properties: readonly string[]): Promise<Record<string, string>> {
  return locator.evaluate((element, names) => {
    const style = getComputedStyle(element)
    return Object.fromEntries(names.map((name) => [name, style.getPropertyValue(name)]))
  }, properties)
}

test("list rows are 22px in Explorer, the compare list, Files, and Search", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  const explorer = page.getByRole("complementary", { name: "Files" })
  expect(await height(explorer.getByRole("treeitem", { name: "README.md" }))).toBe(22)

  await page.keyboard.press("ControlOrMeta+Shift+g")
  const options = sourceControl(page).getByRole("listbox", { name: "Compare" }).getByRole("option")
  await expect(options).toHaveCount(3)
  for (const option of await options.all()) expect(await height(option)).toBe(22)
  const files = sourceControl(page).getByRole("region", { name: /^Files: / })
  expect(await height(files.getByRole("treeitem", { name: "README.md" }))).toBe(22)

  await page.getByRole("button", { name: /^Find in files/ }).click()
  await page.getByRole("textbox", { name: "Search", exact: true }).fill("Fixture")
  const results = page.getByRole("tree", { name: "Search results" }).getByRole("treeitem")
  await expect(results.first()).toBeVisible()
  expect(await height(results.first())).toBe(22)
})

test("view header icon buttons share size, glyph, and rest color", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  // Views stay mounted but hidden; measure each while it shows.
  const measure = async (button: Locator) => {
    await page.mouse.move(0, 0)
    const box = await button.boundingBox()
    const glyph = await button.locator("svg").boundingBox()
    return { width: box?.width, height: box?.height, glyph: glyph?.width, color: (await styles(button, ["color"])).color }
  }
  const measured = [await measure(page.getByRole("complementary", { name: "Files" }).getByRole("button", { name: "Search files" }))]
  await page.keyboard.press("ControlOrMeta+Shift+g")
  measured.push(await measure(sourceControl(page).getByRole("button", { name: "Refresh" })))
  await page.getByRole("button", { name: /^Find in files/ }).click()
  measured.push(await measure(page.getByRole("region", { name: "Search" }).getByRole("button", { name: "Refresh" })))
  expect(measured[0]).toEqual({ width: 24, height: 24, glyph: 16, color: expect.any(String) })
  for (const entry of measured) expect(entry).toEqual(measured[0])
})

test("fields share height, border, focus ring, and invalid border", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  const filter = page.getByRole("textbox", { name: "Filter files by name" })
  const search = page.getByRole("textbox", { name: "Search", exact: true })
  const field = (input: Locator) => input.locator("xpath=..")
  const rest = ["height", "border-top-width", "border-top-color", "border-radius"]
  const ring = ["border-top-color", "box-shadow"]

  // Views stay mounted but hidden; read each field while it shows.
  const filterRest = await styles(field(filter), rest)
  await filter.focus()
  const filterFocused = await styles(field(filter), ring)
  await page.getByRole("button", { name: /^Find in files/ }).click()
  // Opening Search focuses its field; compare the rest state.
  await search.evaluate((element) => { if (element instanceof HTMLElement) element.blur() })
  expect(await styles(field(search), rest)).toEqual(filterRest)
  await search.focus()
  const focused = await styles(field(search), ring)
  expect(filterFocused).toEqual(focused)
  expect(focused["box-shadow"]).toMatch(/0px 0px 0px 1px$/)

  await page.keyboard.press("ControlOrMeta+Shift+e")

  await filter.fill("[")
  await expect(filter).toHaveAttribute("aria-invalid", "true")
  const invalid = await styles(field(filter), ["border-top-color", "border-bottom-color", "border-left-color", "border-right-color"])
  expect(new Set(Object.values(invalid)).size).toBe(1)
  expect(invalid["border-top-color"]).not.toBe(focused["border-top-color"])
})

test("overlays share radius, border, and shadow", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/view?project=${project}&path=README.md`)
  const surface = ["border-radius", "border-top-color", "border-top-width", "box-shadow", "background-color"]

  await page.keyboard.press("F1")
  const palette = await styles(page.getByRole("dialog").first(), surface)
  await page.keyboard.press("Escape")

  await page.getByRole("button", { name: "Settings", exact: true }).click()
  const settings = await styles(page.getByRole("dialog", { name: "Settings", exact: true }), surface)
  await page.keyboard.press("Escape")

  await page.getByRole("tab", { name: "README.md" }).click({ button: "right" })
  const menu = await styles(page.getByRole("menu"), surface)

  expect(palette["border-radius"]).toBe("6px")
  expect(settings).toEqual(palette)
  expect(menu).toEqual(palette)
})

test("standalone actions are 28px on desktop and 44px on touch layouts", async ({ page }) => {
  await page.goto(`${gitApiOrigin}/?project=${project}`)
  await page.getByRole("textbox", { name: "Filter files by name" }).fill("no-such-file")
  // The text action, not the field's icon-only clear button of the same name.
  const clear = page.getByRole("complementary", { name: "Files" }).getByRole("button", { name: "Clear filter" }).filter({ hasText: "Clear filter" })
  expect(await height(clear)).toBe(28)

  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "Open files" }).click()
  const drawerClear = page.getByRole("dialog", { name: "Files", exact: true }).getByRole("button", { name: "Clear filter" }).filter({ hasText: "Clear filter" })
  expect(await height(drawerClear)).toBe(44)
})
