import type { FileResult, SearchMatch, SearchOutcome } from "./search-engine.ts"

// ResultRow is one visible row of the results tree: a file or one of its matches.
export type ResultRow =
  | { kind: "file"; file: FileResult; key: string }
  | { kind: "match"; file: FileResult; match: SearchMatch; matchIndex: number; key: string }

export interface MatchPosition {
  matchIndex: number
  path: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

function isRange(value: unknown): boolean {
  return isRecord(value) && typeof value.start === "number" && typeof value.end === "number"
}

function isMatch(value: unknown): value is SearchMatch {
  return isRecord(value)
    && typeof value.line === "number"
    && typeof value.column === "number"
    && typeof value.endColumn === "number"
    && typeof value.preview === "string"
    && isRange(value.highlight)
}

function isFileResult(value: unknown): value is FileResult {
  return isRecord(value)
    && typeof value.path === "string"
    && typeof value.name === "string"
    && Array.isArray(value.matches)
    && value.matches.every(isMatch)
}

export function isSearchOutcome(value: unknown): value is SearchOutcome {
  if (!isRecord(value)) return false
  if (value.kind === "invalid") {
    return (value.field === "pattern" || value.field === "include" || value.field === "exclude") && typeof value.message === "string"
  }
  return value.kind === "results"
    && Array.isArray(value.files)
    && value.files.every(isFileResult)
    && typeof value.matchCount === "number"
    && typeof value.truncated === "boolean"
}

export function fileRowKey(path: string): string {
  return `file:${path}`
}

export function matchRowKey(path: string, matchIndex: number): string {
  return `match:${path}:${matchIndex}`
}

// resultRows flattens files and their expanded matches in display order.
export function resultRows(files: readonly FileResult[], collapsed: ReadonlySet<string>): ResultRow[] {
  return files.flatMap((file): ResultRow[] => [
    { kind: "file", file, key: fileRowKey(file.path) },
    ...(collapsed.has(file.path) ? [] : file.matches.map((match, matchIndex): ResultRow => ({
      kind: "match", file, match, matchIndex, key: matchRowKey(file.path, matchIndex),
    }))),
  ])
}

// adjacentMatch steps through every match in order, wrapping at either end,
// as VS Code's F4 and Shift+F4 do. Collapsed files are still visited.
export function adjacentMatch(files: readonly FileResult[], current: MatchPosition | null, delta: 1 | -1): MatchPosition | null {
  const positions = files.flatMap((file) => file.matches.map((_, matchIndex) => ({ path: file.path, matchIndex })))
  if (!positions.length) return null
  const index = current ? positions.findIndex((position) => position.path === current.path && position.matchIndex === current.matchIndex) : -1
  if (index < 0) return positions[delta > 0 ? 0 : positions.length - 1] ?? null
  return positions[(index + delta + positions.length) % positions.length] ?? null
}

function plural(count: number, singular: string, pluralForm: string): string {
  return `${count.toLocaleString()} ${count === 1 ? singular : pluralForm}`
}

export function resultSummary(matchCount: number, fileCount: number, truncated: boolean): string {
  const results = truncated ? `${matchCount.toLocaleString()}+ results` : plural(matchCount, "result", "results")
  return `${results} in ${plural(fileCount, "file", "files")}`
}
