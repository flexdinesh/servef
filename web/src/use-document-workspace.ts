import { useCallback, useEffect, useState } from "react"

import type { DocumentMode } from "./DocumentModeToggle.tsx"
import type { PageData } from "./page-data.ts"

interface DocumentWorkspace {
  tabs: string[]
  modes: ReadonlyMap<string, DocumentMode>
}

export function useDocumentWorkspace(page: PageData | null) {
  const selected = page?.hasFile ? page.selected : ""
  const [workspace, setWorkspace] = useState<DocumentWorkspace>(() => ({
    tabs: selected ? [selected] : [],
    modes: new Map(),
  }))

  useEffect(() => {
    if (!selected) return
    setWorkspace((current) => current.tabs.includes(selected)
      ? current
      : { ...current, tabs: [...current.tabs, selected] })
  }, [selected])

  const setDocumentMode = useCallback((path: string, mode: DocumentMode) => {
    setWorkspace((current) => {
      if (current.modes.get(path) === mode) return current
      const modes = new Map(current.modes)
      modes.set(path, mode)
      return { ...current, modes }
    })
  }, [])

  // restore opens remembered tabs ahead of the ones opened this session.
  const restore = useCallback((tabs: readonly string[]) => {
    setWorkspace((current) => {
      const merged = [...tabs, ...current.tabs.filter((tab) => !tabs.includes(tab))]
      return merged.length === current.tabs.length && merged.every((tab, index) => tab === current.tabs[index])
        ? current
        : { ...current, tabs: merged }
    })
  }, [])

  function closeDocument(path: string) {
    const index = workspace.tabs.indexOf(path)
    if (index < 0) return null
    const remaining = workspace.tabs.filter((tab) => tab !== path)
    setWorkspace((current) => {
      const modes = new Map(current.modes)
      modes.delete(path)
      return { tabs: current.tabs.filter((tab) => tab !== path), modes }
    })
    return { replacement: remaining[Math.min(index, remaining.length - 1)] }
  }

  return { tabs: workspace.tabs, modes: workspace.modes, setDocumentMode, closeDocument, restore }
}
