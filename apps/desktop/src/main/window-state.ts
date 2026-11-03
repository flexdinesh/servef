import { isRecord } from "../../../web/src/api/http.ts"

export interface WindowBounds {
  x?: number
  y?: number
  width: number
  height: number
}

export const DEFAULT_BOUNDS: WindowBounds = { width: 1280, height: 800 }

const MIN_SIZE = 400
const MAX_SIZE = 10_000

// parseWindowState validates userData/window-state.json; invalid → DEFAULT_BOUNDS.
// Sizes clamp to [400, 10000]; x and y are kept only as integers.
export function parseWindowState(text: string | null): WindowBounds {
  if (text === null) return { ...DEFAULT_BOUNDS }
  let value: unknown
  try {
    value = JSON.parse(text)
  } catch {
    return { ...DEFAULT_BOUNDS }
  }
  if (!isRecord(value)) return { ...DEFAULT_BOUNDS }
  const { width, height, x, y } = value
  if (typeof width !== "number" || typeof height !== "number" || !Number.isFinite(width) || !Number.isFinite(height)) {
    return { ...DEFAULT_BOUNDS }
  }
  const bounds: WindowBounds = { width: clampSize(width), height: clampSize(height) }
  if (typeof x === "number" && Number.isInteger(x)) bounds.x = x
  if (typeof y === "number" && Number.isInteger(y)) bounds.y = y
  return bounds
}

export function serializeWindowState(bounds: WindowBounds): string {
  const { x, y, width, height } = bounds
  return JSON.stringify({ x, y, width, height })
}

function clampSize(size: number): number {
  return Math.round(Math.min(MAX_SIZE, Math.max(MIN_SIZE, size)))
}
