import assert from "node:assert/strict"
import test from "node:test"

import { isTheme, parseTheme, resolveTheme, themeOptions } from "./theme.ts"

test("theme validation accepts only configured theme values", () => {
  assert.equal(isTheme("light"), true)
  assert.equal(isTheme("dark"), true)
  assert.equal(isTheme("system"), true)
  assert.equal(isTheme("dracula"), true)
  assert.equal(isTheme("gruvbox"), true)
  assert.equal(isTheme("tokyo-night"), true)
  assert.equal(isTheme("solarized-light"), true)
  assert.equal(isTheme("gruvbox-dark"), false)
  assert.equal(isTheme("auto"), false)
  assert.equal(isTheme("toString"), false)
  assert.equal(isTheme(null), false)
})

test("stored theme parsing defaults invalid values to dark", () => {
  assert.equal(parseTheme("light"), "light")
  assert.equal(parseTheme("system"), "system")
  assert.equal(parseTheme(""), "dark")
  assert.equal(parseTheme(null), "dark")
})

test("stored theme parsing migrates renamed themes", () => {
  assert.equal(parseTheme("catppuccin-mocha"), "catppuccin")
  assert.equal(parseTheme("gruvbox-dark"), "gruvbox")
  assert.equal(parseTheme("nord"), "nord")
})

test("theme resolution follows the system only when configured", () => {
  assert.equal(resolveTheme("system", false), "light")
  assert.equal(resolveTheme("system", true), "dark")
  assert.equal(resolveTheme("light", true), "light")
  assert.equal(resolveTheme("dark", false), "dark")
  assert.equal(resolveTheme("catppuccin", false), "dark")
  assert.equal(resolveTheme("dracula", false), "dark")
  assert.equal(resolveTheme("vantablack", false), "dark")
  assert.equal(resolveTheme("solarized-light", true), "light")
})

test("light themes resolve light", () => {
  const light = themeOptions.filter((option) => option.scheme === "light").map((option) => option.value)
  assert.deepEqual(light, ["light", "catppuccin-latte", "flexoki-light", "lupine", "rose-pine", "solarized-light", "white"])
  for (const theme of light) assert.equal(resolveTheme(theme, true), "light")
})

test("theme options list System, Light, and Dark, then named themes by label", () => {
  const labels = themeOptions.map((option) => option.label)
  assert.deepEqual(labels.slice(0, 3), ["System", "Light", "Dark"])
  const named = labels.slice(3)
  assert.deepEqual(named, named.toSorted((a, b) => a.localeCompare(b)))
  assert.equal(themeOptions.length, 28)
})
