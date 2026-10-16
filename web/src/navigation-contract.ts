// NavigationLocation is a definition or reference: a 1-based line and a
// 0-based UTF-16 column, as the browser counts.
export interface NavigationLocation {
  path: string
  line: number
  column: number
  kind: string
  snippet: string
}

// NavigationProvider is who answered: an installed language server, or
// servef's built-in syntax navigation.
export type NavigationProvider = "language-server" | "syntax"

// NavigationResult answers /api/definition and /api/references. symbol is
// empty when no identifier is at the position; server names the language
// server that answered.
export interface NavigationResult {
  symbol: string
  locations: NavigationLocation[]
  provider: NavigationProvider
  server?: string
}

export type LanguageServerState = "missing" | "stopped" | "starting" | "ready" | "failed"

export interface LanguageServerInstall {
  label: string
  command: string
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
}

// NavigationStatus answers /api/navigation-status: who answers navigation
// for a file. provider is empty when the file has no navigation; server is
// null when its family has no language server.
export interface NavigationStatus {
  navigable: boolean
  languageServers: boolean
  provider: NavigationProvider | ""
  server: LanguageServerStatus | null
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

function isPosition(value: unknown): boolean {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0
}

export function isNavigationLocation(value: unknown): value is NavigationLocation {
  return isRecord(value)
    && typeof value.path === "string"
    && isPosition(value.line)
    && isPosition(value.column)
    && typeof value.kind === "string"
    && typeof value.snippet === "string"
}

function isProvider(value: unknown): value is NavigationProvider {
  return value === "language-server" || value === "syntax"
}

export function isNavigationResult(value: unknown): value is NavigationResult {
  return isRecord(value)
    && typeof value.symbol === "string"
    && Array.isArray(value.locations)
    && value.locations.every(isNavigationLocation)
    && isProvider(value.provider)
    && (value.server === undefined || typeof value.server === "string")
}

function isServerState(value: unknown): value is LanguageServerState {
  return value === "missing" || value === "stopped" || value === "starting" || value === "ready" || value === "failed"
}

function isInstall(value: unknown): value is LanguageServerInstall {
  return isRecord(value) && typeof value.label === "string" && typeof value.command === "string"
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
}

export function isNavigationStatus(value: unknown): value is NavigationStatus {
  return isRecord(value)
    && typeof value.navigable === "boolean"
    && typeof value.languageServers === "boolean"
    && (value.provider === "" || isProvider(value.provider))
    && (value.server === null || isServerStatus(value.server))
}
