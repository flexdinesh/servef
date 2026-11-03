import assert from "node:assert/strict"
import test from "node:test"

import { commands, matchCommands, type Command } from "./commands.ts"

const sample: readonly Command[] = [
  { id: "a", category: "Settings", title: "Theme", action: { kind: "open-setting", setting: "theme" } },
  { id: "b", category: "Settings", title: "Font Size", action: { kind: "open-setting", setting: "fontSize" } },
]

test("Open Settings and Color Theme come first, then every setting opens Settings at it, then navigation and walkthroughs", () => {
  assert.deepEqual(commands.map((command) => [command.category, command.title, command.action]), [
    ["Preferences", "Open Settings", { kind: "open-settings" }],
    ["Preferences", "Color Theme", { kind: "select-color-theme" }],
    ["Settings", "Theme", { kind: "open-setting", setting: "theme" }],
    ["Settings", "Font Size", { kind: "open-setting", setting: "fontSize" }],
    ["Settings", "Language Servers", { kind: "open-setting", setting: "languageServers" }],
    ["Settings", "Show Welcome on Startup", { kind: "open-setting", setting: "showWelcome" }],
    ["Navigation", "Go to Definition", { kind: "go-to-definition" }],
    ["Navigation", "Find All References", { kind: "find-references" }],
    ["Walkthrough", "Next Step", { kind: "walkthrough-step", direction: 1 }],
    ["Walkthrough", "Previous Step", { kind: "walkthrough-step", direction: -1 }],
    ["Walkthrough", "Present", { kind: "walkthrough-present" }],
    ["Walkthrough", "Close Walkthrough", { kind: "walkthrough-exit" }],
    ["Help", "Welcome", { kind: "open-welcome" }],
  ])
  assert.deepEqual(commands.slice(6).map((command) => command.shortcut), ["F12", "Shift+F12", "Ctrl+Right", "Ctrl+Left", undefined, undefined, undefined])
})

test("typing welcome finds the Welcome tab and its startup setting", () => {
  assert.deepEqual(matchCommands(commands, "welcome").map((match) => match.command.id), ["settings.showWelcome", "help.welcome"])
})

test("an empty query lists every command in order", () => {
  assert.deepEqual(matchCommands(sample, " ").map((match) => match.command.id), ["a", "b"])
})

test("every term must match the category or title, ignoring case", () => {
  assert.deepEqual(matchCommands(sample, "size").map((match) => match.command.id), ["b"])
  assert.deepEqual(matchCommands(sample, "SETTINGS font").map((match) => match.command.id), ["b"])
  assert.deepEqual(matchCommands(sample, "settings").map((match) => match.command.id), ["a", "b"])
  assert.deepEqual(matchCommands(sample, "font color"), [])
})

test("matches report highlight ranges in title and category", () => {
  const [match] = matchCommands(sample, "set size font")
  assert.deepEqual(match?.category, [{ start: 0, end: 3 }])
  assert.deepEqual(match?.title, [{ start: 0, end: 4 }, { start: 5, end: 9 }])
})

test("title word-prefix matches rank first", () => {
  assert.deepEqual(matchCommands(sample, "t").map((match) => match.command.id), ["a", "b"])
  assert.deepEqual(matchCommands(sample, "s").map((match) => match.command.id), ["b", "a"])
})

test("typing theme finds the Color Theme pick and the Theme setting", () => {
  assert.deepEqual(matchCommands(commands, "theme").map((match) => match.command.id), ["preferences.colorTheme", "settings.theme"])
})
