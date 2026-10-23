import { createRoot } from "react-dom/client"
import { RouterProvider } from "@tanstack/react-router"
import "@fontsource-variable/inter"

import { SettingsProvider } from "./settings/SettingsProvider.tsx"
import { ThemeProvider } from "./settings/ThemeProvider.tsx"
import { router } from "./router.tsx"
import { initializeSettings } from "./settings/settings.ts"
import { initializeTheme } from "./settings/theme.ts"
import "./style.css"

const initialTheme = initializeTheme()
const initialSettings = initializeSettings()
const root = document.querySelector("#root")
if (root) {
  createRoot(root).render(
    <ThemeProvider initialTheme={initialTheme}>
      <SettingsProvider initialSettings={initialSettings}>
        <RouterProvider router={router} />
      </SettingsProvider>
    </ThemeProvider>,
  )
}
