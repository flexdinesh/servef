import type { ThemedToken } from "@pierre/highlights"

import type { HighlightsModule } from "./highlights-loader.ts"
import { splitSourceLines } from "./features/documents/line-selection.ts"
import { SYNTAX_THEME } from "./syntax-theme.ts"

// Lines this long or longer render as one unstyled run, as in diffs, so a
// minified line never builds thousands of spans.
export const TOKENIZE_MAX_LINE_LENGTH = 1000

// Languages that mean "no highlighting"; they skip tokenizing.
const plainLanguages: ReadonlySet<string> = new Set(["", "text", "plain", "plaintext", "txt"])

// Token colors are `var(--color-syntax-<role>, ...)` from SYNTAX_THEME. Each
// role renders as an `hl-<role>` class, which is shorter than an inline style
// and keeps file text out of attributes. Unknown roles render unstyled.
const roles: ReadonlySet<string> = new Set([
  "comment", "keyword", "type", "function", "variable", "string", "number", "operator", "inserted", "deleted", "link",
])
const rolePattern = /^var\(--color-syntax-([a-z]+)[,)]/

function escapeHTML(text: string): string {
  return text.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")
}

function tokenClass(token: ThemedToken): string {
  const role = token.color === undefined ? undefined : rolePattern.exec(token.color)?.[1]
  const classes = role !== undefined && roles.has(role) ? [`hl-${role}`] : []
  const fontStyle = token.fontStyle ?? 0
  if (fontStyle & 1) classes.push("hl-italic")
  if (fontStyle & 2) classes.push("hl-bold")
  return classes.join(" ")
}

// renderLine returns escaped HTML for one line, or the escaped line itself
// when the tokens do not spell it exactly: navigation maps clicks to columns
// through the rendered text, so it must equal the source.
function renderLine(tokens: readonly ThemedToken[], line: string): string {
  if (tokens.map((token) => token.content).join("") !== line) return escapeHTML(line)
  let html = ""
  for (const token of tokens) {
    const className = tokenClass(token)
    const text = escapeHTML(token.content)
    html += className ? `<span class="${className}">${text}</span>` : text
  }
  return html
}

// highlightLines returns escaped HTML per viewer row (splitSourceLines), or
// null when the language has no lexer or highlighting fails; callers then
// show plain text. Syntax recognition never blocks viewing.
export function highlightLines(highlights: HighlightsModule, source: string, language: string): string[] | null {
  if (plainLanguages.has(language.toLowerCase()) || !highlights.isSupportedLanguage(language)) return null
  const lines = splitSourceLines(source)
  let tokens: ThemedToken[][]
  try {
    tokens = highlights.codeToTokens(source.replace(/\r\n?/g, "\n"), {
      lang: language,
      theme: SYNTAX_THEME,
      tokenizeMaxLineLength: TOKENIZE_MAX_LINE_LENGTH,
    }).tokens
  } catch {
    return null
  }
  // A final line break yields an empty last row, which the viewer drops.
  const rows = tokens.length > 1 && tokens.at(-1)?.length === 0 ? tokens.slice(0, -1) : tokens
  if (rows.length !== lines.length) return null
  return rows.map((row, index) => renderLine(row, lines[index] ?? ""))
}
