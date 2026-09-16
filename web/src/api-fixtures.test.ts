import assert from "node:assert/strict"
import { readFile } from "node:fs/promises"
import test from "node:test"

import { parseFeatureValue } from "./features.ts"
import { isPageData } from "./page-contract.ts"
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
  for (const name of ["page-index.json", "page-document.json", "page-missing.json"]) {
    assert.equal(isPageData(await fixture(name)), true, name)
  }
})
