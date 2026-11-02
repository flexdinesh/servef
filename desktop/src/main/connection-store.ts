// Known ssh hosts in userData/connections.json (decision 0070): pure parse
// and serialize plus an atomic file adapter.
import { createHash } from "node:crypto"
import { closeSync, fchmodSync, fsyncSync, mkdirSync, openSync, readFileSync, renameSync, unlinkSync, writeSync } from "node:fs"
import { dirname } from "node:path"
import { isRecord } from "../../../web/src/api/http.ts"
import { validDestination } from "./ssh-args.ts"
import type { SshHints } from "./ssh-links.ts"

export interface KnownConnection {
  // id is ssh-<slug>, assigned once, never changed (stable origin).
  id: string
  destination: string
  port: number | null
  hints: SshHints[]
  // addedAt is an ISO 8601 timestamp.
  addedAt: string
}

const STORE_VERSION = 1
// ID_PATTERN admits every id assignId produces (slug ≤ 40 plus "-" and 4 hex).
const ID_PATTERN = /^ssh-[a-z0-9-]{1,45}$/
const MAX_SLUG = 40
// MAX_HINTS bounds remembered hints per host.
export const MAX_HINTS = 16

function validPort(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 1 && value <= 65_535
}

function hintText(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" && value.length <= 255 ? value : undefined
}

// sanitizeHints keeps string hostname/user/addr and a valid port per hint,
// dropping non-objects and empty hints (at most MAX_HINTS).
export function sanitizeHints(value: readonly unknown[]): SshHints[] {
  const hints: SshHints[] = []
  for (const item of value) {
    if (!isRecord(item)) continue
    const hint: SshHints = {}
    const hostname = hintText(item.hostname)
    const user = hintText(item.user)
    const addr = hintText(item.addr)
    if (hostname !== undefined) hint.hostname = hostname
    if (user !== undefined) hint.user = user
    if (addr !== undefined) hint.addr = addr
    if (validPort(item.port)) hint.port = item.port
    if (Object.keys(hint).length > 0) hints.push(hint)
    if (hints.length >= MAX_HINTS) break
  }
  return hints
}

function parseEntry(value: unknown): KnownConnection | null {
  if (!isRecord(value)) return null
  const { id, destination, port, hints, addedAt } = value
  if (typeof id !== "string" || !ID_PATTERN.test(id)) return null
  if (typeof destination !== "string" || !validDestination(destination)) return null
  if (port !== null && !validPort(port)) return null
  if (!Array.isArray(hints)) return null
  if (typeof addedAt !== "string") return null
  return { id, destination, port, hints: sanitizeHints(hints), addedAt }
}

// parseStore returns null for malformed JSON or an unknown version; invalid
// entries are dropped.
export function parseStore(text: string): KnownConnection[] | null {
  let value: unknown
  try {
    value = JSON.parse(text)
  } catch {
    return null
  }
  if (!isRecord(value) || value.version !== STORE_VERSION || !Array.isArray(value.ssh)) return null
  const known: KnownConnection[] = []
  const ids = new Set<string>()
  for (const item of value.ssh) {
    const entry = parseEntry(item)
    if (entry === null || ids.has(entry.id)) continue
    ids.add(entry.id)
    known.push(entry)
  }
  return known
}

export function serializeStore(known: readonly KnownConnection[]): string {
  const ssh = known.map((entry) => ({ id: entry.id, destination: entry.destination, port: entry.port, hints: entry.hints, addedAt: entry.addedAt }))
  return `${JSON.stringify({ version: STORE_VERSION, ssh }, null, 2)}\n`
}

// assignId returns ssh-<slug of destination> (lowercase, runs of [^a-z0-9]
// → "-", trimmed, ≤ 40 chars), adding "-" + 4 hex of sha256(destination)
// when taken. A destination without letters or digits slugs to "host"; when
// the suffixed id is taken too, later 4-hex windows of the hash are tried.
export function assignId(destination: string, taken: ReadonlySet<string>): string {
  const slug = destination.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+/, "").slice(0, MAX_SLUG).replace(/-+$/, "") || "host"
  const base = `ssh-${slug}`
  if (!taken.has(base)) return base
  const hash = createHash("sha256").update(destination).digest("hex")
  for (let offset = 0; offset + 4 <= hash.length; offset += 4) {
    const id = `${base}-${hash.slice(offset, offset + 4)}`
    if (!taken.has(id)) return id
  }
  for (let counter = 2; ; counter++) {
    const id = `${base}-${counter}`
    if (!taken.has(id)) return id
  }
}

export interface ConnectionStore {
  // load reads the file; malformed or unknown version is renamed to
  // connections.json.invalid once and yields [] (logged).
  load(): KnownConnection[]
  // save writes atomically (temp file 0600, fsync, rename).
  save(known: readonly KnownConnection[]): void
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
        return []
      }
      const known = parseStore(text)
      if (known !== null) return known
      const invalid = `${file}.invalid`
      try {
        renameSync(file, invalid)
        console.error(`servef: ${file} is malformed or has an unknown version; moved it to ${invalid}`)
      } catch (error) {
        console.error(`servef: ${file} is malformed or has an unknown version and could not be moved: ${String(error)}`)
      }
      return []
    },
    save(known) {
      mkdirSync(dirname(file), { recursive: true })
      const temp = `${file}.tmp-${process.pid}`
      try {
        const fd = openSync(temp, "w", 0o600)
        try {
          fchmodSync(fd, 0o600)
          writeSync(fd, serializeStore(known))
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
