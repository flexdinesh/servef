import { isPageData, type PageData } from "@servef/contracts"

import { isRecord } from "./http.ts"

// DocumentMode is the view a document shows: a declared default mode
// (FileType.defaultMode) or the on-demand Format.
export type DocumentMode = "preview" | "source" | "format"

export type PageLoadResult =
  | { kind: "page"; page: PageData }
  | { kind: "failure"; message: string }

export function isPageLoadResult(value: unknown): value is PageLoadResult {
  if (!isRecord(value) || typeof value.kind !== "string") return false
  if (value.kind === "page") return isPageData(value.page)
  return value.kind === "failure" && typeof value.message === "string"
}
