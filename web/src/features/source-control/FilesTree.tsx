import type { GitStatusEntry } from "@pierre/trees"
import { FileTree as PierreFileTree, useFileTree } from "@pierre/trees/react"
import { useCallback, useLayoutEffect, useMemo, useRef, type KeyboardEvent } from "react"

import type { FileStats } from "../../api/compare-stats-contract.ts"
import { treeCSS } from "../../lib/tree-css.ts"
import { ancestorDirectories, type CompareFile } from "./compare-files.ts"
import { statsLabel } from "./use-compare-stats.ts"

// Every Source Control folder holds changes, so folder dots add no signal;
// deleted files read as removed.
const changesTreeCSS = `${treeCSS}
[data-item-contains-git-change="true"] > [data-item-section="git"] { visibility: hidden; }
[data-item-git-status="deleted"] > [data-item-section="content"] { text-decoration: line-through; }`

// CollapseRequest asks the tree to open or close every folder; id makes a
// repeat request distinct.
export interface CollapseRequest {
  expand: boolean
  id: number
}

interface FilesTreeProps {
  // activePath is the file in view in the open review, selected here.
  activePath: string
  collapseRequest: CollapseRequest | null
  files: readonly CompareFile[]
  itemHeight: number
  label: string
  stats: ReadonlyMap<string, FileStats> | null
  onExpandedChange(expanded: boolean): void
  // onOpen reviews a file; focusDocument moves focus to the review.
  onOpen(path: string, focusDocument: boolean): void
}

// FilesTree shows one compare's changed files under compact folders, with
// status letters and line stats (decision 0035).
export function FilesTree({ activePath, collapseRequest, files, itemHeight, label, stats, onExpandedChange, onOpen }: FilesTreeProps) {
  const paths = useMemo(() => files.map((file) => file.path), [files])
  const byPath = useMemo(() => new Map(files.map((file): [string, CompareFile] => [file.path, file])), [files])
  const gitStatus = useMemo(() => files.map((file): GitStatusEntry => ({ path: file.path, status: file.status })), [files])
  const directories = useMemo(() => ancestorDirectories(paths), [paths])
  // Folders open by default; only ones the user collapsed stay closed.
  const collapsed = useRef(new Set<string>())
  const previousPaths = useRef(paths)
  const syncing = useRef(false)
  const current = useRef({ activePath, byPath, stats, onOpen })
  useLayoutEffect(() => { current.current = { activePath, byPath, stats, onOpen } }, [activePath, byPath, stats, onOpen])

  const { model } = useFileTree({
    paths,
    flattenEmptyDirectories: true,
    initialExpansion: "open",
    itemHeight,
    initialSelectedPaths: paths.includes(activePath) ? [activePath] : [],
    density: "compact",
    search: false,
    unsafeCSS: changesTreeCSS,
    icons: { set: "complete", colored: true },
    gitStatus,
    renderRowDecoration({ item }) {
      if (item.kind !== "file") return null
      const oldPath = current.current.byPath.get(item.path)?.oldPath
      const lines = statsLabel(current.current.stats?.get(item.path))
      // Pierre truncates decorations from the start, so line stats win the
      // room; the R badge says "renamed" and the title says from where.
      const text = lines || (oldPath ? `from ${oldPath}` : "")
      if (!text) return null
      return { text, title: [oldPath ? `Renamed from ${oldPath}` : "", lines ? `${lines} lines` : ""].filter(Boolean).join(", ") }
    },
    onSelectionChange(selected) {
      if (syncing.current) return
      const focused = model.getFocusedPath()
      const path = focused && selected.includes(focused) ? focused : selected.at(-1)
      if (!path || model.getItem(path)?.isDirectory()) return
      if (path !== current.current.activePath) current.current.onOpen(path, false)
    },
  })

  const syncExpanded = useCallback(() => {
    let expanded = false
    for (const path of directories) {
      const item = model.getItem(path)
      if (!item || !("isExpanded" in item)) continue
      if (item.isExpanded()) {
        collapsed.current.delete(path)
        expanded = true
      } else {
        collapsed.current.add(path)
      }
    }
    onExpandedChange(expanded || directories.length === 0)
  }, [directories, model, onExpandedChange])

  useLayoutEffect(() => model.subscribe(() => {
    if (!syncing.current) syncExpanded()
  }), [model, syncExpanded])

  useLayoutEffect(() => {
    syncing.current = true
    if (paths !== previousPaths.current) {
      model.resetPaths(paths, { initialExpandedPaths: directories.filter((path) => !collapsed.current.has(path)) })
      previousPaths.current = paths
    }
    model.setGitStatus(gitStatus)
    for (const path of model.getSelectedPaths()) {
      if (path !== activePath) model.getItem(path)?.deselect()
    }
    model.getItem(activePath)?.select()
    syncing.current = false
    syncExpanded()
  }, [activePath, directories, gitStatus, model, paths, syncExpanded])

  // Stats arrive after the list; redraw decorations once they do. Pierre
  // skips an unchanged status, so clear it first.
  useLayoutEffect(() => {
    if (!stats) return
    model.setGitStatus(undefined)
    model.setGitStatus(gitStatus)
  }, [gitStatus, model, stats])

  useLayoutEffect(() => {
    if (!collapseRequest) return
    syncing.current = true
    for (const path of directories) {
      const item = model.getItem(path)
      if (item && "expand" in item) {
        if (collapseRequest.expand) item.expand()
        else item.collapse()
      }
    }
    syncing.current = false
    syncExpanded()
  }, [collapseRequest, directories, model, syncExpanded])

  // Click and Space keep focus in the list; Enter and double-click move it to
  // the review, as in the Search view.
  const openFocused = () => {
    const focused = model.getFocusedPath()
    if (focused && !model.getItem(focused)?.isDirectory()) current.current.onOpen(focused, true)
  }

  return <PierreFileTree model={model} className="file-tree-host" aria-label={label}
    onKeyDown={(event: KeyboardEvent<HTMLElement>) => { if (event.key === "Enter") openFocused() }}
    onDoubleClick={openFocused} />
}
