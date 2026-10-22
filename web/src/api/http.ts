// http.ts is the browser's one JSON transport: every API read goes through
// fetchJSON, which turns each way a request can fail into a user-facing Error.

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

// isAbort reports a request cancelled by its AbortSignal.
export function isAbort(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError"
}

// RequestFailure is why a request produced no payload.
export type RequestFailure =
  // unreachable: no response arrived.
  | { kind: "unreachable" }
  // malformed: the body is not JSON.
  | { kind: "malformed"; status: number; ok: boolean }
  // rejected: the server explained, as { "error": string, "code"?: string }.
  | { kind: "rejected"; status: number; error: string; code: string }
  // invalid: JSON that is not the expected payload and carries no reason.
  | { kind: "invalid"; status: number; ok: boolean }

// RequestError carries the user-facing message and the failure behind it.
export class RequestError extends Error {
  readonly failure: RequestFailure

  constructor(message: string, failure: RequestFailure) {
    super(message)
    this.name = "RequestError"
    this.failure = failure
  }
}

// failureReason phrases a failure as a lowercase reason for prefixed messages
// such as "Couldn't read git status: <reason>.".
export function failureReason(failure: RequestFailure): string {
  switch (failure.kind) {
    case "unreachable":
      return "the server didn't respond"
    case "rejected":
      return failure.error
    case "malformed":
    case "invalid":
      return "the server sent an invalid response"
  }
}

export interface FetchJSONOptions {
  signal?: AbortSignal
  headers?: Record<string, string>
  // anyStatus accepts a valid payload at any status, for responses such as
  // pages that carry their own error.
  anyStatus?: boolean
}

function serverReason(payload: unknown): { error: string; code: string } | null {
  if (!isRecord(payload) || typeof payload.error !== "string" || !payload.error.trim()) return null
  if (payload.code !== undefined && typeof payload.code !== "string") return null
  return { error: payload.error, code: payload.code ?? "" }
}

// fetchJSON reads url and returns the payload valid accepts. An abort rejects
// with the AbortError itself; any other failure rejects with a RequestError
// whose message describe phrases.
export async function fetchJSON<T>(
  url: string,
  options: FetchJSONOptions,
  valid: (value: unknown) => value is T,
  describe: (failure: RequestFailure) => string,
): Promise<T> {
  const fail = (failure: RequestFailure) => new RequestError(describe(failure), failure)
  let response: Response
  try {
    response = await fetch(url, { headers: { Accept: "application/json", ...options.headers }, signal: options.signal })
  } catch (error: unknown) {
    if (isAbort(error)) throw error
    throw fail({ kind: "unreachable" })
  }
  let payload: unknown
  try {
    payload = await response.json()
  } catch (error: unknown) {
    if (isAbort(error)) throw error
    throw fail({ kind: "malformed", status: response.status, ok: response.ok })
  }
  if ((response.ok || options.anyStatus) && valid(payload)) return payload
  const reason = serverReason(payload)
  if (reason) throw fail({ kind: "rejected", status: response.status, ...reason })
  throw fail({ kind: "invalid", status: response.status, ok: response.ok })
}
