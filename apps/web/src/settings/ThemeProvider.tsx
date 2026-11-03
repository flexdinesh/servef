import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react"

import {
  applyTheme as applyDocumentTheme,
  initializeTheme,
  resolveTheme,
  storeTheme,
  type ResolvedTheme,
  type Theme,
  type ThemeSnapshot,
} from "./theme.ts"

// theme is the stored choice; resolvedTheme follows the applied theme, which
// is a transient preview while one is set (the Color Theme quick pick).
interface ThemeContextValue extends ThemeSnapshot {
  setTheme(theme: Theme): void
  previewTheme(theme: Theme | null): void
}

interface ThemeProviderProps {
  children: ReactNode
  initialTheme?: ThemeSnapshot
}

const ThemeContext = createContext<ThemeContextValue | undefined>(undefined)

export function ThemeProvider({ children, initialTheme }: ThemeProviderProps) {
  const [theme, setConfiguredTheme] = useState<Theme>(() => initialTheme?.theme ?? initializeTheme().theme)
  const [preview, setPreview] = useState<Theme | null>(null)
  const appliedTheme = preview ?? theme
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>(
    () => initialTheme?.resolvedTheme ?? resolveTheme(theme, window.matchMedia("(prefers-color-scheme: dark)").matches),
  )

  const applyTheme = useCallback((nextTheme: Theme, prefersDark: boolean) => {
    const nextResolvedTheme = resolveTheme(nextTheme, prefersDark)
    setResolvedTheme(nextResolvedTheme)
    applyDocumentTheme(nextTheme, nextResolvedTheme)
  }, [])

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)")
    applyTheme(appliedTheme, media.matches)
    if (appliedTheme !== "system") return

    const updateSystemTheme = (event: MediaQueryListEvent) => {
      applyTheme("system", event.matches)
    }
    media.addEventListener("change", updateSystemTheme)
    return () => { media.removeEventListener("change", updateSystemTheme) }
  }, [applyTheme, appliedTheme])

  const setTheme = useCallback((nextTheme: Theme) => {
    storeTheme(nextTheme)
    setConfiguredTheme(nextTheme)
    setPreview(null)
    applyTheme(nextTheme, window.matchMedia("(prefers-color-scheme: dark)").matches)
  }, [applyTheme])

  const previewTheme = useCallback((nextTheme: Theme | null) => {
    setPreview(nextTheme)
  }, [])

  const value = useMemo<ThemeContextValue>(
    () => ({ previewTheme, resolvedTheme, setTheme, theme }),
    [previewTheme, resolvedTheme, setTheme, theme],
  )

  return <ThemeContext value={value}>{children}</ThemeContext>
}

export function useTheme(): ThemeContextValue {
  const context = useContext(ThemeContext)
  if (!context) throw new Error("useTheme must be used within ThemeProvider")
  return context
}
