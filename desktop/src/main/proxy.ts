import { APP_HOST, connectionOrigin } from "./connections.ts"
import type { Connection, ConnectionLookup } from "./connections.ts"
import { isDaemonGone } from "./forward.ts"
import type { Forward, ForwardRequest } from "./forward.ts"
import { isEventStreamRequest, upstreamHeaders } from "./headers.ts"
import { failureInstructions, sshFailureInstructions } from "./instructions.ts"
import { failurePage, jsonError, reconnectingPage, STOP_AND_RETRY_PATH, unavailablePage } from "./pages.ts"

export interface ProxyDeps {
  // packaged selects failure instructions (reinstall vs dev build).
  packaged: boolean
  // release is the app's servef release for ssh instructions; null in dev builds.
  release: string | null
  lookup(id: string): Promise<ConnectionLookup>
  forward: Forward
  reensure(id: string): Promise<boolean>
  // stopAndRetry stops a mismatched daemon and ensures again.
  stopAndRetry(id: string): Promise<boolean>
  // appPages serves servef-app://app (the Connect page); no lookup, no upstream.
  appPages(request: Request): Promise<Response>
}

// createProxy returns the servef-app:// protocol handler.
export function createProxy(deps: ProxyDeps): (request: Request) => Promise<Response> {
  const failed = (lookup: Extract<ConnectionLookup, { kind: "failed" }>): Response => lookup.connection === "daemon"
    ? failurePage(failureInstructions(lookup.failure, deps.packaged))
    : failurePage(sshFailureInstructions(lookup.failure, deps.release))

  return async (request) => {
    const url = new URL(request.url)
    const id = url.host
    // Cross-connection guard: the renderer omits Origin on same-origin GETs.
    const origin = request.headers.get("origin")
    if (origin !== null && origin !== connectionOrigin(id)) return jsonError(403, "crossOrigin", "Cross-origin requests are refused.")
    if (id === APP_HOST) return deps.appPages(request)

    let pathname: string
    try {
      pathname = decodeURIComponent(url.pathname)
    } catch {
      return jsonError(400, "invalidPath", "The request path is not valid percent-encoding.")
    }
    // Repeated leading slashes still name the control API.
    pathname = pathname.replace(/^\/+/, "/")
    if (pathname.startsWith("/_daemon")) return jsonError(403, "forbidden", "The daemon control API is not reachable from the app.")

    // Unknown ids (including unconfirmed ssh-* origins) reach nothing.
    let lookup = await deps.lookup(id)
    if (lookup.kind === "unknown") return jsonError(404, "unknownConnection", "Unknown servef connection.")

    if (pathname.startsWith("/_desktop")) {
      if (pathname === STOP_AND_RETRY_PATH && request.method === "POST" && lookup.kind === "failed" && lookup.failure.kind === "mismatch") {
        await deps.stopAndRetry(id)
        return new Response(null, { status: 303, headers: { location: "/", "cache-control": "no-store" } })
      }
      return jsonError(404, "notFound", "Unknown app endpoint.")
    }

    const document = isDocumentRequest(request.headers, pathname)
    if (lookup.kind === "failed" && document) {
      // A reload of the failure page retries; a reconnecting page does not
      // bypass the backoff.
      await deps.reensure(id)
      lookup = await deps.lookup(id)
    }
    if (lookup.kind === "unknown") return jsonError(404, "unknownConnection", "Unknown servef connection.")
    if (lookup.kind === "connecting") {
      if (document) return reconnectingPage(`Reconnecting to ${lookup.label}`, lookup.failure.detail)
      return jsonError(503, "daemonUnavailable", lookup.failure.detail)
    }
    if (lookup.kind === "failed") {
      if (document) return failed(lookup)
      return jsonError(503, "daemonUnavailable", lookup.failure.detail)
    }

    const readOnly = request.method === "GET" || request.method === "HEAD"
    const toForward = (connection: Connection): ForwardRequest => ({
      socket: connection.socket,
      method: request.method,
      path: url.pathname + url.search,
      headers: upstreamHeaders(request.headers),
      body: readOnly ? null : request.body,
      stream: isEventStreamRequest(request.headers),
      signal: request.signal,
    })
    const unavailable = (detail: string): Response => document
      ? unavailablePage(detail)
      : jsonError(503, "daemonUnavailable", detail)

    try {
      return await deps.forward(toForward(lookup.connection))
    } catch (error) {
      if (!isDaemonGone(error)) return unavailable(errorMessage(error))
    }
    if (!await deps.reensure(id)) return unavailable("The servef daemon stopped and could not be restarted.")
    if (!readOnly) return jsonError(503, "daemonRestarted", "The servef daemon restarted; the request was not sent. Try again.")
    const retry = await deps.lookup(id)
    if (retry.kind !== "ready") return unavailable("The servef daemon stopped and could not be restarted.")
    try {
      return await deps.forward(toForward(retry.connection))
    } catch (error) {
      return unavailable(errorMessage(error))
    }
  }
}

function isDocumentRequest(headers: Headers, pathname: string): boolean {
  if (pathname.startsWith("/api/")) return false
  return headers.get("sec-fetch-dest") === "document" || (headers.get("accept") ?? "").includes("text/html")
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
