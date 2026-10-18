import { parsePathScope, type PathMatcher } from "./search-scope.ts"

// Content search follows VS Code: every match on every line, literal by
// default, with case, whole-word, and regular-expression options.
export const maximumMatches = 2000
const previewContext = 16
const previewLength = 250

export interface HighlightRange {
  end: number
  start: number
}

export interface SearchDocument {
  content: string
  name: string
  path: string
}

export interface SearchQuery {
  exclude: string
  include: string
  matchCase: boolean
  pattern: string
  regex: boolean
  wholeWord: boolean
}

// SearchMatch is one occurrence: a 1-based line and 0-based UTF-16 columns.
export interface SearchMatch {
  column: number
  endColumn: number
  highlight: HighlightRange
  line: number
  preview: string
}

export interface FileResult {
  matches: SearchMatch[]
  name: string
  path: string
}

export type SearchOutcome =
  | { kind: "results"; files: FileResult[]; matchCount: number; truncated: boolean }
  | { kind: "invalid"; field: "exclude" | "include" | "pattern"; message: string }

type Compiled =
  | { kind: "ok"; exclude: PathMatcher | null; include: PathMatcher | null; pattern: RegExp | null }
  | Extract<SearchOutcome, { kind: "invalid" }>

const unicodeWord = "[\\p{L}\\p{N}_]"
const asciiWord = "[A-Za-z0-9_]"

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\/]/g, "\\$&")
}

function buildPattern(source: string, query: SearchQuery, unicode: boolean): RegExp {
  const word = unicode ? unicodeWord : asciiWord
  const body = query.wholeWord ? `(?<!${word})(?:${source})(?!${word})` : source
  return new RegExp(body, `g${unicode ? "u" : ""}${query.matchCase ? "" : "i"}`)
}

function compile(query: SearchQuery): Compiled {
  const include = parsePathScope(query.include)
  if (!include.valid) return { kind: "invalid", field: "include", message: `Invalid pattern: ${include.pattern}` }
  const exclude = parsePathScope(query.exclude)
  if (!exclude.valid) return { kind: "invalid", field: "exclude", message: `Invalid pattern: ${exclude.pattern}` }
  if (!query.pattern) return { kind: "ok", include: include.matcher, exclude: exclude.matcher, pattern: null }
  const source = query.regex ? query.pattern : escapeRegExp(query.pattern)
  try {
    return { kind: "ok", include: include.matcher, exclude: exclude.matcher, pattern: buildPattern(source, query, true) }
  } catch {
    // Unicode mode rejects some escapes other engines accept, such as "\-".
    try {
      return { kind: "ok", include: include.matcher, exclude: exclude.matcher, pattern: buildPattern(source, query, false) }
    } catch {
      return { kind: "invalid", field: "pattern", message: "Invalid regular expression." }
    }
  }
}

// linePreview trims leading indentation and, for a match deep in a long line,
// starts shortly before it so the match stays visible.
export function linePreview(line: string, start: number, end: number): { highlight: HighlightRange; text: string } {
  const indent = line.length - line.trimStart().length
  let from = Math.min(indent, start)
  let prefix = ""
  if (start - from > previewContext) {
    from = start - previewContext
    // Start at a word when one begins within the context.
    const space = line.slice(from, start).search(/\s\S/)
    if (space >= 0) from += space + 1
    prefix = "…"
  }
  const to = Math.min(line.length, Math.max(from + previewLength, Math.min(end, start + previewLength)))
  return {
    text: prefix + line.slice(from, to),
    highlight: { start: start - from + prefix.length, end: Math.min(end, to) - from + prefix.length },
  }
}

function comparePaths(left: SearchDocument, right: SearchDocument): number {
  return left.path < right.path ? -1 : left.path > right.path ? 1 : 0
}

// searchDocuments scans documents in order, stopping after limit matches.
export function searchDocuments(documents: readonly SearchDocument[], query: SearchQuery, limit = maximumMatches): SearchOutcome {
  const compiled = compile(query)
  if (compiled.kind === "invalid") return compiled
  const { pattern, include, exclude } = compiled
  const files: FileResult[] = []
  let matchCount = 0
  let truncated = false
  if (!pattern) return { kind: "results", files, matchCount, truncated }

  for (const document of documents) {
    if (truncated) break
    if (include && !include.matches(document.path)) continue
    if (exclude?.matches(document.path)) continue
    const content = document.content
    // A literal never spans lines, so one scan rules out most documents.
    if (!query.regex) {
      pattern.lastIndex = 0
      if (!pattern.test(content)) continue
    }
    const matches: SearchMatch[] = []
    let lineStart = 0
    let lineNumber = 1
    while (lineStart <= content.length && !truncated) {
      let lineEnd = content.indexOf("\n", lineStart)
      if (lineEnd < 0) lineEnd = content.length
      const line = content.slice(lineStart, content[lineEnd - 1] === "\r" ? lineEnd - 1 : lineEnd)
      pattern.lastIndex = 0
      for (let match = pattern.exec(line); match; match = pattern.exec(line)) {
        const start = match.index
        const end = start + match[0].length
        if (end === start) {
          pattern.lastIndex++
          continue
        }
        if (matchCount >= limit) {
          truncated = true
          break
        }
        const preview = linePreview(line, start, end)
        matches.push({ line: lineNumber, column: start, endColumn: end, preview: preview.text, highlight: preview.highlight })
        matchCount++
      }
      lineStart = lineEnd + 1
      lineNumber++
    }
    if (matches.length) files.push({ path: document.path, name: document.name, matches })
  }
  return { kind: "results", files, matchCount, truncated }
}

export function createSearchEngine() {
  let documents: SearchDocument[] | undefined

  function initialize(next: readonly SearchDocument[]): void {
    documents = [...new Map(next.map((document) => [document.path, document])).values()].sort(comparePaths)
  }

  function search(query: SearchQuery): SearchOutcome {
    if (!documents) throw new Error("Search index has not been initialized")
    return searchDocuments(documents, query)
  }

  return { initialize, search }
}
