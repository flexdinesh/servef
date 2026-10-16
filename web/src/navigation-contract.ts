// NavigationLocation is a definition or reference: a 1-based line and a
// 0-based UTF-16 column, as the browser counts.
export interface NavigationLocation {
  path: string
  line: number
  column: number
  endLine: number
  endColumn: number
  kind: string
  snippet: string
}

// NavigationResult answers /api/definition and /api/references.
export interface NavigationResult {
  symbol: string
  locations: NavigationLocation[]
  server: string
  filtered: number
  truncated: boolean
}

export type LanguageServerState = "missing" | "stopped" | "starting" | "ready" | "failed"

export interface LanguageServerInstall {
  label: string
  command: string
  docs?: string
  prerequisites?: string[]
}

// LanguageServerStatus describes a file family's language server. memory is
// resident bytes while running, 0 otherwise.
export interface LanguageServerStatus {
  name: string
  language: string
  state: LanguageServerState
  message: string
  install: LanguageServerInstall[]
  docs: string
  memory: number
  idleMinutes: number
  capabilities: { definition: boolean; references: boolean } | null
}

export interface NavigationStatus {
  supported: boolean
  enabled: boolean
  eligible: boolean
  reason: string
  server: LanguageServerStatus | null
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

function isPosition(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0
}

export function isNavigationLocation(value: unknown): value is NavigationLocation {
  return isRecord(value)
    && typeof value.path === "string"
    && isPosition(value.line) && value.line > 0
    && isPosition(value.column)
    && isPosition(value.endLine) && value.endLine >= value.line
    && isPosition(value.endColumn)
    && (value.endLine > value.line || value.endColumn >= value.column)
    && typeof value.kind === "string"
    && typeof value.snippet === "string"
}

export function isNavigationResult(value: unknown): value is NavigationResult {
  return isRecord(value)
    && typeof value.symbol === "string"
    && Array.isArray(value.locations)
    && value.locations.every(isNavigationLocation)
    && typeof value.server === "string"
    && isPosition(value.filtered)
    && typeof value.truncated === "boolean"
}

function isServerState(value: unknown): value is LanguageServerState {
  return value === "missing" || value === "stopped" || value === "starting" || value === "ready" || value === "failed"
}

function isInstall(value: unknown): value is LanguageServerInstall {
  return isRecord(value) && typeof value.label === "string" && typeof value.command === "string"
    && (value.docs === undefined || typeof value.docs === "string")
    && (value.prerequisites === undefined || (Array.isArray(value.prerequisites) && value.prerequisites.every((item: unknown) => typeof item === "string")))
}

function isServerStatus(value: unknown): value is LanguageServerStatus {
  return isRecord(value)
    && typeof value.name === "string"
    && typeof value.language === "string"
    && isServerState(value.state)
    && typeof value.message === "string"
    && Array.isArray(value.install)
    && value.install.every(isInstall)
    && typeof value.docs === "string"
    && isPosition(value.memory)
    && isPosition(value.idleMinutes)
    && (value.capabilities === null || (isRecord(value.capabilities)
      && typeof value.capabilities.definition === "boolean"
      && typeof value.capabilities.references === "boolean"))
}

export function isNavigationStatus(value: unknown): value is NavigationStatus {
  return isRecord(value)
    && typeof value.supported === "boolean"
    && typeof value.enabled === "boolean"
    && typeof value.eligible === "boolean"
    && typeof value.reason === "string"
    && (value.server === null || isServerStatus(value.server))
}
