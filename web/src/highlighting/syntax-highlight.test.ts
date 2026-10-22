import assert from "node:assert/strict"
import test from "node:test"

import * as highlights from "@pierre/highlights"

import type { HighlightsModule } from "./highlights-loader.ts"
import { splitSourceLines } from "../features/documents/line-selection.ts"
import { highlightLines, TOKENIZE_MAX_LINE_LENGTH } from "./syntax-highlight.ts"

function text(html: string): string {
  return html.replace(/<[^>]*>/g, "").replaceAll("&lt;", "<").replaceAll("&gt;", ">").replaceAll("&amp;", "&")
}

function rows(source: string, language: string): string[] {
  const lines = highlightLines(highlights, source, language)
  assert.ok(lines, `${language} highlights`)
  return lines
}

test("each row's text equals its source line, so navigation columns hold", () => {
  for (const [source, language] of [
    ["package main\n\nfunc main() {\n\tprintln(\"<b>&</b>\")\n}\n", "go"],
    ["const a = `multi\nline ${1 + 2}`\n// done", "ts"],
    ["key: |\n  <script>\n  second\n", "yaml"],
    ["a = 1\r\nb = 2\rc = 3\n\n", "python"],
    ["const s = \"𝒳é\"; // UTF-16 pairs\n", "js"],
  ]) {
    const lines = rows(source, language)
    assert.deepEqual(lines.map(text), splitSourceLines(source), language)
  }
})

test("tokens render as servef syntax classes and file text is escaped", () => {
  const [keyword, , call] = rows("package main\n\nprintln(\"<b>\")\n", "go")
  assert.match(keyword ?? "", /^<span class="hl-keyword">package <\/span>/)
  assert.ok(call?.includes("&lt;b&gt;"))
  assert.ok(!call?.includes("<b>"))
  const [comment] = rows("// note\n", "go")
  assert.equal(comment, `<span class="hl-comment hl-italic">// note</span>`)
})

test("multiline state carries across rows", () => {
  const lines = rows("/* open\nstill comment\n*/ x\n", "c")
  assert.ok(lines[1]?.includes("hl-comment"))
})

test("plain, image, and unknown languages skip highlighting", () => {
  for (const language of ["text", "plain", "", "servef-unknown", "TEXT"]) {
    assert.equal(highlightLines(highlights, "a\n", language), null, language)
  }
})

test("very long lines render as one unstyled run", () => {
  const long = `x = "${"a".repeat(TOKENIZE_MAX_LINE_LENGTH)}"`
  const [line] = rows(`${long}\n`, "python")
  assert.equal(line, long)
})

test("highlighter failures fall back to plain text", () => {
  const failing: HighlightsModule = {
    isSupportedLanguage: highlights.isSupportedLanguage,
    codeToTokens() {
      throw new RangeError("lexer failed")
    },
  }
  assert.equal(highlightLines(failing, "package main\n", "go"), null)
  const short: HighlightsModule = {
    isSupportedLanguage: highlights.isSupportedLanguage,
    codeToTokens: () => ({ tokens: [[{ content: "other", offset: 0 }]] }),
  }
  // Tokens that do not spell the line render the line itself.
  assert.deepEqual(highlightLines(short, "package main\n", "go"), ["package main"])
  const misaligned: HighlightsModule = {
    isSupportedLanguage: highlights.isSupportedLanguage,
    codeToTokens: () => ({ tokens: [[], [], []] }),
  }
  assert.equal(highlightLines(misaligned, "package main\n", "go"), null)
})
