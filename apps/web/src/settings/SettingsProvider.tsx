import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react"

import {
  applySettings,
  defaultSettings,
  initializeSettings,
  storeSettings,
  type SettingKey,
  type Settings,
} from "./settings.ts"

interface SettingsContextValue {
  settings: Settings
  setSetting<K extends SettingKey>(key: K, value: Settings[K]): void
  resetSetting(key: SettingKey): void
  resetAll(): void
}

interface SettingsProviderProps {
  children: ReactNode
  initialSettings?: Settings
}

const SettingsContext = createContext<SettingsContextValue | undefined>(undefined)

export function SettingsProvider({ children, initialSettings }: SettingsProviderProps) {
  const [settings, setSettings] = useState<Settings>(() => initialSettings ?? initializeSettings())

  const commit = useCallback((next: Settings) => {
    storeSettings(next)
    applySettings(next)
    setSettings(next)
  }, [])

  const setSetting = useCallback(<K extends SettingKey>(key: K, value: Settings[K]) => {
    commit({ ...settings, [key]: value })
  }, [commit, settings])

  const resetSetting = useCallback((key: SettingKey) => {
    commit({ ...settings, [key]: defaultSettings[key] })
  }, [commit, settings])

  const resetAll = useCallback(() => commit(defaultSettings), [commit])

  const value = useMemo<SettingsContextValue>(
    () => ({ settings, setSetting, resetSetting, resetAll }),
    [settings, setSetting, resetSetting, resetAll],
  )

  return <SettingsContext value={value}>{children}</SettingsContext>
}

export function useSettings(): SettingsContextValue {
  const context = useContext(SettingsContext)
  if (!context) throw new Error("useSettings must be used within SettingsProvider")
  return context
}
