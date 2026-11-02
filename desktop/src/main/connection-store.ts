// Known ssh hosts and confirmed http servers in userData/connections.json
// (decisions 0070, 0076): pure parse and serialize plus an atomic file adapter.
import { createHash } from "node:crypto"
import { closeSync, fchmodSync, fsyncSync, mkdirSync, openSync, readFileSync, renameSync, unlinkSync, writeSync } from "node:fs"
import { dirname } from "node:path"
import { isRecord } from "../../../web/src/api/http.ts"
import { isLoopbackHost } from "./cli-servers.ts"
import { validDestination } from "./ssh-args.ts"

export interface KnownConnection {
  // id is ssh-<slug>, assigned once, never changed (stable origin).
  id: string
  destination: string
  port: number | null
  // addedAt is an ISO 8601 timestamp.
  addedAt: string
}

// KnownServer is an http server the user confirmed or chose (decision 0076).
export interface KnownServer {
  // id is http-<slug of host>-<port> (see assignServerId), never changed.
  id: string
  // host is the WHATWG URL hostname (IPv6 in brackets), never loopback.
  host: string
  port: number
  addedAt: string
}

export interface StoredConnections {
  ssh: KnownConnection[]
  http: KnownServer[]
}

const STORE_VERSION = 1
// ID_PATTERN admits every id assignId produces (slug ≤ 40 plus "-" and 4 hex).
const ID_PATTERN = /^ssh-[a-z0-9-]{1,45}$/
// SERVER_ID_PATTERN admits every id assignServerId produces.
const SERVER_ID_PATTERN = /^http-[a-z0-9-]{1,52}$/
const MAX_SLUG = 40

function validPort(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 1 && value <= 65_535
}

// serverHost normalizes host as an http URL hostname; null when it is not a
// valid host or names loopback (those are CLI servers, decision 0071).
export function serverHost(host: string): string | null {
  if (host === "" || /[/?#@\\\s]/.test(host)) return null
  // A colon only inside an IPv6 literal: never a port.
  if (host.includes(":") && !/^\[[^\]]*\]$/.test(host)) return null
  let url: URL
  try {
    url = new URL(`http://${host}/`)
  } catch {
    return null
  }
  if (url.port !== "" || url.hostname === "") return null
  return isLoopbackHost(url.hostname.replace(/^\[(.*)\]$/, "$1")) ? null : url.hostname
}

function parseEntry(value: unknown): KnownConnection | null {
  if (!isRecord(value)) return null
  const { id, destination, port, addedAt } = value
  if (typeof id !== "string" || !ID_PATTERN.test(id)) return null
  if (typeof destination !== "string" || !validDestination(destination)) return null
  if (port !== null && !validPort(port)) return null
  if (typeof addedAt !== "string") return null
  return { id, destination, port, addedAt }
}

function parseServer(value: unknown): KnownServer | null {
  if (!isRecord(value)) return null
  const { id, host, port, addedAt } = value
  if (typeof id !== "string" || !SERVER_ID_PATTERN.test(id)) return null
  if (typeof host !== "string" || serverHost(host) !== host) return null
  if (!validPort(port) || typeof addedAt !== "string") return null
  return { id, host, port, addedAt }
}

// parseStore returns null for malformed JSON or an unknown version; invalid
// entries, repeated ids, and repeated http host+port pairs are dropped.
export function parseStore(text: string): StoredConnections | null {
  let value: unknown
  try {
    value = JSON.parse(text)
  } catch {
    return null
  }
  if (!isRecord(value) || value.version !== STORE_VERSION || !Array.isArray(value.ssh)) return null
  const servers: unknown[] = Array.isArray(value.http) ? value.http : []
  const ids = new Set<string>()
  const ssh: KnownConnection[] = []
  for (const item of value.ssh) {
    const entry = parseEntry(item)
    if (entry === null || ids.has(entry.id)) continue
    ids.add(entry.id)
    ssh.push(entry)
  }
  const http: KnownServer[] = []
  for (const item of servers) {
    const entry = parseServer(item)
    if (entry === null || ids.has(entry.id) || http.some((other) => other.host === entry.host && other.port === entry.port)) continue
    ids.add(entry.id)
    http.push(entry)
  }
  return { ssh, http }
}

export function serializeStore(stored: StoredConnections): string {
  const ssh = stored.ssh.map((entry) => ({ id: entry.id, destination: entry.destination, port: entry.port, addedAt: entry.addedAt }))
  const http = stored.http.map((entry) => ({ id: entry.id, host: entry.host, port: entry.port, addedAt: entry.addedAt }))
  return `${JSON.stringify({ version: STORE_VERSION, ssh, http }, null, 2)}\n`
}

function slugOf(text: string): string {
  return text.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+/, "").slice(0, MAX_SLUG).replace(/-+$/, "") || "host"
}

