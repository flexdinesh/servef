import { themeOptions } from "./theme.ts"

export const SETTINGS_STORAGE_KEY = "servef-settings"

export interface Settings {
  fontSize: number
  languageServers: LanguageServerSetting
  showWelcome: ShowWelcomeSetting
}

// ShowWelcomeSetting opens the Welcome tab when a project opens with no tabs
// (decision 0052).
export type ShowWelcomeSetting = "off" | "on"

export const showWelcomeOptions: readonly { label: string; value: ShowWelcomeSetting }[] = [
  { label: "Off", value: "off" },
  { label: "On", value: "on" },
]

export function isShowWelcomeSetting(value: unknown): value is ShowWelcomeSetting {
  return value === "off" || value === "on"
}

// LanguageServerSetting opts code navigation into installed language
// servers. Off by default: servers cost hundreds of MB while running.
export type LanguageServerSetting = "off" | "on"

export const languageServerOptions: readonly { label: string; value: LanguageServerSetting }[] = [
  { label: "Off", value: "off" },
  { label: "On", value: "on" },
]

export function isLanguageServerSetting(value: unknown): value is LanguageServerSetting {
  return value === "off" || value === "on"
}

// SettingKey names values stored under SETTINGS_STORAGE_KEY. Theme is a
// setting too but keeps its own key (see theme.ts).
export type SettingKey = keyof Settings
export type SettingId = SettingKey | "theme"

const storedKeys: readonly SettingKey[] = ["fontSize", "languageServers", "showWelcome"]

// The UI type scale is authored for a 13px base: explorer, tabs, and source
// text are 0.8125rem, so 1rem is 16px at the default.
export const DEFAULT_FONT_SIZE = 13
export const FONT_SIZE_MIN = 10
export const FONT_SIZE_MAX = 20

export const defaultSettings: Settings = { fontSize: DEFAULT_FONT_SIZE, languageServers: "on", showWelcome: "on" }

interface SettingBase {
  id: SettingId
  category: string
  title: string
  description: string
}

export interface NumberSettingDefinition extends SettingBase {
  kind: "number"
  min: number
  max: number
  unit: string
}

export interface SelectSettingDefinition extends SettingBase {
  kind: "select"
  options: readonly { label: string; value: string }[]
}

export type SettingDefinition = NumberSettingDefinition | SelectSettingDefinition

export const settingDefinitions: readonly SettingDefinition[] = [
  {
    id: "theme",
    kind: "select",
    category: "Appearance",
    title: "Theme",
    description: "Color theme for the interface, code, and diagrams. Auto follows the operating system's light or dark mode.",
    options: themeOptions,
  },
  {
    id: "fontSize",
    kind: "number",
    category: "Appearance",
    title: "Font Size",
    description: "Base size for explorer, tab, and source text. Preview, status bar, and spacing scale with it.",
    min: FONT_SIZE_MIN,
    max: FONT_SIZE_MAX,
    unit: "px",
  },
  {
    id: "languageServers",
    kind: "select",
    category: "Code Navigation",
    title: "Language Servers",
    description: "Use installed language servers, such as gopls and typescript-language-server, for go to definition and find references. Each starts on first use, can take hundreds of MB, and stops after 10 idle minutes. Off disables code navigation.",
    options: languageServerOptions,
  },
  {
    id: "showWelcome",
    kind: "select",
    category: "Workbench",
    title: "Show Welcome on Startup",
    description: "Open the Welcome tab when a project opens with no tabs to restore. Off opens the project picker on first launch instead.",
    options: showWelcomeOptions,
  },
]

// parseSettingValue accepts whole numbers, or numeric text, within the
// definition's range.
export function parseSettingValue(definition: Pick<NumberSettingDefinition, "min" | "max">, value: unknown): number | null {
  const number = typeof value === "string" && value.trim() !== "" ? Number(value) : value
  return typeof number === "number" && Number.isInteger(number) && number >= definition.min && number <= definition.max
    ? number
    : null
}

export function parseFontSize(value: unknown): number | null {
  return parseSettingValue({ min: FONT_SIZE_MIN, max: FONT_SIZE_MAX }, value)
}

// parseSettings keeps each valid stored value and defaults the rest.
export function parseSettings(raw: string | null): Settings {
  let data: unknown = null
  try {
    data = raw === null ? null : JSON.parse(raw)
  } catch {
    return defaultSettings
  }
  const stored = typeof data === "object" && data !== null && !Array.isArray(data) ? data : {}
  const fontSize = "fontSize" in stored ? parseFontSize(stored.fontSize) : null
  const languageServers = "languageServers" in stored && isLanguageServerSetting(stored.languageServers)
    ? stored.languageServers
    : defaultSettings.languageServers
  const showWelcome = "showWelcome" in stored && isShowWelcomeSetting(stored.showWelcome)
    ? stored.showWelcome
    : defaultSettings.showWelcome
  return { fontSize: fontSize ?? DEFAULT_FONT_SIZE, languageServers, showWelcome }
}

export function modifiedSettings(settings: Settings): SettingKey[] {
  return storedKeys.filter((key) => settings[key] !== defaultSettings[key])
}

// uiScale is the factor applied to px-sized widgets that cannot use rem.
export function uiScale(settings: Settings): number {
  return settings.fontSize / DEFAULT_FONT_SIZE
}

export function matchSettings(query: string): readonly SettingDefinition[] {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
  return settingDefinitions.filter((definition) => {
    const options = definition.kind === "select" ? definition.options.map((option) => option.label).join(" ") : ""
    const text = `${definition.category} ${definition.title} ${definition.description} ${options}`.toLowerCase()
    return terms.every((term) => text.includes(term))
  })
}

export function readStoredSettings(): Settings {
  try {
    return parseSettings(window.localStorage.getItem(SETTINGS_STORAGE_KEY))
  } catch {
    return defaultSettings
  }
}

// storeSettings keeps only values that differ from the defaults.
export function storeSettings(settings: Settings): void {
  try {
    const modified = modifiedSettings(settings)
    if (modified.length === 0) {
      window.localStorage.removeItem(SETTINGS_STORAGE_KEY)
      return
    }
    window.localStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(
      Object.fromEntries(modified.map((key) => [key, settings[key]])),
    ))
  } catch {
    // Settings still apply when storage is unavailable.
  }
}

export function applySettings(settings: Settings): void {
  document.documentElement.style.setProperty("--font-size-base", `${settings.fontSize}px`)
}

export function initializeSettings(): Settings {
  const settings = readStoredSettings()
  applySettings(settings)
  return settings
}
