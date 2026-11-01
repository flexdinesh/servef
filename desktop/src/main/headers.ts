import type { IncomingHttpHeaders } from "node:http"

// Request header policy for the servef-app:// proxy.
export const FORWARDED_REQUEST_HEADERS: readonly string[] = [
  "accept", "accept-language", "content-type", "content-length", "range", "if-none-match",
  "if-modified-since", "last-event-id", "cache-control", "x-servef-language-servers",
]

export const HOP_BY_HOP: ReadonlySet<string> = new Set(["connection", "keep-alive", "transfer-encoding", "upgrade"])

// upstreamHeaders keeps the allowlist and sets Host: localhost (or
// localhost:<port> for a CLI server on port) and Sec-Fetch-Site: same-origin.
// The servef Host check refuses names other than localhost and IP literals.
export function upstreamHeaders(incoming: Headers, port: number | null = null): Record<string, string> {
  const headers: Record<string, string> = {}
  for (const name of FORWARDED_REQUEST_HEADERS) {
    const value = incoming.get(name)
    if (value !== null) headers[name] = value
  }
  headers.host = port === null ? "localhost" : `localhost:${port}`
  headers["sec-fetch-site"] = "same-origin"
  return headers
}

export function isEventStreamRequest(headers: Headers): boolean {
  return (headers.get("accept") ?? "").toLowerCase().includes("text/event-stream")
}

// responseHeaders copies a daemon response head minus hop-by-hop headers;
// array values are appended one by one.
export function responseHeaders(raw: IncomingHttpHeaders): Headers {
  const headers = new Headers()
  for (const [name, value] of Object.entries(raw)) {
    if (value === undefined || HOP_BY_HOP.has(name.toLowerCase())) continue
    for (const one of Array.isArray(value) ? value : [value]) headers.append(name, one)
  }
  return headers
}
