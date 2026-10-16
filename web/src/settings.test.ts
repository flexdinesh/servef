import assert from "node:assert/strict"
import test from "node:test"

import {
  DEFAULT_FONT_SIZE,
  FONT_SIZE_MAX,
  FONT_SIZE_MIN,
  matchSettings,
  modifiedSettings,
  parseFontSize,
  parseSettings,
  uiScale,
} from "./settings.ts"

test("font size accepts whole pixel values within range", () => {
  assert.equal(parseFontSize(14), 14)
  assert.equal(parseFontSize("12"), 12)
  assert.equal(parseFontSize(" 16 "), 16)
  assert.equal(parseFontSize(FONT_SIZE_MIN), FONT_SIZE_MIN)
  assert.equal(parseFontSize(FONT_SIZE_MAX), FONT_SIZE_MAX)
  assert.equal(parseFontSize(FONT_SIZE_MIN - 1), null)
  assert.equal(parseFontSize(FONT_SIZE_MAX + 1), null)
  assert.equal(parseFontSize(13.5), null)
  assert.equal(parseFontSize(""), null)
  assert.equal(parseFontSize("abc"), null)
  assert.equal(parseFontSize(null), null)
})

test("stored settings keep valid values and default the rest", () => {
  const off = { fontSize: DEFAULT_FONT_SIZE, languageServers: "off" }
  assert.deepEqual(parseSettings(null), off)
  assert.deepEqual(parseSettings("not json"), off)
  assert.deepEqual(parseSettings("[]"), off)
  assert.deepEqual(parseSettings('{"fontSize":15}'), { ...off, fontSize: 15 })
  assert.deepEqual(parseSettings('{"fontSize":99}'), off)
  assert.deepEqual(parseSettings('{"fontSize":"15"}'), { ...off, fontSize: 15 })
  assert.deepEqual(parseSettings('{"languageServers":"on"}'), { ...off, languageServers: "on" })
  assert.deepEqual(parseSettings('{"languageServers":true}'), off)
})

test("only values differing from defaults count as modified", () => {
  assert.deepEqual(modifiedSettings({ fontSize: DEFAULT_FONT_SIZE, languageServers: "off" }), [])
  assert.deepEqual(modifiedSettings({ fontSize: 15, languageServers: "off" }), ["fontSize"])
  assert.deepEqual(modifiedSettings({ fontSize: DEFAULT_FONT_SIZE, languageServers: "on" }), ["languageServers"])
})

test("ui scale is relative to the default font size", () => {
  assert.equal(uiScale({ fontSize: DEFAULT_FONT_SIZE, languageServers: "off" }), 1)
  assert.equal(uiScale({ fontSize: 26, languageServers: "off" }), 2)
})

test("setting search matches every term across category, title, and description", () => {
  assert.deepEqual(matchSettings("").map((definition) => definition.id), ["theme", "fontSize", "languageServers"])
  assert.deepEqual(matchSettings("gopls").map((definition) => definition.id), ["languageServers"])
  assert.deepEqual(matchSettings("font").map((definition) => definition.id), ["fontSize"])
  assert.deepEqual(matchSettings("APPEARANCE size").map((definition) => definition.id), ["fontSize"])
  assert.deepEqual(matchSettings("dark").map((definition) => definition.id), ["theme"])
  assert.deepEqual(matchSettings("nord").map((definition) => definition.id), ["theme"])
  assert.equal(matchSettings("font color").length, 0)
})
