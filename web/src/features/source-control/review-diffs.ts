import type { FileDiffMetadata } from "@pierre/diffs"
import { useEffect, useState } from "react"

import { isDiffResponse, type DiffResponse } from "../../api/changes-contract.ts"
import { failureReason, fetchJSON, isAbort, RequestError, type RequestFailure } from "../../api/http.ts"
import { boundedCache } from "../../lib/bounded-cache.ts"
import { compareKey, type Compare } from "./compare.ts"
import type { CompareFile } from "./compare-files.ts"
import { diffContent, type DiffContent } from "./diff-content.ts"
import { parseDiff, releaseDiffResources } from "./use-parsed-diff.ts"

// A review tab reads each file's diff when its section nears the viewport
// (decision 0035). Parsed diffs stay in a bounded cache shared by review
// tabs, so scrolling back is instant while memory stays bounded.

export type ReviewDiff =
  | { kind: "loading" }
  | { kind: "ready"; diff: DiffResponse; content: DiffContent; fileDiff: FileDiffMetadata | null; parseFailed: boolean }
  // unchanged: the file no longer differs, e.g. a branch dropped it.
  | { kind: "unchanged"; message: string }
  | { kind: "error"; message: string }

export const maxParsedDiffs = 30

// cache holds settled diffs.
const cache = boundedCache<ReviewDiff>(maxParsedDiffs)

// releaseReviewDiffs drops cached diffs and the parse worker once no review
// tab is open.
export function releaseReviewDiffs(): void {
  cache.clear()
  releaseDiffResources()
}

// diffError keeps the server's own sentence; other failures say what failed.
function diffError(failure: RequestFailure): string {
  return failure.kind === "rejected" ? failure.error : `Couldn't load this diff: ${failureReason(failure)}.`
}

export function diffUrl(project: string, compare: Compare, file: CompareFile): string {
  const params = new URLSearchParams({ project, path: file.path })
  if (file.oldPath) params.set("oldPath", file.oldPath)
  if (compare.kind === "commit") params.set("commit", compare.hash)
  if (compare.kind === "branch") {
    params.set("branch", "1")
    if (compare.base) params.set("base", compare.base)
  }
  if (compare.kind === "range") {
    params.set("from", compare.from)
    params.set("to", compare.to)
  }
  return `/api/diff?${params.toString()}`
}

async function readDiff(url: string, ignoreWhitespace: boolean, signal: AbortSignal): Promise<ReviewDiff> {
  let diff: DiffResponse
  try {
    diff = await fetchJSON(url, { signal }, isDiffResponse, diffError)
  } catch (error: unknown) {
    if (!(error instanceof RequestError)) throw error
    // Code "unchanged": the file matches again, or the branch no longer changes it.
    return error.failure.kind === "rejected" && error.failure.code === "unchanged"
      ? { kind: "unchanged", message: error.message }
      : { kind: "error", message: error.message }
  }
  const content = diffContent(diff)
  if (content.kind !== "text" || (!content.oldFile && !content.newFile)) {
    return { kind: "ready", diff, content, fileDiff: null, parseFailed: false }
  }
  try {
    const fileDiff = await parseDiff(content.oldFile, content.newFile, ignoreWhitespace, signal)
    return { kind: "ready", diff, content, fileDiff, parseFailed: false }
  } catch (error: unknown) {
    if (isAbort(error)) throw error
    return { kind: "ready", diff, content, fileDiff: null, parseFailed: true }
  }
}

interface ReviewDiffSource {
  project: string
  compare: Compare
  file: CompareFile
  // version names the list read: live compares reread diffs when it changes;
  // "" for commits, which never change. Stats arriving for the same read do
  // not change it.
  version: string
  ignoreWhitespace: boolean
}

interface ReviewDiffOptions extends ReviewDiffSource {
  // wanted is false while the section is collapsed or far from view.
  wanted: boolean
}

function diffKey({ project, compare, file, version, ignoreWhitespace }: ReviewDiffSource): string {
  return `${project}\n${compareKey(compare)}\n${file.path}\n${file.oldPath ?? ""}\n${version}\n${ignoreWhitespace ? 1 : 0}`
}

// loadReviewDiff reads a file's diff ahead of its section, as a step into a
// file not yet shown does; its section then finds it cached.
export async function loadReviewDiff(source: ReviewDiffSource, signal: AbortSignal): Promise<ReviewDiff> {
  const key = diffKey(source)
  const cached = cache.get(key)
  if (cached) return cached
  const diff = await readDiff(diffUrl(source.project, source.compare, source.file), source.ignoreWhitespace, signal)
  if (diff.kind !== "error") cache.set(key, diff)
  return diff
}

// useReviewDiff reads and parses one file's diff while wanted.
export function useReviewDiff({ project, compare, file, version, ignoreWhitespace, wanted }: ReviewDiffOptions) {
  const key = diffKey({ project, compare, file, version, ignoreWhitespace })
  const [entry, setEntry] = useState<{ key: string; diff: ReviewDiff } | null>(null)
  const [attempt, setAttempt] = useState(0)
  const url = diffUrl(project, compare, file)
  const cached = wanted ? cache.get(key) : undefined
  // A section scrolled away lets go of its diff; the cache may keep it.
  if (!wanted && entry) setEntry(null)

  useEffect(() => {
    if (!wanted || !project || cache.has(key)) return
    const controller = new AbortController()
    readDiff(url, ignoreWhitespace, controller.signal).then((diff) => {
      // Failures are not kept, so scrolling back retries them.
      if (diff.kind !== "error") cache.set(key, diff)
      setEntry({ key, diff })
    }, () => {
      // Aborted by scrolling away, a newer read, or unmount.
    })
    return () => controller.abort()
  }, [attempt, ignoreWhitespace, key, project, url, wanted])

  const retry = () => {
    cache.delete(key)
    setEntry(null)
    setAttempt((count) => count + 1)
  }
  // A reread (new content or whitespace option) keeps the last diff shown
  // until it lands, so sections keep their height and the review its scroll.
  const diff: ReviewDiff = cached ?? entry?.diff ?? { kind: "loading" }
  return { diff, retry }
}
