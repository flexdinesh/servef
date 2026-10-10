import MiniSearch, { type SearchResult as MiniSearchResult } from "minisearch"

const maximumResults = 10
const snippetLength = 180

export interface HighlightRange {
  end: number
  start: number
}

export interface SearchDocument {
  content: string
  name: string
  path: string
}

export interface SearchSnippet {
  highlights: HighlightRange[]
  text: string
}

export interface SearchResult {
  name: string
  path: string
  score: number
  snippet?: SearchSnippet
}

export interface SearchGroups {
  contentResults: SearchResult[]
  pathResults: SearchResult[]
}

interface Occurrence extends HighlightRange {
  termIndex: number
}

export function queryTerms(query: string): string[] {
  const matches = query.toLocaleLowerCase().match(/[\p{L}\p{N}]+/gu)
  return [...new Set(matches || [])]
}

function occurrencesFor(content: string, terms: readonly string[]): Occurrence[] {
  const lowerContent = content.toLocaleLowerCase()
  const occurrences: Occurrence[] = []
  terms.forEach((term, termIndex) => {
    let from = 0
    let count = 0
    while (from < lowerContent.length && count < 100) {
      const position = lowerContent.indexOf(term, from)
      if (position < 0) break
      occurrences.push({ start: position, end: position + term.length, termIndex })
      from = position + Math.max(1, term.length)
      count += 1
    }
  })
  return occurrences.sort((left, right) => left.start - right.start || left.end - right.end)
}

function mergeRanges(ranges: readonly HighlightRange[]): HighlightRange[] {
  const merged: HighlightRange[] = []
  for (const range of ranges) {
    const previous = merged.at(-1)
    if (previous && range.start <= previous.end) previous.end = Math.max(previous.end, range.end)
    else merged.push({ ...range })
  }
  return merged
}

export function bestSnippet(content: string, query: string): SearchSnippet {
  const text = String(content || "")
  const terms = queryTerms(query)
  const occurrences = occurrencesFor(text, terms)
  const firstOccurrence = occurrences[0]
  if (!firstOccurrence) {
    const clipped = text.length > snippetLength
    return { text: text.slice(0, snippetLength) + (clipped ? "…" : ""), highlights: [] }
  }

  let best = { score: -1, start: firstOccurrence.start }
  for (const anchor of occurrences) {
    const windowStart = Math.max(0, anchor.start - 60)
    const windowEnd = Math.min(text.length, windowStart + snippetLength)
    const inside = occurrences.filter((item) => item.start < windowEnd && item.end > windowStart)
    const distinctTerms = new Set(inside.map((item) => item.termIndex)).size
    const score = distinctTerms * 100 + inside.length
    if (score > best.score) best = { score, start: anchor.start }
  }

  let start = Math.max(0, best.start - 60)
  let end = Math.min(text.length, start + snippetLength)
  if (start > 0) {
    const boundary = text.slice(start, Math.min(end, start + 24)).search(/\s/)
    if (boundary >= 0) start += boundary + 1
  }
  if (end < text.length) {
    const boundary = text.slice(Math.max(start, end - 24), end).search(/\s[^\s]*$/)
    if (boundary >= 0) end = Math.max(start, end - 24) + boundary
  }

  const leadingEllipsis = start > 0 ? "…" : ""
  const trailingEllipsis = end < text.length ? "…" : ""
  const offset = leadingEllipsis.length
  const highlights = mergeRanges(
    occurrences
      .filter((item) => item.start < end && item.end > start)
      .map((item) => ({
        start: Math.max(item.start, start) - start + offset,
        end: Math.min(item.end, end) - start + offset,
      })),
  )
  return {
    text: leadingEllipsis + text.slice(start, end) + trailingEllipsis,
    highlights,
  }
}

function resultIdentity(result: MiniSearchResult): Pick<SearchResult, "name" | "path" | "score"> | null {
  const path: unknown = result.path
  const name: unknown = result.name
  if (typeof path !== "string") return null
  return {
    path,
    name: typeof name === "string" ? name : path,
    score: result.score,
  }
}

export function createSearchEngine() {
  let contentIndex: MiniSearch<SearchDocument> | undefined

  function initialize(documents: readonly SearchDocument[]): void {
    contentIndex = new MiniSearch<SearchDocument>({ fields: ["content"], storeFields: ["path", "name", "content"], idField: "path" })
    const uniqueDocuments = [...new Map(documents.map((document) => [document.path, document])).values()]
    contentIndex.addAll(uniqueDocuments)
  }

  function search(query: string): SearchGroups {
    if (!contentIndex) throw new Error("Search index has not been initialized")
    const contentResults = contentIndex.search(query, {
      combineWith: "AND",
      prefix: true,
      fuzzy: false,
    }).slice(0, maximumResults).flatMap((result) => {
      const identity = resultIdentity(result)
      const content: unknown = result.content
      return identity && typeof content === "string"
        ? [{ ...identity, snippet: bestSnippet(content, query) }]
        : []
    })
    return { pathResults: [], contentResults }
  }

  return { initialize, search }
}
