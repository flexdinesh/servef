// Pure servef://ssh/ link parsing and resolution (decisions 0069, 0070).
import type { KnownConnection } from "./connection-store.ts"
import { validDestination } from "./ssh-args.ts"
import type { SshTarget } from "./ssh-connection.ts"

export interface SshHints {
  hostname?: string
  user?: string
  addr?: string
  port?: number
}

export interface SshLink {
  // host is the percent-decoded <host> segment (alias or remote hostname).
  // It may be any non-empty text (testdata/remote-links.json has spaces);
  // only resolution results must be valid destinations.
  host: string
  route: "/" | "/view"
  // query is the web query without ssh-* pairs and without the leading "?";
  // remaining pairs keep their original encoding and order. "" when none.
  query: string
  // hash includes the leading "#", or "".
  hash: string
  hints: SshHints
}

// parseSshLink parses servef://ssh/<host>/<route>?<query>#<hash>; null when invalid.
// `servef://ssh/<host>` (no trailing slash) is the "/" route. An ssh-port
// hint that is not an integer 1-65535 rejects the link; empty hints and
// unknown ssh-* pairs are dropped.
export function parseSshLink(url: URL): SshLink | null {
  if (url.protocol !== "servef:" || url.host !== "ssh" || url.username !== "" || url.password !== "") return null
  const segments = url.pathname.split("/")
  // pathname "/<host>[/<route>]" splits into ["", host, route?].
  if (segments.length < 2 || segments.length > 3 || segments[0] !== "") return null
  const route = routeOf(segments[2])
  if (route === null) return null
  const host = decodeSegment(segments[1] ?? "")
  if (host === null || host === "" || host.includes("/") || hasControl(host)) return null
  const hints: SshHints = {}
  const kept: string[] = []
  const search = url.search.startsWith("?") ? url.search.slice(1) : url.search
  for (const pair of search.split("&")) {
    if (pair === "") continue
    const entry = [...new URLSearchParams(pair)][0]
    if (entry === undefined) continue
    const [key, value] = entry
    if (!key.startsWith("ssh-")) {
      kept.push(pair)
      continue
    }
    if (value === "" || hasControl(value)) continue
    if (key === "ssh-hostname") hints.hostname ??= value
    else if (key === "ssh-user") hints.user ??= value
    else if (key === "ssh-addr") hints.addr ??= value
    else if (key === "ssh-port") {
      const port = parsePort(value)
      if (port === null) return null
      hints.port ??= port
    }
  }
  return { host, route, query: kept.join("&"), hash: url.hash, hints }
}

function routeOf(segment: string | undefined): "/" | "/view" | null {
  if (segment === undefined || segment === "") return "/"
  if (segment === "view") return "/view"
  return null
}

function decodeSegment(segment: string): string | null {
  try {
    return decodeURIComponent(segment)
  } catch {
    return null
  }
}

function hasControl(text: string): boolean {
  for (const char of text) {
    const code = char.codePointAt(0) ?? 0
    if (code < 0x20 || (code >= 0x7f && code < 0xa0)) return true
  }
  return false
}

function parsePort(text: string): number | null {
  if (!/^\d{1,5}$/.test(text)) return null
  const port = Number(text)
  return port >= 1 && port <= 65_535 ? port : null
}

export interface ResolvedAlias {
  alias: string
  // hostname, user, and port are `ssh -G` values.
  hostname: string
  user: string
  port: number
}

export type SshResolution =
  | { kind: "known"; id: string }
  | { kind: "ask"; candidates: SshTarget[]; fallbacks: SshTarget[] }

const MAX_CHOICES = 4
const DEFAULT_PORT = 22

function same(a: string | undefined, b: string | undefined): boolean {
  return a !== undefined && b !== undefined && a.toLowerCase() === b.toLowerCase()
}

// hintPort is the link's ssh port: explicit, else 22 when the link carries
// an address (the CLI omits the default port), else unknown.
function hintPort(hints: SshHints): number | undefined {
  if (hints.port !== undefined) return hints.port
  return hints.addr === undefined ? undefined : DEFAULT_PORT
}

// resolveSshLink applies the plan's resolution steps 1-4. Candidate and
// fallback targets have id "" (assigned on confirm) and valid destinations;
// at most 4 of each.
//
// 1. A known connection whose destination, id slug, or remembered hint
//    hostname equals the host (case-insensitive) is used without asking.
// 2. A known connection with a remembered hint matching the link's hints
//    (hostname and user, or addr, port, and user) is used without asking.
// 3. Candidates: config aliases equal to the host, then aliases whose `ssh -G`
//    hostname equals the host, the hostname hint, or the addr hint, unless
//    their user or port differs from the link's.
// 4. Fallbacks: user@addr, then user@hostname, with the link's port.
export function resolveSshLink(link: SshLink, known: readonly KnownConnection[], aliases: readonly ResolvedAlias[]): SshResolution {
  const host = link.host
  const hints = link.hints
  const byDestination = known.find((entry) => same(entry.destination, host))
    ?? known.find((entry) => same(entry.id.replace(/^ssh-/, ""), host))
    ?? known.find((entry) => entry.hints.some((hint) => same(hint.hostname, host)))
  if (byDestination !== undefined) return { kind: "known", id: byDestination.id }

  const port = hintPort(hints)
  const byHints = known.find((entry) => entry.hints.some((hint) => {
    if (hints.user === undefined || hint.user !== hints.user) return false
    if (same(hint.hostname, hints.hostname)) return true
    return same(hint.addr, hints.addr) && (hint.port ?? DEFAULT_PORT) === (port ?? DEFAULT_PORT)
  }))
  if (byHints !== undefined) return { kind: "known", id: byHints.id }

  const candidates: SshTarget[] = []
  const addTo = (list: SshTarget[], destination: string, targetPort: number | null): void => {
    if (list.length >= MAX_CHOICES || !validDestination(destination)) return
    if (list.some((target) => target.destination === destination && target.port === targetPort)) return
    list.push({ id: "", destination, port: targetPort })
  }
  for (const alias of aliases) if (same(alias.alias, host)) addTo(candidates, alias.alias, null)
  for (const alias of aliases) {
    const hostMatch = same(alias.hostname, host) || same(alias.hostname, hints.hostname) || same(alias.hostname, hints.addr)
    if (!hostMatch) continue
    if (hints.user !== undefined && alias.user !== hints.user) continue
    if (port !== undefined && alias.port !== port) continue
    addTo(candidates, alias.alias, null)
  }

  const fallbacks: SshTarget[] = []
  if (hints.user !== undefined) {
    const fallbackPort = hints.port ?? null
    if (hints.addr !== undefined) addTo(fallbacks, `${hints.user}@${hints.addr}`, fallbackPort)
    if (hints.hostname !== undefined) addTo(fallbacks, `${hints.user}@${hints.hostname}`, fallbackPort)
  }
  return { kind: "ask", candidates, fallbacks }
}

// sshTargetURL is the servef-app://<id><route>?<query><hash> URL a link opens.
export function sshTargetURL(id: string, link: SshLink): string {
  return `servef-app://${id}${link.route}${link.query === "" ? "" : `?${link.query}`}${link.hash}`
}
