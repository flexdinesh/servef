import assert from "node:assert/strict"
import test from "node:test"

import { parsePathScope } from "./search-scope.ts"

function matcher(input: string) {
  const result = parsePathScope(input)
  assert.equal(result.valid, true)
  if (!result.valid || !result.matcher) throw new Error("expected a matcher")
  return result.matcher
}

test("empty scopes match nothing and impose no filter", () => {
  assert.deepEqual(parsePathScope(" , "), { valid: true, matcher: null })
})

test("patterns without a leading ./ match at any depth, case-insensitively", () => {
  const scope = matcher("*.MD")
  assert.equal(scope.matches("README.md"), true)
  assert.equal(scope.matches("docs/guides/intro.md"), true)
  assert.equal(scope.matches("docs/intro.mdx"), false)
})

test("a matched folder covers its contents; ./ anchors at the project root", () => {
  assert.equal(matcher("src").matches("web/src/app.ts"), true)
  assert.equal(matcher("./src").matches("web/src/app.ts"), false)
  assert.equal(matcher("./src/").matches("src/app.ts"), true)
})

test("* and ? stay within a segment while ** spans segments", () => {
  assert.equal(matcher("./src/*.ts").matches("src/app.ts"), true)
  assert.equal(matcher("./src/*.ts").matches("src/lib/app.ts"), false)
  assert.equal(matcher("./src/**/*.ts").matches("src/lib/deep/app.ts"), true)
  assert.equal(matcher("./src/**/*.ts").matches("src/app.ts"), true)
  assert.equal(matcher("a?c.md").matches("abc.md"), true)
  assert.equal(matcher("a?c.md").matches("a/c.md"), false)
})

test("braces alternate and classes match one character", () => {
  const scope = matcher("*.{ts,tsx}, docs")
  assert.equal(scope.matches("web/App.tsx"), true)
  assert.equal(scope.matches("web/app.ts"), true)
  assert.equal(scope.matches("docs/a.md"), true)
  assert.equal(scope.matches("web/app.js"), false)
  assert.equal(matcher("file[0-9].md").matches("file7.md"), true)
  assert.equal(matcher("file[!0-9].md").matches("file7.md"), false)
})

test("unterminated classes and braces are invalid", () => {
  assert.deepEqual(parsePathScope("*.md, ["), { valid: false, pattern: "[" })
  assert.deepEqual(parsePathScope("{a,b"), { valid: false, pattern: "{a,b" })
})
