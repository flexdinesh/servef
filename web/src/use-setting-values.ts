import { useSettings } from "./components/SettingsProvider.tsx"
import { useTheme } from "./components/ThemeProvider.tsx"
import {
  DEFAULT_FONT_SIZE,
  defaultSettings,
  isLanguageServerSetting,
  languageServerOptions,
  modifiedSettings,
  parseFontSize,
  settingDefinitions,
  type SettingDefinition,
  type SettingId,
} from "./settings.ts"
import { DEFAULT_THEME, isTheme, themeOptions } from "./theme.ts"

export interface SettingValue {
  definition: SettingDefinition
  // value is the control's value; label is how menus and lists show it.
  value: string
  label: string
  defaultLabel: string
  modified: boolean
  // set applies a control value and reports whether it was valid.
  set(value: string): boolean
  reset(): void
}

export interface SettingValues {
  values: Record<SettingId, SettingValue>
  modifiedCount: number
  resetAll(): void
}

function definition(id: SettingId): SettingDefinition {
  const found = settingDefinitions.find((candidate) => candidate.id === id)
  if (!found) throw new Error(`Unknown setting ${id}`)
  return found
}

function languageServerLabel(value: string): string {
  return languageServerOptions.find((option) => option.value === value)?.label ?? value
}

function themeLabel(value: string): string {
  return themeOptions.find((option) => option.value === value)?.label ?? value
}

// useSettingValues joins the stored UI settings with the theme so every
// setting reads, changes, and resets the same way.
export function useSettingValues(): SettingValues {
  const { settings, setSetting, resetSetting, resetAll } = useSettings()
  const { theme, setTheme } = useTheme()
  const themeModified = theme !== DEFAULT_THEME

  return {
    values: {
      theme: {
        definition: definition("theme"),
        value: theme,
        label: themeLabel(theme),
        defaultLabel: themeLabel(DEFAULT_THEME),
        modified: themeModified,
        set(value) {
          if (!isTheme(value)) return false
          setTheme(value)
          return true
        },
        reset() { setTheme(DEFAULT_THEME) },
      },
      fontSize: {
        definition: definition("fontSize"),
        value: String(settings.fontSize),
        label: `${settings.fontSize}px`,
        defaultLabel: `${DEFAULT_FONT_SIZE}px`,
        modified: settings.fontSize !== DEFAULT_FONT_SIZE,
        set(value) {
          const size = parseFontSize(value)
          if (size === null) return false
          setSetting("fontSize", size)
          return true
        },
        reset() { resetSetting("fontSize") },
      },
      languageServers: {
        definition: definition("languageServers"),
        value: settings.languageServers,
        label: languageServerLabel(settings.languageServers),
        defaultLabel: languageServerLabel(defaultSettings.languageServers),
        modified: settings.languageServers !== defaultSettings.languageServers,
        set(value) {
          if (!isLanguageServerSetting(value)) return false
          setSetting("languageServers", value)
          return true
        },
        reset() { resetSetting("languageServers") },
      },
    },
    modifiedCount: modifiedSettings(settings).length + (themeModified ? 1 : 0),
    resetAll() {
      resetAll()
      setTheme(DEFAULT_THEME)
    },
  }
}
