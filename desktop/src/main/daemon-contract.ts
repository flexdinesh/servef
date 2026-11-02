import { isRecord } from "../../../web/src/api/http.ts"

// Wire contract for the daemon control API the app consumes (decisions 0061, 0066).
export const EXPECTED_PROTOCOL = 1

export interface DaemonStatus {
  protocol: number
  version: string
  pid: number
  socket: string
  roots: string[]
  started?: boolean
}

export interface OpenResult {
  root: string
  added: boolean
  replaced: string[]
  project: string
}

// ControlStatus is /_daemon/events' first event.
export interface ControlStatus {
  protocol: number
  version: string
}

export interface WalkthroughAnnouncement {
  project: string
  id: string
  title: string
  created: boolean
}

export interface ControlErrorBody {
  error: string
  code?: string
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string")
}

export function isDaemonStatus(value: unknown): value is DaemonStatus {
  return isRecord(value)
    && Number.isInteger(value.protocol)
    && typeof value.version === "string"
    && Number.isInteger(value.pid)
    && typeof value.socket === "string"
    && isStringArray(value.roots)
    && (value.started === undefined || typeof value.started === "boolean")
}

export function isOpenResult(value: unknown): value is OpenResult {
  return isRecord(value)
    && typeof value.root === "string"
    && typeof value.added === "boolean"
    && isStringArray(value.replaced)
    && typeof value.project === "string"
}

export function isControlStatus(value: unknown): value is ControlStatus {
  return isRecord(value) && Number.isInteger(value.protocol) && typeof value.version === "string"
}

export function isWalkthroughAnnouncement(value: unknown): value is WalkthroughAnnouncement {
  return isRecord(value)
    && typeof value.project === "string"
    && typeof value.id === "string"
    && typeof value.title === "string"
    && typeof value.created === "boolean"
}

export function isControlErrorBody(value: unknown): value is ControlErrorBody {
  return isRecord(value)
    && typeof value.error === "string"
    && (value.code === undefined || typeof value.code === "string")
}
