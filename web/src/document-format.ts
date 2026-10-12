import { isFormatResult } from "./page-contract.ts"

export type FormatState =
  | { kind: "loading" }
  | { kind: "ready"; lines: readonly string[] }
  | { kind: "error"; message: string }

function serverError(payload: unknown): string {
  if (typeof payload !== "object" || payload === null || !("error" in payload)) return ""
  return typeof payload.error === "string" ? payload.error.trim() : ""
}

// formatFailureMessage turns a server reason into the inline notice sentence.
export function formatFailureMessage(reason: string): string {
  const trimmed = reason.trim().replace(/[.\s]+$/, "")
  return `Couldn't format: ${trimmed || "the server returned an invalid response"}.`
}

export async function fetchFormat(path: string, signal: AbortSignal): Promise<FormatState> {
  let response: Response
  try {
    response = await fetch(`/api/format?path=${encodeURIComponent(path)}`, {
      headers: { Accept: "application/json" },
      signal,
    })
  } catch (error: unknown) {
    if (error instanceof Error && error.name === "AbortError") throw error
    return { kind: "error", message: "the server could not be reached" }
  }

  let payload: unknown = null
  try {
    payload = await response.json()
  } catch (error: unknown) {
    if (error instanceof Error && error.name === "AbortError") throw error
  }
  if (response.ok && isFormatResult(payload)) return { kind: "ready", lines: payload.formattedLines }
  return {
    kind: "error",
    message: serverError(payload) || (response.ok
      ? "the server returned an invalid response"
      : `the server responded with ${response.status}`),
  }
}
