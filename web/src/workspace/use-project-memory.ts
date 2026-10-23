import { useEffect, useMemo, useState } from "react"

import { filePaths, type PageData } from "../api/page-data.ts"
import {
  projectMemory,
  readMemory,
  rememberProject,
  restoreTabs,
  updateMemory,
} from "./workspace-memory.ts"
import { includesTab, type WorkspaceTab } from "./workspace-tabs.ts"

const noRoots: readonly string[] = []

interface ProjectMemoryOptions {
  project: string
  // page is the loaded page for project, or null while loading or failed.
  page: PageData | null
  // active is the routed tab, or null on the project's home.
  active: WorkspaceTab | null
  tabs: readonly WorkspaceTab[]
  restore(tabs: readonly WorkspaceTab[]): void
  openRemembered(tab: WorkspaceTab): void
}

// useProjectMemory restores a project's remembered tabs once its first page
// loads, checked against the fresh tree, then remembers tabs as they change.
export function useProjectMemory({ project, page, active, tabs, restore, openRemembered }: ProjectMemoryOptions) {
  const [restored, setRestored] = useState<{ opening: boolean } | null>(null)
  const ready = page !== null && !page.error && page.project === project
  const roots = page?.roots ?? noRoots
  const files = useMemo(() => new Set(page ? filePaths(page.tree) : []), [page])

  useEffect(() => {
    if (restored || !ready) return
    const remembered = restoreTabs(projectMemory(readMemory(), project), files)
    restore(remembered.tabs)
    const opening = !active && remembered.active !== null
    setRestored({ opening })
    if (opening && remembered.active) openRemembered(remembered.active)
  }, [active, files, openRemembered, project, ready, restore, restored])

  useEffect(() => {
    // Restoration schedules tab state and possibly navigation. Wait for those
    // updates before saving, so mounting never erases the remembered workspace.
    if (!restored || !ready || (restored.opening && !active)) return
    if (active && !includesTab(tabs, active)) return
    if (restored.opening) setRestored({ opening: false })
    updateMemory((memory) => rememberProject(memory, roots, project, tabs, active))
  }, [active, project, ready, restored, roots, tabs])
}
