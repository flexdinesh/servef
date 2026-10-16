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

  function closeDocuments(paths: readonly string[], activePath: string) {
    const closed = workspace.tabs.filter((tab) => paths.includes(tab))
    if (closed.length === 0) return null
    const index = workspace.tabs.indexOf(activePath)
    const replacement = workspace.tabs.slice(index + 1).find((tab) => !closed.includes(tab))
      ?? workspace.tabs.slice(0, index).reverse().find((tab) => !closed.includes(tab))
    setWorkspace((current) => {
      const modes = new Map(current.modes)
      for (const path of closed) modes.delete(path)
      return { tabs: current.tabs.filter((tab) => !closed.includes(tab)), modes }
    })
    return { paths: closed, replacement }
  }

  return { tabs: workspace.tabs, modes: workspace.modes, setDocumentMode, closeDocuments, restore }
}
