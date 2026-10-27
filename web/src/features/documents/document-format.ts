import { fetchJSON, RequestError, type RequestFailure } from "../../api/http.ts"
import { isFormatResult } from "../../api/page-contract.ts"
import { projectUrl } from "../../api/project-url.ts"

export type FormatState =
  | { kind: "loading" }
  | { kind: "ready"; text: string }
  | { kind: "error"; message: string }

// formatFailureMessage turns a server reason into the inline notice sentence.
export function formatFailureMessage(reason: string): string {
  const trimmed = reason.trim().replace(/[.\s]+$/, "")
  return `Couldn't format: ${trimmed || "the server returned an invalid response"}.`
}

// formatError gives the reason formatFailureMessage turns into the notice.
function formatError(failure: RequestFailure): string {
  switch (failure.kind) {
    case "unreachable":
      return "the server could not be reached"
    case "rejected":
      return failure.error.trim()
    case "malformed":
    case "invalid":
      return failure.ok ? "the server returned an invalid response" : `the server responded with ${failure.status}`
  }
}

// fetchFormat formats a working-tree document, or with at a snapshot.
export async function fetchFormat(project: string, path: string, at: string, signal: AbortSignal): Promise<FormatState> {
  try {
    const url = projectUrl("/api/format", project, path) + (at ? `&at=${at}` : "")
    const payload = await fetchJSON(url, { signal }, isFormatResult, formatError)
    return { kind: "ready", text: payload.formatted }
  } catch (error: unknown) {
    if (error instanceof RequestError) return { kind: "error", message: error.message }
    throw error
  }
}
