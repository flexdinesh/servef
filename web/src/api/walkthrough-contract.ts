import { isRecord } from "./http.ts"

// Transport contract for walkthroughs (decision 0047): GET
// /api/walkthroughs?project= lists summaries, newest first; GET
// /api/walkthrough?project=&id= reads one with its Markdown rendered; DELETE
// /api/walkthroughs?project=&id= removes one. A missing walkthrough is a 404
// with code walkthroughUnavailableCode. Agents publish with POST; the browser
// never does.

export const walkthroughUnavailableCode = "walkthroughUnavailable"

export interface WalkthroughSummary {
  id: string
  title: string
  stepCount: number
  // updatedAt is an RFC 3339 time.
  updatedAt: string
}

// WalkthroughLines is an inclusive, 1-based [start, end].
export type WalkthroughLines = readonly [number, number]

export interface WalkthroughFileTarget {
  path: string
  lines?: WalkthroughLines
  // at is a full commit hash: the file as of that commit.
  at?: string
  // expect is the trimmed text the first line started with when written.
  expect?: string
}

export interface WalkthroughCompare {
  kind: "working" | "branch" | "commit" | "range"
  base?: string
  head?: string
  commit?: string
  from?: string
  to?: string
}

export interface WalkthroughDiffTarget {
  compare: WalkthroughCompare
  path?: string
  side?: "new" | "old"
  lines?: WalkthroughLines
  expect?: string
}

export interface WalkthroughStep {
  title: string
  body?: string
  // bodyHTML is body rendered as a Markdown preview, "" without a body.
  bodyHTML: string
  file?: WalkthroughFileTarget
  diff?: WalkthroughDiffTarget
}

export interface Walkthrough {
  id: string
  project: string
  title: string
  summary?: string
  summaryHTML: string
  steps: WalkthroughStep[]
  createdAt: string
  updatedAt: string
}

function optionalString(value: unknown): boolean {
  return value === undefined || typeof value === "string"
}

function isLines(value: unknown): value is WalkthroughLines {
  return Array.isArray(value) && value.length === 2
    && value.every((line) => Number.isInteger(line) && line >= 1)
    && value[0] <= value[1]
}

function isFileTarget(value: unknown): value is WalkthroughFileTarget {
  return isRecord(value)
    && typeof value.path === "string" && value.path !== ""
    && (value.lines === undefined || isLines(value.lines))
    && optionalString(value.at)
    && optionalString(value.expect)
}

function isCompare(value: unknown): value is WalkthroughCompare {
  return isRecord(value)
    && (value.kind === "working" || value.kind === "branch" || value.kind === "commit" || value.kind === "range")
    && optionalString(value.base) && optionalString(value.head) && optionalString(value.commit)
    && optionalString(value.from) && optionalString(value.to)
}

function isDiffTarget(value: unknown): value is WalkthroughDiffTarget {
  return isRecord(value)
    && isCompare(value.compare)
    && optionalString(value.path)
    && (value.side === undefined || value.side === "new" || value.side === "old")
    && (value.lines === undefined || isLines(value.lines))
    && optionalString(value.expect)
}

function isStep(value: unknown): value is WalkthroughStep {
  return isRecord(value)
    && typeof value.title === "string"
    && optionalString(value.body)
    && typeof value.bodyHTML === "string"
    && (value.file === undefined || isFileTarget(value.file))
    && (value.diff === undefined || isDiffTarget(value.diff))
    && (value.file === undefined || value.diff === undefined)
}

function isSummary(value: unknown): value is WalkthroughSummary {
  return isRecord(value)
    && typeof value.id === "string" && value.id !== ""
    && typeof value.title === "string"
    && typeof value.stepCount === "number" && Number.isInteger(value.stepCount) && value.stepCount >= 0
    && typeof value.updatedAt === "string"
}

export function isWalkthroughSummaries(value: unknown): value is WalkthroughSummary[] {
  return Array.isArray(value) && value.every(isSummary)
}

export function isWalkthrough(value: unknown): value is Walkthrough {
  return isRecord(value)
    && typeof value.id === "string" && value.id !== ""
    && typeof value.project === "string"
    && typeof value.title === "string"
    && optionalString(value.summary)
    && typeof value.summaryHTML === "string"
    && Array.isArray(value.steps) && value.steps.length > 0 && value.steps.every(isStep)
    && typeof value.createdAt === "string"
    && typeof value.updatedAt === "string"
}
