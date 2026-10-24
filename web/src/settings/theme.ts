export const THEME_STORAGE_KEY = "servef-theme"

export type Theme =
  | "system"
  | "light"
  | "dark"
  | "catppuccin"
  | "catppuccin-latte"
  | "dracula"
  | "ethereal"
  | "everforest"
  | "flexoki-light"
  | "gruvbox"
  | "hackerman"
  | "kanagawa"
  | "last-horizon"
  | "lumon"
  | "lupine"
  | "matte-black"
  | "miasma"
  | "nord"
  | "osaka-jade"
  | "retro-82"
  | "ristretto"
  | "rose-pine"
  | "solarized-light"
  | "solitude"
  | "starship"
  | "tokyo-night"
  | "vantablack"
  | "white"
export type ResolvedTheme = "light" | "dark"

export interface ThemeSnapshot {
  theme: Theme
  resolvedTheme: ResolvedTheme
}

export const DEFAULT_THEME: Theme = "system"

// scheme is the polarity a theme resolves to; System follows the browser.
export interface ThemeOption {
  label: string
  value: Theme
  scheme: ResolvedTheme | "system"
}

// Named themes other than Dracula and Solarized Light come from Omarchy's
// built-in themes (decision 0036); their values live in themes.css.
const themeDefinitions: Record<Theme, Omit<ThemeOption, "value">> = {
  "system": { label: "System", scheme: "system" },
  "light": { label: "Light", scheme: "light" },
  "dark": { label: "Dark", scheme: "dark" },
  "catppuccin": { label: "Catppuccin", scheme: "dark" },
  "catppuccin-latte": { label: "Catppuccin Latte", scheme: "light" },
  "dracula": { label: "Dracula", scheme: "dark" },
  "ethereal": { label: "Ethereal", scheme: "dark" },
  "everforest": { label: "Everforest", scheme: "dark" },
  "flexoki-light": { label: "Flexoki Light", scheme: "light" },
  "gruvbox": { label: "Gruvbox", scheme: "dark" },
  "hackerman": { label: "Hackerman", scheme: "dark" },
  "kanagawa": { label: "Kanagawa", scheme: "dark" },
  "last-horizon": { label: "Last Horizon", scheme: "dark" },
  "lumon": { label: "Lumon", scheme: "dark" },
  "lupine": { label: "Lupine", scheme: "light" },
  "matte-black": { label: "Matte Black", scheme: "dark" },
  "miasma": { label: "Miasma", scheme: "dark" },
  "nord": { label: "Nord", scheme: "dark" },
  "osaka-jade": { label: "Osaka Jade", scheme: "dark" },
  "retro-82": { label: "Retro 82", scheme: "dark" },
  "ristretto": { label: "Ristretto", scheme: "dark" },
  "rose-pine": { label: "Rose Pine", scheme: "light" },
  "solarized-light": { label: "Solarized Light", scheme: "light" },
  "solitude": { label: "Solitude", scheme: "dark" },
  "starship": { label: "Starship", scheme: "dark" },
  "tokyo-night": { label: "Tokyo Night", scheme: "dark" },
  "vantablack": { label: "Vantablack", scheme: "dark" },
  "white": { label: "White", scheme: "light" },
}

// Renamed theme ids keep stored preferences working.
const legacyThemes = new Map<string, Theme>([
  ["catppuccin-mocha", "catppuccin"],
  ["gruvbox-dark", "gruvbox"],
])

export function isTheme(value: unknown): value is Theme {
  return typeof value === "string" && Object.hasOwn(themeDefinitions, value)
}

function optionRank(value: Theme): number {
  if (value === "system") return 0
  if (value === "light") return 1
  if (value === "dark") return 2
  return 3
}

// themeOptions lists System, Light, and Dark, then named themes by label.
export const themeOptions: readonly ThemeOption[] = Object.keys(themeDefinitions)
  .filter(isTheme)
  .map((value): ThemeOption => ({ value, ...themeDefinitions[value] }))
  .toSorted((a, b) => optionRank(a.value) - optionRank(b.value) || a.label.localeCompare(b.label))

function isResolvedTheme(value: unknown): value is ResolvedTheme {
  return value === "light" || value === "dark"
}

export function parseTheme(value: string | null): Theme {
  if (isTheme(value)) return value
  return (value === null ? undefined : legacyThemes.get(value)) ?? DEFAULT_THEME
}

export function resolveTheme(theme: Theme, prefersDark: boolean): ResolvedTheme {
  const { scheme } = themeDefinitions[theme]
  if (scheme === "system") return prefersDark ? "dark" : "light"
  return scheme
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
