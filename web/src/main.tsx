import { createRoot } from "react-dom/client"
import { RouterProvider } from "@tanstack/react-router"
import "@fontsource-variable/inter"

import { SettingsProvider } from "./components/SettingsProvider.tsx"
import { ThemeProvider } from "./components/ThemeProvider.tsx"
import { router } from "./router.tsx"
import { initializeSettings } from "./settings.ts"
import { initializeTheme } from "./theme.ts"
import "./style.css"

declare global {
  interface Window {
    EXCALIDRAW_ASSET_PATH: string
  }
}

window.EXCALIDRAW_ASSET_PATH = "/assets/excalidraw/"

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
