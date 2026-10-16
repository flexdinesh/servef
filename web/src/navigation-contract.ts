// NavigationLocation is a definition or reference: a 1-based line and a
// 0-based UTF-16 column, as the browser counts.
export interface NavigationLocation {
  path: string
  line: number
  column: number
  kind: string
  snippet: string
}

// NavigationResult answers /api/definition and /api/references. symbol is
// empty when no identifier is at the position.
export interface NavigationResult {
  symbol: string
  locations: NavigationLocation[]
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

export function isNavigationResult(value: unknown): value is NavigationResult {
  return isRecord(value)
    && typeof value.symbol === "string"
    && Array.isArray(value.locations)
    && value.locations.every(isNavigationLocation)
}
