import assert from "node:assert/strict"
import test from "node:test"

import { countFiles, filterTreeByFilename, isDefaultFilenameScope, parseFilenameFilter } from "./filename-filter.ts"
import type { TreeNode } from "../../api/page-contract.ts"

function matches(pattern: string, name: string): boolean {
  const result = parseFilenameFilter(pattern)
  assert.equal(result.valid, true, `${pattern} should be valid`)
  return result.valid && result.matcher !== null && result.matcher.matches(name)
}

// Cases follow Go's path.Match table, limited to slash-free base names.
test("globs follow Go path.Match semantics for base names", () => {
  const cases: ReadonlyArray<readonly [string, string, boolean]> = [
    ["abc", "abc", true],
    ["*", "abc", true],
    ["*c", "abc", true],
    ["a*", "a", true],
    ["a*", "abc", true],
    ["a*b*c*d*e*f", "axbxcxdxexxxf", true],
    ["a*b?c*x", "abxbbxdbxebxczzx", true],
    ["a*b?c*x", "abxbbxdbxebxczzy", false],
    ["ab[c]", "abc", true],
    ["ab[b-d]", "abc", true],
    ["ab[e-g]", "abc", false],
    ["ab[^c]", "abc", false],
    ["ab[^b-d]", "abc", false],
    ["ab[^e-g]", "abc", true],
    ["a\\*b", "a*b", true],
    ["a\\*b", "ab", false],
    ["a?b", "a☺b", true],
    ["a[^a]b", "a☺b", true],
    ["a???b", "a☺b", false],
    ["[a-ζ]*", "α", true],
    ["[\\]a]", "]", true],
    ["[\\-]", "-", true],
    ["[x\\-]", "x", true],
    ["[x\\-]", "-", true],
    ["[x\\-]", "z", false],
    ["[\\-x]", "a", false],
    ["*x", "xxx", true],
    ["[z-a]", "m", false],
    ["*.md", "readme.md.bak", false],
    ["?", "", false],
    ["*", "", true],
  ]
  for (const [pattern, name, expected] of cases) {
    assert.equal(matches(pattern, name), expected, `${pattern} vs ${name}`)
  }
})

test("malformed globs report the offending entry", () => {
  for (const pattern of ["[]a]", "[-]", "[x-]", "[-x]", "\\", "[a-b-c]", "[", "[^", "[^bc", "a[", "[a-"]) {
    assert.deepEqual(parseFilenameFilter(`*.md, ${pattern}`), { valid: false, pattern }, pattern)
  }
})

test("lists are comma-separated, trimmed, case-insensitive, and empty means all", () => {
  assert.equal(matches("*.md, *.GO", "MAIN.go"), true)
  assert.equal(matches("*.md, *.GO", "README.MD"), true)
  assert.equal(matches("*.md, *.GO", "main.rs"), false)
  assert.equal(matches("Dockerfile", "dockerfile"), true)
  assert.equal(matches("[A-C]*", "beta.txt"), true)
  assert.equal(matches("*.md,", "a.md"), true)
})

test("case folding matches Go strings.ToLower rune by rune", () => {
  // Go lowers İ (U+0130) to a single "i", never "i" + combining dot.
  assert.equal(matches("?", "İ"), true)
  assert.equal(matches("İ?", "ix"), true)
  assert.equal(matches("i", "İ"), true)
  // Go has no final-sigma rule: Σ is always σ, and ς stays distinct.
  assert.equal(matches("*σ", "ΑΣ"), true)
  assert.equal(matches("ας", "ΑΣ"), false)
  assert.equal(matches("ς", "ς"), true)
  assert.equal(matches("ẞ*", "ß.txt"), true)
  assert.deepEqual(parseFilenameFilter(""), { valid: true, matcher: null })
  assert.deepEqual(parseFilenameFilter(" , "), { valid: true, matcher: null })
})

function file(path: string, fileType = "text"): TreeNode {
  return { name: path.split("/").at(-1) ?? path, path, fileType, children: [], isDir: false, open: false, selected: false }
}

const tree: TreeNode[] = [
  { name: "config", path: "config", fileType: "", isDir: true, open: true, selected: false,
    children: [file("config/settings.json", "json"), file("config/settings.yaml", "yaml")] },
  { name: "docs.md", path: "docs.md", fileType: "", isDir: true, open: false, selected: false,
    children: [file("docs.md/main.go", "go")] },
  file("README.md", "markdown"),
]

test("tree filtering matches file base names, prunes empty folders, and preserves metadata", () => {
  const result = parseFilenameFilter("*.md, settings.*")
  assert.equal(result.valid, true)
  const matcher = result.valid ? result.matcher : null
  const visible = filterTreeByFilename(tree, matcher)
  assert.deepEqual(visible.map((node) => node.path), ["config", "README.md"])
  assert.deepEqual(visible[0]?.children.map((node) => node.path), ["config/settings.json", "config/settings.yaml"])
  assert.equal(visible[0]?.open, true)
  assert.equal(tree[0]?.children.length, 2)
  assert.deepEqual(filterTreeByFilename(tree, null), tree)
  assert.equal(countFiles(tree), 4)
  assert.equal(countFiles(visible), 3)
})

test("default CLI scope is a single star", () => {
  assert.equal(isDefaultFilenameScope(["*"]), true)
  assert.equal(isDefaultFilenameScope([]), true)
  assert.equal(isDefaultFilenameScope(["*.md"]), false)
  assert.equal(isDefaultFilenameScope(["*", "*.md"]), false)
})
