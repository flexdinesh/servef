import assert from "node:assert/strict"
import test from "node:test"

import { themeOptions } from "../settings/theme.ts"
import { initialActiveIndex, matchThemes, pickOrder } from "./color-theme-pick.ts"

function labels(query: string) {
  return matchThemes(themeOptions, query).map((group) => [group.title, group.matches.map((match) => match.option.label)])
}

test("an empty query lists System, then light themes, then dark themes", () => {
  const groups = matchThemes(themeOptions, "")
  assert.deepEqual(groups.map((group) => group.title), [null, "light themes", "dark themes"])
  assert.deepEqual(groups[0]?.matches.map((match) => match.option.value), ["system"])
  assert.deepEqual(groups[1]?.matches.map((match) => match.option.label), [
    "Light", "Catppuccin Latte", "Flexoki Light", "Lupine", "Rose Pine", "Solarized Light", "White",
  ])
  assert.equal(groups[2]?.matches[0]?.option.label, "Dark")
  assert.equal(pickOrder(groups).length, themeOptions.length)
})

test("every term must match the label, ignoring case; empty groups drop", () => {
  assert.deepEqual(labels("NIGHT"), [["dark themes", ["Tokyo Night"]]])
  assert.deepEqual(labels("cat latte"), [["light themes", ["Catppuccin Latte"]]])
  assert.deepEqual(labels("zzz"), [])
})

test("matches report merged label highlight ranges", () => {
  const [group] = matchThemes(themeOptions, "tok yo")
  assert.deepEqual(group?.matches[0]?.label, [{ start: 0, end: 5 }])
})

test("the current theme starts active, else the first row", () => {
  const order = pickOrder(matchThemes(themeOptions, ""))
  assert.equal(order[initialActiveIndex(order, "nord")]?.option.value, "nord")
  const filtered = pickOrder(matchThemes(themeOptions, "rose"))
  assert.equal(initialActiveIndex(filtered, "nord"), 0)
})
