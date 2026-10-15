import { useCallback, useEffect, useState } from "react"

import type { DocumentMode } from "./DocumentModeToggle.tsx"
import type { PageData } from "./page-data.ts"

interface DocumentWorkspace {
  tabs: string[]
  modes: ReadonlyMap<string, DocumentMode>
  // preferPreview opens documents without a chosen view in Preview when they offer it.
  preferPreview: boolean
}

export function useDocumentWorkspace(page: PageData | null) {
  const selected = page?.hasFile ? page.selected : ""
  const [workspace, setWorkspace] = useState<DocumentWorkspace>(() => ({
    tabs: selected ? [selected] : [],
    modes: new Map(),
    preferPreview: false,
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

  // chooseDocumentMode records a user's view choice. Preview and Source also
  // set the view that documents without a chosen view open in; Format does not.
  const chooseDocumentMode = useCallback((path: string, mode: DocumentMode) => {
    setWorkspace((current) => {
      const preferPreview = mode === "format" ? current.preferPreview : mode === "preview"
      if (current.modes.get(path) === mode && preferPreview === current.preferPreview) return current
      const modes = new Map(current.modes)
      modes.set(path, mode)
      return { ...current, modes, preferPreview }
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
      return { ...current, tabs: current.tabs.filter((tab) => tab !== path), modes }
    })
    return { replacement: remaining[Math.min(index, remaining.length - 1)] }
  }

  const preferredMode: DocumentMode | undefined = workspace.preferPreview ? "preview" : undefined
  return { tabs: workspace.tabs, modes: workspace.modes, preferredMode, setDocumentMode, chooseDocumentMode, closeDocument, restore }
}
