export const THEME_STORAGE_KEY = "servef-theme"

export type Theme = "system" | "light" | "dark" | "dracula" | "gruvbox-dark" | "catppuccin-mocha" | "nord" | "solarized-light"
export type ResolvedTheme = "light" | "dark"

export interface ThemeSnapshot {
  theme: Theme
  resolvedTheme: ResolvedTheme
}

export const DEFAULT_THEME: Theme = "system"

export interface ThemeOption {
  label: string
  value: Theme
}

export const themeOptions: readonly ThemeOption[] = [
  { label: "System", value: "system" },
  { label: "Light", value: "light" },
  { label: "Dark", value: "dark" },
  { label: "Catppuccin Mocha", value: "catppuccin-mocha" },
  { label: "Dracula", value: "dracula" },
  { label: "Gruvbox Dark", value: "gruvbox-dark" },
  { label: "Nord", value: "nord" },
  { label: "Solarized Light", value: "solarized-light" },
]

export function isTheme(value: unknown): value is Theme {
  return value === "system"
    || value === "light"
    || value === "dark"
    || value === "dracula"
    || value === "gruvbox-dark"
    || value === "catppuccin-mocha"
    || value === "nord"
    || value === "solarized-light"
}

function isResolvedTheme(value: unknown): value is ResolvedTheme {
  return value === "light" || value === "dark"
}

export function parseTheme(value: string | null): Theme {
  return isTheme(value) ? value : DEFAULT_THEME
}

export function resolveTheme(theme: Theme, prefersDark: boolean): ResolvedTheme {
  if (theme === "system") return prefersDark ? "dark" : "light"
  if (theme === "light" || theme === "solarized-light") return "light"
  return "dark"
}

export function readStoredTheme(): Theme {
  try {
    return parseTheme(window.localStorage.getItem(THEME_STORAGE_KEY))
  } catch {
    return DEFAULT_THEME
  }
}

export function storeTheme(theme: Theme): void {
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, theme)
  } catch {
    // Theme selection still applies when storage is unavailable.
  }
}

function themeColorScheme(meta: HTMLMetaElement): ResolvedTheme | null {
  const markedScheme = meta.dataset.servefThemeColor
  if (isResolvedTheme(markedScheme)) return markedScheme

  const media = meta.media.toLowerCase()
  const scheme = media.includes("prefers-color-scheme: dark")
    ? "dark"
    : media.includes("prefers-color-scheme: light")
      ? "light"
      : null
  if (scheme) meta.dataset.servefThemeColor = scheme
  return scheme
}

export function applyResolvedTheme(resolvedTheme: ResolvedTheme): void {
  const root = document.documentElement
  root.classList.toggle("light", resolvedTheme === "light")
  root.classList.toggle("dark", resolvedTheme === "dark")
  root.style.colorScheme = resolvedTheme

  const themeColors = document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')
  for (const meta of themeColors) {
    const scheme = themeColorScheme(meta)
    if (scheme) meta.media = scheme === resolvedTheme ? "all" : "not all"
  }
}

export function applyTheme(theme: Theme, resolvedTheme: ResolvedTheme): void {
  document.documentElement.dataset.theme = theme
  applyResolvedTheme(resolvedTheme)
}

export function initializeTheme(): ThemeSnapshot {
  const theme = readStoredTheme()
  const resolvedTheme = resolveTheme(theme, window.matchMedia("(prefers-color-scheme: dark)").matches)
  applyTheme(theme, resolvedTheme)
  return { theme, resolvedTheme }
}
