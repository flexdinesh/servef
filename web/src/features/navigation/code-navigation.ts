import type { NavigationLocation, NavigationResult, NavigationStatus } from "../../api/navigation-contract.ts"

// SourcePosition addresses a character in Source: a 1-based line and a
// 0-based UTF-16 column into the line's text.
export interface SourcePosition {
  line: number
  column: number
}

// ColumnRange is a half-open span of UTF-16 columns within one line.
export interface ColumnRange {
  start: number
  end: number
}

const identifierPart = /[\p{L}\p{N}_$]/u

function isIdentifierPart(text: string, index: number): boolean {
  const character = text[index]
  return character !== undefined && identifierPart.test(character)
}

// identifierAt finds the word under a column, for hover feedback only; the
// server decides what the word means. A column just past a word counts, so
// pointing at a word's trailing edge still finds it.
export function identifierAt(text: string, column: number): ColumnRange | null {
  let at = column
  if (!isIdentifierPart(text, at) && isIdentifierPart(text, at - 1)) at -= 1
  if (!isIdentifierPart(text, at)) return null
  let start = at
  while (isIdentifierPart(text, start - 1)) start -= 1
  // Private class names start with #.
  if (text[start - 1] === "#") start -= 1
  let end = at + 1
  while (isIdentifierPart(text, end)) end += 1
  return /^[\p{N}]/u.test(text.slice(start, end)) ? null : { start, end }
}

export type DefinitionOutcome =
  | { kind: "none" }
  | { kind: "open"; location: NavigationLocation }
  | { kind: "references" }
  | { kind: "choose"; locations: NavigationLocation[] }

// definitionOutcome follows VS Code: one definition opens; several open a
// list; a definition that is the clicked word itself shows its references.
export function definitionOutcome(result: NavigationResult, path: string, position: SourcePosition): DefinitionOutcome {
  const [first, ...rest] = result.locations
  if (!first) return { kind: "none" }
  if (rest.length > 0) return { kind: "choose", locations: result.locations }
  if (first.path === path
    && (position.line > first.line || (position.line === first.line && position.column >= first.column))
    && (position.line < first.endLine || (position.line === first.endLine && position.column < first.endColumn))) {
    return { kind: "references" }
  }
  return { kind: "open", location: first }
}

export interface LocationGroup {
  path: string
  locations: NavigationLocation[]
}

// groupLocations groups by file in first-seen order, as the server ranks.
export function groupLocations(locations: readonly NavigationLocation[]): LocationGroup[] {
  const groups = new Map<string, NavigationLocation[]>()
  for (const location of locations) {
    const group = groups.get(location.path)
    if (group) group.push(location)
    else groups.set(location.path, [location])
  }
  return [...groups].map(([path, grouped]) => ({ path, locations: grouped }))
}

// navigationQuery builds the query for a navigation endpoint.
export function navigationQuery(project: string, path: string, position: SourcePosition): string {
  return new URLSearchParams({ project, path, line: String(position.line), column: String(position.column) }).toString()
}

export type NavigationKind = "definition" | "references"

// navigationTitle heads the result list.
export function navigationTitle(kind: NavigationKind, symbol: string, count: number): string {
  const noun = kind === "definition" ? (count === 1 ? "definition" : "definitions") : (count === 1 ? "reference" : "references")
  return symbol ? `${count} ${noun} ${kind === "definition" ? "of" : "to"} ${symbol}` : `${count} ${noun}`
}

// emptyMessage explains a result without locations.
export function emptyMessage(kind: NavigationKind, result: NavigationResult): string {
  if (result.filtered > 0) return "The language server found results outside the browsable project scope."
  if (result.symbol === "") return "No results at this position."
  return kind === "definition" ? `No definition found for ${result.symbol}.` : `No references found for ${result.symbol}.`
}

// Navigation eligibility is independent of a server's installed/running state.
export function navigationEligibility(source: boolean, supported: boolean, enabled: boolean, status: NavigationStatus | null): string {
  if (!source) return "Code navigation works in the Source view."
  if (!supported) return "No language server is configured for this language."
  if (!enabled) return "Language servers are off. Turn them on in Settings."
  if (status && !status.eligible) return status.reason || "This file is unavailable for code navigation."
  return ""
}

export function resultNotice(result: NavigationResult): string {
  return [
    result.filtered > 0 ? `${result.filtered} result${result.filtered === 1 ? "" : "s"} outside the browsable project scope omitted.` : "",
    result.truncated ? "Results truncated. Narrow the query in your editor for more results." : "",
  ].filter(Boolean).join(" ")
}
