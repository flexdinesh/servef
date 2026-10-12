import assert from "node:assert/strict"
import { readFile } from "node:fs/promises"
import test from "node:test"

import { parseFeatureValue } from "./features.ts"
import { isFormatResult, isPageData, type PageData } from "./page-contract.ts"
import { isProcessMetrics } from "./process-metrics.ts"
import { isSearchDocumentsResponse } from "./search-documents.ts"

async function fixture(name: string): Promise<unknown> {
  const source = await readFile(
    new URL(`../../testdata/api/default/${name}`, import.meta.url),
    "utf8",
  )
  const value: unknown = JSON.parse(source)
  return value
}

test("frontend API fixtures satisfy runtime contracts", async () => {
  assert.notEqual(parseFeatureValue(await fixture("features.json")), null)
  assert.equal(isProcessMetrics(await fixture("metrics.json")), true)
  assert.equal(isProcessMetrics(await fixture("metrics-go.json")), true)
  assert.equal(isSearchDocumentsResponse(await fixture("search-documents.json")), true)
  for (const name of [
    "page-index.json", "page-document.json", "page-missing.json", "page-json.json",
    "page-json-broken.json", "page-image.json", "page-binary.json", "page-large.json", "page-too-large.json",
  ]) {
    assert.equal(isPageData(await fixture(name)), true, name)
  }
  assert.equal(isFormatResult(await fixture("format-settings.json")), true)
  assert.equal(isFormatResult(await fixture("format-error.json")), false)
})

async function page(name: string): Promise<PageData> {
  const value = await fixture(name)
  assert.ok(isPageData(value), name)
  return value
}

test("page fixtures follow document-shape invariants", async () => {
  const json = await page("page-json.json")
  assert.equal(json.documentType?.formattable, true)
  assert.equal(json.content, "")

  for (const name of ["page-image.json", "page-binary.json"]) {
    const binary = await page(name)
    assert.equal(binary.binary, true, name)
    assert.equal(binary.source, "", name)
    assert.deepEqual(binary.sourceLines, [], name)
    assert.equal(binary.content, "", name)
  }
  assert.equal((await page("page-image.json")).documentType?.previewKind, "image")
  assert.equal((await page("page-binary.json")).documentType?.label, "Plain text")

  const large = await page("page-large.json")
  assert.equal(large.previewDisabled, true)
  assert.equal(large.searchExcluded, true)
  assert.ok(large.fileSize > 8 * 1_048_576)
  assert.equal(large.tooLarge, false)

  const tooLarge = await page("page-too-large.json")
  assert.equal(tooLarge.tooLarge, true)
  assert.equal(tooLarge.binary, false)
  assert.equal(tooLarge.previewDisabled, true)
  assert.equal(tooLarge.searchExcluded, true)
  assert.equal(tooLarge.source, "")
  assert.deepEqual(tooLarge.sourceLines, [])
  assert.equal(tooLarge.content, "")
  assert.ok(tooLarge.fileSize > 32 * 1_048_576)
})

test("contract validators reject removed and malformed fields", async () => {
  const value = await fixture("page-document.json")
  assert.ok(isPageData(value))
  const { filenamePatterns: _patterns, ...withoutPatterns } = value
  assert.equal(isPageData(withoutPatterns), false)
  assert.equal(isPageData({ ...value, binary: "no" }), false)
  const { tooLarge: _tooLarge, ...withoutTooLarge } = value
  assert.equal(isPageData(withoutTooLarge), false)
  assert.equal(isPageData({ ...value, documentType: { ...value.documentType, previewKind: "code" } }), false)
  assert.equal(isPageData({ ...value, documentType: { ...value.documentType, filenames: null } }), false)
  assert.equal(isPageData({ ...value, documentType: { ...value.documentType, formattable: undefined } }), false)
  assert.equal(isPageData({ ...value, documentType: null }), false)
  assert.equal(isPageData({ ...value, documentType: { ...value.documentType, previewLabel: "" } }), false)
  assert.equal(isFormatResult({ formattedLines: ["ok", 1] }), false)
})
