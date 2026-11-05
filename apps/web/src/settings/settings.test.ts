import assert from "node:assert/strict"
import test from "node:test"

import {
  availableSettings,
  DEFAULT_FONT_SIZE,
  defaultSettings,
  FONT_SIZE_MAX,
  FONT_SIZE_MIN,
  matchSettings as matchAmong,
  modifiedSettings,
  parseFontSize,
  parseSettings,
  settingDefinitions,
  uiScale,
} from "./settings.ts"

const matchSettings = (query: string) => matchAmong(query, availableSettings(false))

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
  const defaults = { fontSize: DEFAULT_FONT_SIZE, languageServers: "on", showWelcome: "on", notifyCommits: "on", notifyBranches: "on", notifyEdits: "off", notifyWalkthroughs: "on" }
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
  assert.deepEqual(parseSettings('{"notifyEdits":"on","notifyCommits":"off"}'), { ...defaults, notifyEdits: "on", notifyCommits: "off" })
  assert.deepEqual(parseSettings('{"notifyBranches":"off","notifyWalkthroughs":"off"}'), { ...defaults, notifyBranches: "off", notifyWalkthroughs: "off" })
  assert.deepEqual(parseSettings('{"notifyEdits":true,"notifyCommits":"maybe"}'), defaults)
})

test("only values differing from defaults count as modified", () => {
  const defaults = defaultSettings
  assert.deepEqual(modifiedSettings(defaults), [])
  assert.deepEqual(modifiedSettings({ ...defaults, notifyEdits: "on", notifyCommits: "off" }), ["notifyCommits", "notifyEdits"])
  assert.deepEqual(modifiedSettings({ ...defaults, fontSize: 15 }), ["fontSize"])
  assert.deepEqual(modifiedSettings({ ...defaults, languageServers: "off" }), ["languageServers"])
  assert.deepEqual(modifiedSettings({ ...defaults, showWelcome: "off" }), ["showWelcome"])
})

test("ui scale is relative to the default font size", () => {
  assert.equal(uiScale(defaultSettings), 1)
  assert.equal(uiScale({ ...defaultSettings, fontSize: 26 }), 2)
})

test("notification settings exist only where notifications do", () => {
  const notify = ["notifyCommits", "notifyBranches", "notifyEdits", "notifyWalkthroughs"]
  assert.deepEqual(availableSettings(true), settingDefinitions)
  assert.deepEqual(availableSettings(true).map((definition) => definition.id).slice(-4), notify)
  assert.ok(availableSettings(false).every((definition) => !notify.includes(definition.id)))
  assert.deepEqual(matchAmong("commit", availableSettings(true)).map((definition) => definition.id), ["notifyCommits"])
  assert.equal(matchAmong("commit", availableSettings(false)).length, 0)
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
