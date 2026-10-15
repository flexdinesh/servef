import { useEffect, useMemo, useRef } from "react"

import { filePaths, type PageData } from "./page-data.ts"
import {
  projectMemory,
  readMemory,
  rememberProject,
  restoreTabs,
  updateMemory,
} from "./workspace-memory.ts"

const noRoots: readonly string[] = []

interface ProjectMemoryOptions {
  project: string
  // page is the loaded page for project, or null while loading or failed.
  page: PageData | null
  tabs: readonly string[]
  restore(tabs: readonly string[]): void
  openRemembered(path: string): void
}

// useProjectMemory restores a project's remembered tabs once its first page
// loads, checked against the fresh tree, then remembers tabs as they change.
export function useProjectMemory({ project, page, tabs, restore, openRemembered }: ProjectMemoryOptions) {
  const restored = useRef(false)
  const ready = page !== null && !page.error && page.project === project
  const roots = page?.roots ?? noRoots
  const active = page?.hasFile ? page.selected : ""
  const files = useMemo(() => new Set(page ? filePaths(page.tree) : []), [page])

  useEffect(() => {
    if (restored.current || !ready) return
    restored.current = true
    const remembered = restoreTabs(projectMemory(readMemory(), project), files)
    restore(remembered.tabs)
    if (!active && remembered.active) openRemembered(remembered.active)
  }, [active, files, openRemembered, project, ready, restore])

  useEffect(() => {
    if (!restored.current || !ready) return
    updateMemory((memory) => rememberProject(memory, roots, project, tabs, active))
  }, [active, project, ready, roots, tabs])
}
