import assert from "node:assert/strict"
import { readFile } from "node:fs/promises"
import test from "node:test"

import { diffGrammars } from "./diff-grammars.ts"
import { diffLanguage, mappedFamilies } from "./diff-language.ts"

async function registeredFamilies(): Promise<string[]> {
  const source = await readFile(new URL("../../testdata/document-families.json", import.meta.url), "utf8")
  const value: unknown = JSON.parse(source)
  assert.ok(Array.isArray(value))
  return value.filter((family: unknown): family is string => typeof family === "string")
}

test("diffs use the server's file family, never a guess", () => {
  assert.equal(diffLanguage("go"), "go")
  assert.equal(diffLanguage("typescript"), "typescript")
  assert.equal(diffLanguage("tsx"), "tsx")
  assert.equal(diffLanguage("jsx"), "jsx")
  assert.equal(diffLanguage("dockerfile"), "docker")
  assert.equal(diffLanguage("svg"), "xml")
  // Plain text, images, and unknown families render without highlighting.
  assert.equal(diffLanguage("text"), "text")
  assert.equal(diffLanguage("image"), "text")
  assert.equal(diffLanguage(""), "text")
  assert.equal(diffLanguage("constructor"), "text")
})

test("every registered family has an explicit diff grammar choice", async () => {
  const families = await registeredFamilies()
  assert.ok(families.length > 0)
  const unmapped = families.filter((family) => !mappedFamilies.includes(family))
  assert.deepEqual(unmapped, [], "map new families in diff-language.ts (decision 0024)")
  const stale = mappedFamilies.filter((family) => !families.includes(family))
  assert.deepEqual(stale, [], "remove families the registry no longer has")
})

test("only grammars a family uses are bundled", () => {
  const used = new Set<string>(mappedFamilies.map(diffLanguage))
  const unused = Object.keys(diffGrammars).filter((grammar) => !used.has(grammar))
  assert.deepEqual(unused, [], "drop grammars no family maps to (decision 0024)")
})
