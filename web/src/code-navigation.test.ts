import assert from "node:assert/strict"
import test from "node:test"

import { definitionOutcome, emptyMessage, groupLocations, identifierAt, navigationQuery, navigationTitle } from "./code-navigation.ts"
import type { NavigationLocation, NavigationResult } from "./navigation-contract.ts"

function location(path: string, line: number, column: number, kind = "function"): NavigationLocation {
  return { path, line, column, kind, snippet: "" }
}

function result(locations: NavigationLocation[], overrides: Partial<NavigationResult> = {}): NavigationResult {
  return { symbol: "Format", indexing: false, locations, ...overrides }
}

test("identifierAt finds the word under a column, including its trailing edge", () => {
  const text = "\treturn Format(\"hello\", name)"
  assert.deepEqual(identifierAt(text, 8), { start: 8, end: 14 })
  assert.deepEqual(identifierAt(text, 11), { start: 8, end: 14 })
  assert.deepEqual(identifierAt(text, 14), { start: 8, end: 14 })
  assert.equal(identifierAt(text, 0), null)
  assert.deepEqual(identifierAt(text, 7), { start: 1, end: 7 })
  assert.equal(identifierAt("a  b", 2), null)
  assert.deepEqual(identifierAt("this.#count += $el", 6), { start: 5, end: 11 })
  assert.deepEqual(identifierAt("this.#count += $el", 16), { start: 15, end: 18 })
  assert.deepEqual(identifierAt("const naïve = 1", 8), { start: 6, end: 11 })
  assert.equal(identifierAt("x = 42", 4), null)
})

test("one definition opens, several are chosen, none is reported", () => {
  const at = { line: 6, column: 8 }
  const word = { start: 8, end: 14 }
  const target = location("navigation/format.go", 6, 5)
  assert.deepEqual(definitionOutcome(result([target]), "navigation/greet.go", at, word), { kind: "open", location: target })
  const several = [target, location("other/format.go", 2, 5)]
  assert.deepEqual(definitionOutcome(result(several), "navigation/greet.go", at, word), { kind: "choose", locations: several })
  assert.deepEqual(definitionOutcome(result([]), "navigation/greet.go", at, word), { kind: "none" })
})

test("a definition at the clicked word shows its references, as VS Code does", () => {
  const self = location("navigation/format.go", 6, 5)
  assert.deepEqual(definitionOutcome(result([self]), "navigation/format.go", { line: 6, column: 5 }, { start: 5, end: 11 }), { kind: "references" })
  // Another word on the same line is not the definition itself.
  assert.deepEqual(definitionOutcome(result([self]), "navigation/format.go", { line: 6, column: 12 }, { start: 12, end: 17 }), { kind: "open", location: self })
})

test("groupLocations keeps server order within and across files", () => {
  const locations = [location("b.go", 1, 0), location("a.go", 3, 0), location("b.go", 9, 0)]
  assert.deepEqual(groupLocations(locations).map((group) => [group.path, group.locations.map((entry) => entry.line)]), [
    ["b.go", [1, 9]],
    ["a.go", [3]],
  ])
})

test("navigation queries and messages", () => {
  assert.equal(navigationQuery("/root", "a b.go", { line: 2, column: 4 }), "project=%2Froot&path=a+b.go&line=2&column=4")
  assert.equal(navigationTitle("references", "Format", 2), "2 references to Format")
  assert.equal(navigationTitle("definition", "Format", 1), "1 definition of Format")
  assert.equal(emptyMessage("definition", result([])), "No definition found for Format.")
  assert.equal(emptyMessage("references", result([], { indexing: true })), "Still indexing this project. Try Format again in a moment.")
  assert.match(emptyMessage("definition", result([], { symbol: "" })), /No symbol here/)
})
