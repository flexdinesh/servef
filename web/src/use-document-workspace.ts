import { useCallback, useEffect, useState } from "react"

import type { DocumentMode } from "./features/documents/DocumentModeToggle.tsx"
import { includesTab, sameTab, type WorkspaceTab } from "./workspace-tabs.ts"

interface DocumentWorkspace {
  tabs: WorkspaceTab[]
  modes: ReadonlyMap<string, DocumentMode>
}

// useDocumentWorkspace owns open tabs; the routed active tab joins them.
export function useDocumentWorkspace(active: WorkspaceTab | null) {
  const [workspace, setWorkspace] = useState<DocumentWorkspace>(() => ({
    tabs: active ? [active] : [],
    modes: new Map(),
  }))
  useEffect(() => {
    if (!active) return
    setWorkspace((current) => includesTab(current.tabs, active)
      ? current
      : { ...current, tabs: [...current.tabs, active] })
  }, [active])

  const setDocumentMode = useCallback((path: string, mode: DocumentMode) => {
    setWorkspace((current) => {
      if (current.modes.get(path) === mode) return current
      const modes = new Map(current.modes)
      modes.set(path, mode)
      return { ...current, modes }
    })
  }, [])

  // restore opens remembered tabs ahead of the ones opened this session.
  const restore = useCallback((tabs: readonly WorkspaceTab[]) => {
    setWorkspace((current) => {
      const merged = [...tabs, ...current.tabs.filter((tab) => !includesTab(tabs, tab))]
      return merged.length === current.tabs.length && merged.every((tab, index) => sameTab(tab, current.tabs[index] ?? null))
        ? current
        : { ...current, tabs: merged }
    })
  }, [])

  function closeTabs(closing: readonly WorkspaceTab[], activeTab: WorkspaceTab | null) {
    const closed = workspace.tabs.filter((tab) => includesTab(closing, tab))
    if (closed.length === 0) return null
    const index = workspace.tabs.findIndex((tab) => sameTab(tab, activeTab))
    const replacement = workspace.tabs.slice(index + 1).find((tab) => !includesTab(closed, tab))
      ?? workspace.tabs.slice(0, Math.max(index, 0)).reverse().find((tab) => !includesTab(closed, tab))
    setWorkspace((current) => {
      const modes = new Map(current.modes)
      // Modes belong to documents; a document's mode outlives its diff tab.
      for (const tab of closed) if (tab.kind === "document") modes.delete(tab.path)
      return { tabs: current.tabs.filter((tab) => !includesTab(closed, tab)), modes }
    })
    return { tabs: closed, replacement: replacement ?? null }
  }

  return { tabs: workspace.tabs, modes: workspace.modes, setDocumentMode, closeTabs, restore }
}
