import assert from "node:assert/strict"
import test from "node:test"

import { bestSnippet, createSearchEngine } from "./search-engine.ts"

test("content search matches document text, independently of file names", () => {
  const engine = createSearchEngine()
  engine.initialize([
    { path: "docs/guide.md", name: "guide.md", content: "The guide explains alpha and beta." },
    { path: "notes/alpha.md", name: "alpha.md", content: "alpha only" },
    { path: "reference/configuration.md", name: "configuration.md", content: "Settings" },
  ])

  assert.deepEqual(engine.search("guide").contentResults.map((result) => result.path), ["docs/guide.md"])
  assert.deepEqual(engine.search("alpha beta").contentResults.map((result) => result.path), ["docs/guide.md"])
  assert.deepEqual(engine.search("configuration").contentResults, [])
  assert.deepEqual(engine.search("guide").pathResults, [])
})

test("content search supports prefixes without fuzzy matches", () => {
  const engine = createSearchEngine()
  engine.initialize([{ path: "reference/options.md", name: "options.md", content: "Configuration settings" }])

  assert.equal(engine.search("config").contentResults[0]?.path, "reference/options.md")
  assert.deepEqual(engine.search("configuraton").contentResults, [])
})

test("content results are capped at ten", () => {
  const engine = createSearchEngine()
  engine.initialize(Array.from({ length: 15 }, (_, index) => ({
    path: `guide-${index}.md`,
    name: `guide-${index}.md`,
    content: `guide content ${index}`,
  })))
  const results = engine.search("guide")
  assert.equal(results.pathResults.length, 0)
  assert.equal(results.contentResults.length, 10)
})

test("the best content snippet includes safe highlight ranges", () => {
  const snippet = bestSnippet("A long introduction. Alpha appears near beta in this useful section.", "alpha beta")
  assert.match(snippet.text, /Alpha/)
  assert.match(snippet.text, /beta/)
  assert.equal(snippet.highlights.length, 2)
  assert.deepEqual(snippet.highlights.map((range) => snippet.text.slice(range.start, range.end).toLowerCase()), ["alpha", "beta"])
})
