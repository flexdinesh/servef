import assert from "node:assert/strict"
import test from "node:test"

import { createSearchEngine, linePreview, searchDocuments, type SearchDocument, type SearchOutcome, type SearchQuery } from "./search-engine.ts"

const documents: SearchDocument[] = [
  { path: "docs/guide.md", name: "guide.md", content: "# Guide\n\nThe guide explains Alpha.\r\n  alpha and alphabet\n" },
  { path: "src/main.go", name: "main.go", content: "package main\n\nfunc main() { alpha() }\n" },
  { path: "notes/empty.md", name: "empty.md", content: "" },
]

function query(pattern: string, options: Partial<SearchQuery> = {}): SearchQuery {
  return { pattern, include: "", exclude: "", matchCase: false, wholeWord: false, regex: false, ...options }
}

function results(outcome: SearchOutcome) {
  assert.equal(outcome.kind, "results")
  if (outcome.kind !== "results") throw new Error("expected results")
  return outcome
}

function locations(outcome: SearchOutcome): string[] {
  return results(outcome).files.flatMap((file) => file.matches.map((match) => `${file.path}:${match.line}:${match.column}-${match.endColumn}`))
}

test("literal search finds every case-insensitive occurrence by line and UTF-16 column", () => {
  assert.deepEqual(locations(searchDocuments(documents, query("alpha"))), [
    "docs/guide.md:3:19-24",
    "docs/guide.md:4:2-7",
    "docs/guide.md:4:12-17",
    "src/main.go:3:14-19",
  ])
  const outcome = results(searchDocuments(documents, query("alpha")))
  assert.equal(outcome.matchCount, 4)
  assert.equal(outcome.truncated, false)
})

test("match case, whole word, and regular expression options narrow matches", () => {
  assert.deepEqual(locations(searchDocuments(documents, query("Alpha", { matchCase: true }))), ["docs/guide.md:3:19-24"])
  assert.deepEqual(locations(searchDocuments(documents, query("alpha", { wholeWord: true }))).length, 3)
  assert.deepEqual(locations(searchDocuments(documents, query("alpha\\w+", { regex: true }))), ["docs/guide.md:4:12-20"])
  assert.deepEqual(locations(searchDocuments(documents, query("^func", { regex: true }))), ["src/main.go:3:0-4"])
  // Literal mode escapes regular-expression syntax.
  assert.deepEqual(locations(searchDocuments(documents, query("main()"))), ["src/main.go:3:5-11"])
})

test("regular expressions never span lines and skip empty matches", () => {
  assert.deepEqual(locations(searchDocuments(documents, query("Guide\\s+The", { regex: true }))), [])
  assert.deepEqual(results(searchDocuments(documents, query("q*", { regex: true }))).files, [])
})

test("invalid regular expressions and scopes are reported by field", () => {
  assert.deepEqual(searchDocuments(documents, query("(", { regex: true })), { kind: "invalid", field: "pattern", message: "Invalid regular expression." })
  assert.equal(searchDocuments(documents, query("alpha", { include: "[" })).kind, "invalid")
  const exclude = searchDocuments(documents, query("alpha", { exclude: "{a" }))
  assert.equal(exclude.kind === "invalid" && exclude.field, "exclude")
  // Escapes unicode mode rejects still compile.
  assert.equal(searchDocuments(documents, query("\\-", { regex: true })).kind, "results")
})

test("include and exclude globs scope documents by path", () => {
  assert.deepEqual(results(searchDocuments(documents, query("alpha", { include: "*.go" }))).files.map((file) => file.path), ["src/main.go"])
  assert.deepEqual(results(searchDocuments(documents, query("alpha", { include: "docs" }))).files.map((file) => file.path), ["docs/guide.md"])
  assert.deepEqual(results(searchDocuments(documents, query("alpha", { exclude: "docs/**" }))).files.map((file) => file.path), ["src/main.go"])
})

test("results stop at the match limit and report truncation", () => {
  const outcome = results(searchDocuments(documents, query("alpha"), 2))
  assert.equal(outcome.matchCount, 2)
  assert.equal(outcome.truncated, true)
  assert.deepEqual(outcome.files.map((file) => file.matches.length), [2])
})

test("an empty pattern searches nothing", () => {
  assert.deepEqual(searchDocuments(documents, query("")), { kind: "results", files: [], matchCount: 0, truncated: false })
})

test("line previews trim indentation and keep distant matches visible", () => {
  assert.deepEqual(linePreview("    const alpha = 1", 10, 15), { text: "const alpha = 1", highlight: { start: 6, end: 11 } })
  const long = `${"x".repeat(100)}needle${"y".repeat(400)}`
  const preview = linePreview(long, 100, 106)
  assert.ok(preview.text.startsWith("…"))
  assert.equal(preview.text.slice(preview.highlight.start, preview.highlight.end), "needle")
  assert.ok(preview.text.length <= 251)
})

test("the engine sorts files by path and deduplicates documents", () => {
  const engine = createSearchEngine()
  engine.initialize([
    { path: "b.md", name: "b.md", content: "term" },
    { path: "a.md", name: "a.md", content: "term" },
    { path: "a.md", name: "a.md", content: "term" },
  ])
  assert.deepEqual(results(engine.search(query("term"))).files.map((file) => file.path), ["a.md", "b.md"])
})
