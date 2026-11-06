import { useEffect, useLayoutEffect, useState } from "react"

import { desktopBridge } from "../lib/desktop-bridge.ts"
import type { WorkspaceTab } from "./workspace-tabs.ts"

// The native menu owns Cmd/Ctrl+W. Close through the workspace so routing,
// scroll cleanup, and remembered tabs follow the same path as the tab's X.
export function useDesktopCloseTab(active: WorkspaceTab | null, tabs: readonly WorkspaceTab[], closeTab: (tab: WorkspaceTab) => void) {
  const [closingWindow, setClosingWindow] = useState(false)

  useLayoutEffect(() => {
    if (!desktopBridge()?.closeWindow) return
    function close(event: Event) {
      event.preventDefault()
      const tab = active ?? tabs.at(-1)
      if (tab) closeTab(tab)
      if (tabs.length <= 1) setClosingWindow(true)
    }
    window.addEventListener("srcz:close-tab", close)
    return () => window.removeEventListener("srcz:close-tab", close)
  }, [active, closeTab, tabs])

  // Let navigation and the project's memory effect save the empty workspace
  // before closing the native window. A tab opened meanwhile keeps it alive.
  useEffect(() => {
    if (closingWindow && !active && tabs.length === 0) desktopBridge()?.closeWindow?.()
    else if (closingWindow && tabs.length > 0) setClosingWindow(false)
  }, [active, closingWindow, tabs.length])
}
