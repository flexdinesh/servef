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
  type Settings,
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
  const defaults = { fontSize: DEFAULT_FONT_SIZE, languageServers: "on", showWelcome: "on" }
  assert.deepEqual(parseSettings(null), defaults)
  assert.deepEqual(parseSettings("not json"), defaults)
  assert.deepEqual(parseSettings("[]"), defaults)
  assert.deepEqual(parseSettings('{"fontSize":15}'), { ...defaults, fontSize: 15 })
  assert.deepEqual(parseSettings('{"fontSize":99}'), defaults)
  assert.deepEqual(parseSettings('{"fontSize":"15"}'), { ...defaults, fontSize: 15 })
  assert.deepEqual(parseSettings('{"languageServers":"off"}'), { ...defaults, languageServers: "off" })
  assert.deepEqual(parseSettings('{"languageServers":true}'), defaults)
  assert.deepEqual(parseSettings('{"showWelcome":"off"}'), { ...defaults, showWelcome: "off" })
  assert.deepEqual(parseSettings('{"showWelcome":false}'), defaults)
})

test("only values differing from defaults count as modified", () => {
  const defaults: Settings = { fontSize: DEFAULT_FONT_SIZE, languageServers: "on", showWelcome: "on" }
  assert.deepEqual(modifiedSettings(defaults), [])
  assert.deepEqual(modifiedSettings({ ...defaults, fontSize: 15 }), ["fontSize"])
  assert.deepEqual(modifiedSettings({ ...defaults, languageServers: "off" }), ["languageServers"])
  assert.deepEqual(modifiedSettings({ ...defaults, showWelcome: "off" }), ["showWelcome"])
})

test("ui scale is relative to the default font size", () => {
  assert.equal(uiScale({ fontSize: DEFAULT_FONT_SIZE, languageServers: "off", showWelcome: "on" }), 1)
  assert.equal(uiScale({ fontSize: 26, languageServers: "off", showWelcome: "on" }), 2)
})

test("setting search matches every term across category, title, and description", () => {
  assert.deepEqual(matchSettings("").map((definition) => definition.id), ["theme", "fontSize", "languageServers", "showWelcome"])
  assert.deepEqual(matchSettings("welcome startup").map((definition) => definition.id), ["showWelcome"])
  assert.deepEqual(matchSettings("gopls").map((definition) => definition.id), ["languageServers"])
  assert.deepEqual(matchSettings("font").map((definition) => definition.id), ["fontSize"])
  assert.deepEqual(matchSettings("APPEARANCE size").map((definition) => definition.id), ["fontSize"])
  assert.deepEqual(matchSettings("dark").map((definition) => definition.id), ["theme"])
  assert.deepEqual(matchSettings("nord").map((definition) => definition.id), ["theme"])
  assert.equal(matchSettings("font color").length, 0)
})
