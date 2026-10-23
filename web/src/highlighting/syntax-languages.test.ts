import assert from "node:assert/strict"
import { readFile } from "node:fs/promises"
import test from "node:test"

import { isSupportedLanguage } from "@pierre/highlights"

interface Family {
  id: string
  language: string
}

function isFamily(value: unknown): value is Family {
  return typeof value === "object" && value !== null
    && "id" in value && typeof value.id === "string"
    && "language" in value && typeof value.language === "string"
}

// testdata/document-families.json mirrors internal/document.Builtins(); Go
// tests keep it equal to the registry.
async function registeredFamilies(): Promise<Family[]> {
  const source = await readFile(new URL("../../../testdata/document-families.json", import.meta.url), "utf8")
  const value: unknown = JSON.parse(source)
  assert.ok(Array.isArray(value))
  assert.ok(value.every(isFamily), "families are {id, language} objects")
  return value.filter(isFamily)
}

test("every registered family declares a Highlights language", async () => {
  const families = await registeredFamilies()
  assert.ok(families.length > 0)
  // Raster images have no text source and declare no language.
  const unsupported = families.filter((family) => family.id !== "image" && !isSupportedLanguage(family.language))
  assert.deepEqual(unsupported, [], "use a Highlights language or `text` in Builtins() (decision 0029)")
  assert.equal(families.find((family) => family.id === "image")?.language, "")
})
