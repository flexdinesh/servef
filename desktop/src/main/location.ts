// Server locations (decisions 0071, 0076): pure mapping of server URLs, from
// servef://<host>:<port> links and the Connect page's "Open a link" field, to
// connections. Links and pasted text are untrusted; the caller verifies cli
// ports against `servef control list` and confirms http servers.
import { cliId, isLoopbackHost } from "./cli-servers.ts"
import { serverHost } from "./connection-store.ts"

// ServerLocation is a server URL on / or /view. path is the route plus the
// query and fragment, kept verbatim.
export type ServerLocation =
  // target is servef-app://cli-<port><path>.
  | { kind: "cli"; port: number; target: string }
  // host is the URL hostname (IPv6 in brackets), never loopback.
  | { kind: "http"; host: string; port: number; path: string }

export type PastedLocation =
  | { kind: "scheme"; url: string }
  | ServerLocation
  | { kind: "invalid"; reason: string }

export const NOT_A_SERVEF_LINK = "Not a servef link."

// PASTE_LIMIT bounds a pasted link.
export const PASTE_LIMIT = 8192

// serverLocation maps an http: URL with port (1-65535) on / or /view to a
// CLI server (loopback hosts) or an http server; null otherwise.
export function serverLocation(url: URL, port: number): ServerLocation | null {
  if (url.protocol !== "http:" || url.username !== "" || url.password !== "") return null
  if (url.pathname !== "/" && url.pathname !== "/view") return null
  if (!Number.isInteger(port) || port < 1 || port > 65_535) return null
  const path = `${url.pathname}${url.search}${url.hash}`
  if (isLoopbackHost(url.hostname.replace(/^\[(.*)\]$/, "$1"))) return { kind: "cli", port, target: `servef-app://${cliId(port)}${path}` }
  const host = serverHost(url.hostname)
  return host === null ? null : { kind: "http", host, port, path }
}

// parsePastedLocation accepts servef:// links (opened as the OS handler
// would) and http://<host>[:<port>] URLs on / or /view, any query and
// fragment, no credentials. A missing http port is 80.
export function parsePastedLocation(raw: string): PastedLocation {
  const text = raw.trim()
  const invalid: PastedLocation = { kind: "invalid", reason: NOT_A_SERVEF_LINK }
  if (text === "" || text.length > PASTE_LIMIT) return invalid
  let url: URL
  try {
    url = new URL(text)
  } catch {
    return invalid
  }
  if (url.username !== "" || url.password !== "") return invalid
  if (url.protocol === "servef:") return { kind: "scheme", url: text }
  return serverLocation(url, url.port === "" ? 80 : Number(url.port)) ?? invalid
}
