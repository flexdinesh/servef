import { MonitorIcon, MoonIcon, PaletteIcon, SunIcon } from "lucide-react"

import { isTheme, type Theme } from "../theme.ts"
import { useTheme } from "./ThemeProvider.tsx"
import { Button } from "./ui/button.tsx"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "./ui/dropdown-menu.tsx"

interface ThemeOption {
  icon: typeof SunIcon
  label: string
  value: Theme
}

const themeOptions: readonly ThemeOption[] = [
  { icon: SunIcon, label: "Light", value: "light" },
  { icon: MoonIcon, label: "Dark", value: "dark" },
  { icon: PaletteIcon, label: "Catppuccin Mocha", value: "catppuccin-mocha" },
  { icon: PaletteIcon, label: "Gruvbox Dark", value: "gruvbox-dark" },
  { icon: PaletteIcon, label: "Nord", value: "nord" },
  { icon: PaletteIcon, label: "Solarized Light", value: "solarized-light" },
  { icon: MonitorIcon, label: "System", value: "system" },
]

export function ModeToggle() {
  const { resolvedTheme, setTheme, theme } = useTheme()
  const ResolvedIcon = resolvedTheme === "dark" ? MoonIcon : SunIcon
  const themeLabel = themeOptions.find((option) => option.value === theme)?.label ?? "System"

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={(
          <Button
            className="theme-trigger"
            type="button"
            variant="outline"
            aria-label={`Theme: ${themeLabel}`}
          >
            <ResolvedIcon aria-hidden="true" />
            <span>{themeLabel}</span>
          </Button>
        )}
      />
      <DropdownMenuContent align="end" side="top">
        <DropdownMenuRadioGroup
          value={theme}
          onValueChange={(value) => {
            if (isTheme(value)) setTheme(value)
          }}
        >
          {themeOptions.map(({ icon: Icon, label, value }) => (
            <DropdownMenuRadioItem key={value} value={value}>
              <Icon aria-hidden="true" />
              {label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
