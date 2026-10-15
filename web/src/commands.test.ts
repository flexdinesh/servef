import assert from "node:assert/strict"
import test from "node:test"

import { commands, matchCommands, type Command } from "./commands.ts"

const sample: readonly Command[] = [
  { id: "a", category: "Settings", title: "Theme", action: { kind: "open-setting", setting: "theme" } },
  { id: "b", category: "Settings", title: "Font Size", action: { kind: "open-setting", setting: "fontSize" } },
]

test("Open Settings comes first, then every setting opens Settings at it", () => {
  assert.deepEqual(commands.map((command) => [command.category, command.title, command.action]), [
    ["Preferences", "Open Settings", { kind: "open-settings" }],
    ["Settings", "Theme", { kind: "open-setting", setting: "theme" }],
    ["Settings", "Font Size", { kind: "open-setting", setting: "fontSize" }],
  ])
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
