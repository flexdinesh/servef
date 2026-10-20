import assert from "node:assert/strict"
import test from "node:test"

import { isTheme, parseTheme, resolveTheme } from "./theme.ts"

test("theme validation accepts only configured theme values", () => {
  assert.equal(isTheme("light"), true)
  assert.equal(isTheme("dark"), true)
  assert.equal(isTheme("system"), true)
  assert.equal(isTheme("dracula"), true)
  assert.equal(isTheme("gruvbox-dark"), true)
  assert.equal(isTheme("solarized-light"), true)
  assert.equal(isTheme("auto"), false)
  assert.equal(isTheme(null), false)
})

test("stored theme parsing defaults invalid values to system", () => {
  assert.equal(parseTheme("dark"), "dark")
  assert.equal(parseTheme(""), "system")
  assert.equal(parseTheme(null), "system")
})

test("theme resolution follows the system only when configured", () => {
  assert.equal(resolveTheme("system", false), "light")
  assert.equal(resolveTheme("system", true), "dark")
  assert.equal(resolveTheme("light", true), "light")
  assert.equal(resolveTheme("dark", false), "dark")
  assert.equal(resolveTheme("catppuccin-mocha", false), "dark")
  assert.equal(resolveTheme("dracula", false), "dark")
  assert.equal(resolveTheme("solarized-light", true), "light")
})
