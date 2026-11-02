import { isRecord } from "../../../web/src/api/http.ts"

// Window title band (decision 0075): the native title bar is hidden and the
// page draws a band in its theme's canvas color through env(titlebar-area-*).
// Main colors the window controls from the page's theme-color meta.

// TITLEBAR_HEIGHT is the band height in CSS pixels.
export const TITLEBAR_HEIGHT = 32

// DEFAULT_CANVAS is the Servef canvas per OS polarity (web/src/styles/tokens.css),
// used until a page reports its theme color.
export const DEFAULT_CANVAS: Readonly<{ light: string; dark: string }> = { light: "#ffffff", dark: "#0b0b0b" }

const LIGHT_SYMBOL = "#e6e6e6"
const DARK_SYMBOL = "#3b3b3b"
// macOS traffic lights are about 16px tall; center them in the band.
const TRAFFIC_LIGHT_SIZE = 16

export interface TitleBarOverlay {
  color: string
  symbolColor: string
  height: number
}

export interface WindowChrome {
  titleBarStyle: "hidden"
  titleBarOverlay: TitleBarOverlay | true
  trafficLightPosition?: { x: number; y: number }
  backgroundColor: string
}

// windowChrome is the BrowserWindow title options for platform and a band
// color: macOS keeps its traffic lights; Windows and Linux draw overlay
// controls in the band color.
export function windowChrome(platform: NodeJS.Platform, color: string): WindowChrome {
  if (platform === "darwin") {
    const inset = (TITLEBAR_HEIGHT - TRAFFIC_LIGHT_SIZE) / 2
    return { titleBarStyle: "hidden", titleBarOverlay: true, trafficLightPosition: { x: 12, y: inset }, backgroundColor: color }
  }
  return { titleBarStyle: "hidden", titleBarOverlay: overlayFor(color), backgroundColor: color }
}

// overlayFor is the Windows and Linux overlay for a band color.
export function overlayFor(color: string): TitleBarOverlay {
  return { color, symbolColor: symbolColorFor(color), height: TITLEBAR_HEIGHT }
}

// parseThemeColor accepts only an opaque #rrggbb color (pages are untrusted);
// anything else is null. It returns lowercase hex.
export function parseThemeColor(color: string | null): string | null {
  if (color === null || !/^#[0-9a-f]{6}$/i.test(color)) return null
  return color.toLowerCase()
}

// symbolColorFor picks light controls on dark bands and dark controls on
// light bands, by WCAG relative luminance.
export function symbolColorFor(color: string): string {
  return luminance(color) > 0.179 ? DARK_SYMBOL : LIGHT_SYMBOL
}

function luminance(hex: string): number {
  const channel = (offset: number): number => {
    const value = Number.parseInt(hex.slice(offset, offset + 2), 16) / 255
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * channel(1) + 0.7152 * channel(3) + 0.0722 * channel(5)
}

export interface MenuPoint {
  x: number
  y: number
}

// parseMenuPoint validates an app-menu popup point from a renderer: finite
// numbers inside the window's content size, rounded to integers.
export function parseMenuPoint(value: unknown, size: { width: number; height: number }): MenuPoint | null {
  if (!isRecord(value)) return null
  const { x, y } = value
  if (typeof x !== "number" || typeof y !== "number" || !Number.isFinite(x) || !Number.isFinite(y)) return null
  if (x < 0 || y < 0 || x > size.width || y > size.height) return null
  return { x: Math.round(x), y: Math.round(y) }
}
