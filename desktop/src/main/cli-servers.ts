// CLI servers on this computer (decision 0071): `servef control list --json`
// reports the running registered servers; the app never reads the registry
// or its tokens.
import type { ExecFile } from "./daemon.ts"

export interface CliServer {
  // host is the registered --host; origin is the address servef dials.
  host: string
  port: number
  origin: string
}

export type CliList = { ok: true; servers: CliServer[] } | { ok: false; detail: string }

export const CLI_PREFIX = "cli-"
// CLI_LIST_TIMEOUT_MS bounds the command (its own checks take at most 2 s).
export const CLI_LIST_TIMEOUT_MS = 5_000

const MAX_DETAIL = 2_000

// listCliServers runs `servef control list --json` and keeps the servers that
// listen on loopback (see dialHost), one per port, in listed order.
export async function listCliServers(binary: string, run: ExecFile): Promise<CliList> {
  const outcome = await run(binary, ["control", "list", "--json"], { timeoutMs: CLI_LIST_TIMEOUT_MS })
  if (outcome.error?.timedOut) return { ok: false, detail: `servef control list timed out after ${CLI_LIST_TIMEOUT_MS / 1000} s.` }
  if (outcome.error !== undefined) return { ok: false, detail: `Cannot run ${binary}: ${outcome.error.message}` }
  if (outcome.code !== 0) {
    const stderr = outcome.stderr.trim()
    const detail = stderr === "" ? `servef control list exited with code ${outcome.code ?? "unknown"}.` : stderr
    return { ok: false, detail: detail.length > MAX_DETAIL ? `…${detail.slice(-MAX_DETAIL)}` : detail }
  }
  const servers = parseCliList(outcome.stdout)
  return servers === null ? { ok: false, detail: "servef control list printed an unexpected answer." } : { ok: true, servers }
}

// parseServerList validates `servef control list --json` output,
// {"servers":[{host, port, origin}]} (decision 0071; remote hosts, decision
// 0075); null when the shape is wrong.
export function parseServerList(stdout: string): CliServer[] | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(stdout)
  } catch {
    return null
  }
  if (!isRecord(parsed) || !Array.isArray(parsed.servers)) return null
  const servers: CliServer[] = []
  for (const entry of parsed.servers) {
    if (!isRecord(entry) || typeof entry.host !== "string" || typeof entry.origin !== "string" || !validPort(entry.port)) return null
    servers.push({ host: entry.host, port: entry.port, origin: entry.origin })
  }
  return servers
}

// parseCliList is parseServerList without entries whose origin is not
// http://<loopback>:<port> and later entries for a port already listed.
export function parseCliList(stdout: string): CliServer[] | null {
  const listed = parseServerList(stdout)
  if (listed === null) return null
  const servers: CliServer[] = []
  for (const server of listed) {
    if (dialHost(server) === null || servers.some((other) => other.port === server.port)) continue
    servers.push(server)
  }
  return servers
}

// originHost is the bare hostname (IPv6 without brackets) of the server's
// origin, which must be http://<host>:<port> with the server's port and no
// path; null otherwise.
export function originHost(server: CliServer): string | null {
  let url: URL
  try {
    url = new URL(server.origin)
  } catch {
    return null
  }
  if (url.protocol !== "http:" || url.username !== "" || url.password !== "" || url.pathname !== "/" || url.search !== "" || url.hash !== "") return null
  if (url.port !== String(server.port)) return null
  return url.hostname.replace(/^\[(.*)\]$/, "$1")
}

// dialHost is the loopback host to connect to (localhost, 127.0.0.0/8, or
// ::1) from the server's origin (see originHost); null otherwise.
export function dialHost(server: CliServer): string | null {
  const host = originHost(server)
  return host !== null && isLoopbackHost(host) ? host : null
}

// isLoopbackHost: localhost, an IPv4 address in 127.0.0.0/8, or ::1, as
// WHATWG URL parsing normalizes them.
export function isLoopbackHost(host: string): boolean {
  return host === "localhost" || host === "::1" || /^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)
}

// cliId is the connection id of the server on port: cli-<port>.
export function cliId(port: number): string {
  return `${CLI_PREFIX}${port}`
}

// cliPort parses a canonical cli-<port> id (1-65535, no leading zeros).
export function cliPort(id: string): number | null {
  const digits = /^cli-([1-9][0-9]{0,4})$/.exec(id)?.[1]
  if (digits === undefined) return null
  const port = Number(digits)
  return port <= 65535 ? port : null
}

function validPort(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 1 && value <= 65535
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}