// suffixed returns the first of base-<4 hex windows of sha256(text)>, then
// base-<counter>, that is not taken; end is appended after the suffix.
function suffixed(base: string, text: string, end: string, taken: ReadonlySet<string>): string {
  const hash = createHash("sha256").update(text).digest("hex")
  for (let offset = 0; offset + 4 <= hash.length; offset += 4) {
    const id = `${base}-${hash.slice(offset, offset + 4)}${end}`
    if (!taken.has(id)) return id
  }
  for (let counter = 2; ; counter++) {
    const id = `${base}-${counter}${end}`
    if (!taken.has(id)) return id
  }
}

// assignId returns ssh-<slug of destination> (lowercase, runs of [^a-z0-9]
// → "-", trimmed, ≤ 40 chars), adding "-" + 4 hex of sha256(destination)
// when taken. A destination without letters or digits slugs to "host"; when
// the suffixed id is taken too, later 4-hex windows of the hash are tried.
export function assignId(destination: string, taken: ReadonlySet<string>): string {
  const base = `ssh-${slugOf(destination)}`
  return taken.has(base) ? suffixed(base, destination, "", taken) : base
}

// IPV4_SLUG matches the slug of a dotted IPv4 address (no leading zeros).
const OCTET = "(?:25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
const IPV4_SLUG = new RegExp(`^${OCTET}-${OCTET}-${OCTET}-${OCTET}$`)

// assignServerId returns http-<slug of host>-<port> with assignId's slug
// rules, adding "-" + 4 hex of sha256(host) before the port when taken. Only
// an IPv4 address keeps a slug of four octets: any other host whose slug
// looks like one gets the suffix, so shareableOrigin (web) can map an IPv4
// id back to http://<address>:<port> exactly.
export function assignServerId(host: string, port: number, taken: ReadonlySet<string>): string {
  const bare = host.replace(/^\[(.*)\]$/, "$1")
  const slug = slugOf(bare)
  const ipv4 = /^\d{1,3}(?:\.\d{1,3}){3}$/.test(bare)
  const base = `http-${slug}`
  const id = `${base}-${port}`
  if (!taken.has(id) && (ipv4 || !IPV4_SLUG.test(slug))) return id
  return suffixed(base, host, `-${port}`, taken)
}

export interface ConnectionStore {
  // load reads the file; malformed or unknown version is renamed to
  // connections.json.invalid once and yields nothing (logged).
  load(): StoredConnections
  // save writes atomically (temp file 0600, fsync, rename).
  save(stored: StoredConnections): void
}

function errorCode(error: unknown): string | undefined {
  if (!isRecord(error)) return undefined
  return typeof error.code === "string" ? error.code : undefined
}

// createFileStore persists to file. An invalid file is renamed to
// `<file>.invalid` (replacing an older one), so the next load starts empty.
export function createFileStore(file: string): ConnectionStore {
  return {
    load() {
      let text: string
      try {
        text = readFileSync(file, "utf8")
      } catch (error) {
        if (errorCode(error) !== "ENOENT") console.error(`servef: cannot read ${file}: ${String(error)}`)
        return { ssh: [], http: [] }
      }
      const stored = parseStore(text)
      if (stored !== null) return stored
      const invalid = `${file}.invalid`
      try {
        renameSync(file, invalid)
        console.error(`servef: ${file} is malformed or has an unknown version; moved it to ${invalid}`)
      } catch (error) {
        console.error(`servef: ${file} is malformed or has an unknown version and could not be moved: ${String(error)}`)
      }
      return { ssh: [], http: [] }
    },
    save(stored) {
      mkdirSync(dirname(file), { recursive: true })
      const temp = `${file}.tmp-${process.pid}`
      try {
        const fd = openSync(temp, "w", 0o600)
        try {
          fchmodSync(fd, 0o600)
          writeSync(fd, serializeStore(stored))
          fsyncSync(fd)
        } finally {
          closeSync(fd)
        }
        renameSync(temp, file)
      } catch (error) {
        try {
          unlinkSync(temp)
        } catch {
          // The temp file may not exist.
        }
        throw error
      }
    },
  }
}
