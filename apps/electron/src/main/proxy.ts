import { cliPort } from "./cli-servers.ts"
import { APP_HOST, connectionOrigin, upstreamHost } from "./connections.ts"
import type { CliConnection, Connection, ConnectionLookup } from "./connections.ts"
import { isDaemonGone } from "./forward.ts"
import type { Forward, ForwardRequest, ForwardTarget } from "./forward.ts"
import { isEventStreamRequest, upstreamHeaders } from "./headers.ts"
import { failureInstructions, sshFailureInstructions } from "./instructions.ts"
import { cliStoppedPage, escapeHTML, failurePage, jsonError, noCliServerPage, reconnectingPage, serverUnreachablePage, STOP_AND_RETRY_PATH, unavailablePage } from "./pages.ts"

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
  // dropCli unregisters a CLI server that stopped answering (decision 0071).
  dropCli(id: string): void
}

// createProxy returns the servef-app:// protocol handler.
export function createProxy(deps: ProxyDeps): (request: Request) => Promise<Response> {
  // An ssh host that refused the daemon forward links its host page, which
  // lists the remote servers reachable without it (decision 0076).
  const failed = (id: string, lookup: Extract<ConnectionLookup, { kind: "failed" }>): Response => lookup.connection === "daemon"
    ? failurePage(failureInstructions(lookup.failure, deps.packaged))
    : failurePage(sshFailureInstructions(lookup.failure, deps.release), lookup.failure.kind === "forwardDenied" ? hostPageLink(id) : "")

  // forwardCli sends a request to a CLI server. One that stopped is dropped
  // and listed again (never started: the user runs it); a read retries once
  // when the list still reports it (a pooled socket outlived a restart).
  const forwardCli = async (connection: CliConnection, readOnly: boolean, document: boolean, toForward: (connection: Connection) => ForwardRequest): Promise<Response> => {
    const stopped = (): Response => document
      ? cliStoppedPage(connection.port)
      : jsonError(503, "serverStopped", `The servef server on port ${connection.port} stopped.`)
    const unavailable = (error: unknown): Response => document
      ? unavailablePage(errorMessage(error))
      : jsonError(503, "daemonUnavailable", errorMessage(error))
    try {
      return await deps.forward(toForward(connection))
    } catch (error) {
      if (!isDaemonGone(error)) return unavailable(error)
    }
    deps.dropCli(connection.id)
    if (!readOnly) return stopped()
    const retry = await deps.lookup(connection.id)
    if (retry.kind !== "ready") return stopped()
    try {
      return await deps.forward(toForward(retry.connection))
    } catch (error) {
      if (!isDaemonGone(error)) return unavailable(error)
      deps.dropCli(connection.id)
      return stopped()
    }
  }

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
    // /_servef is a CLI server's control endpoint (decision 0071).
    if (pathname.startsWith("/_servef")) return jsonError(403, "forbidden", "The servef control API is not reachable from the app.")

    const document = isDocumentRequest(request.headers, pathname)
    const port = cliPort(id)
    // Unknown ids (including unconfirmed ssh-* origins) reach nothing.
    let lookup = await deps.lookup(id)
    if (lookup.kind === "unknown") {
      if (port !== null && document) return noCliServerPage(port)
      return jsonError(404, "unknownConnection", "Unknown servef connection.")
    }

    if (pathname.startsWith("/_desktop")) {
      if (pathname === STOP_AND_RETRY_PATH && request.method === "POST" && lookup.kind === "failed" && lookup.failure.kind === "mismatch") {
        await deps.stopAndRetry(id)
        return new Response(null, { status: 303, headers: { location: "/", "cache-control": "no-store" } })
      }
      return jsonError(404, "notFound", "Unknown app endpoint.")
    }

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
      if (document) return failed(id, lookup)
      return jsonError(503, "daemonUnavailable", lookup.failure.detail)
    }

    const readOnly = request.method === "GET" || request.method === "HEAD"
    const toForward = (connection: Connection): ForwardRequest => ({
      target: targetOf(connection),
      method: request.method,
      path: url.pathname + url.search,
      headers: upstreamHeaders(request.headers, upstreamHost(connection)),
      body: readOnly ? null : request.body,
      stream: isEventStreamRequest(request.headers),
      signal: request.signal,
    })
    const unavailable = (detail: string): Response => document
      ? unavailablePage(detail)
      : jsonError(503, "daemonUnavailable", detail)

    if (lookup.connection.kind === "cli") return forwardCli(lookup.connection, readOnly, document, toForward)
    // http servers are never restarted or re-listed: the page says why.
    if (lookup.connection.kind === "http") {
      const label = lookup.connection.label
      try {
        return await deps.forward(toForward(lookup.connection))
      } catch (error) {
        return document ? serverUnreachablePage(label, errorMessage(error)) : jsonError(503, "serverUnreachable", errorMessage(error))
      }
    }
    // A forwarded remote server: a read that finds the forward gone forwards
    // the port again (the master may have reconnected) and retries once.
    if (lookup.connection.kind === "ssh-port") {
      const label = lookup.connection.label
      const unreachable = (detail: string): Response => document ? serverUnreachablePage(label, detail) : jsonError(503, "serverUnreachable", detail)
      try {
        return await deps.forward(toForward(lookup.connection))
      } catch (error) {
        if (!isDaemonGone(error) || !readOnly) return unreachable(errorMessage(error))
      }
      if (!await deps.reensure(id)) return unreachable(`The servef server at ${label} is not answering through SSH.`)
      const retry = await deps.lookup(id)
      if (retry.kind !== "ready") return unreachable(`The servef server at ${label} is not answering through SSH.`)
      try {
        return await deps.forward(toForward(retry.connection))
      } catch (error) {
        return unreachable(errorMessage(error))
      }
    }

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

function targetOf(connection: Connection): ForwardTarget {
  return connection.kind === "cli" || connection.kind === "http" ? { host: connection.host, port: connection.port } : { socket: connection.socket }
}

// hostPageLink links an ssh id's host page (an ssh-port id has none).
function hostPageLink(id: string): string {
  if (!/^ssh-[a-z0-9-]+$/.test(id)) return ""
  return `<a href="${escapeHTML(`servef-app://${APP_HOST}/ssh/${id}`)}">Servers on this host</a>`
}

function isDocumentRequest(headers: Headers, pathname: string): boolean {
  if (pathname.startsWith("/api/")) return false
  return headers.get("sec-fetch-dest") === "document" || (headers.get("accept") ?? "").includes("text/html")
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
