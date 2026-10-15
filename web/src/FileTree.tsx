import type { FileTree as FileTreeModel } from "@pierre/trees"
import { FileTree as PierreFileTree, useFileTree, useFileTreeSearch } from "@pierre/trees/react"
import { ChevronDownIcon, FoldVerticalIcon, GitBranchIcon, GitForkIcon, SearchIcon, UnfoldVerticalIcon, XIcon } from "lucide-react"
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from "react"

import { FilenameFilter, useFilenameFilter, type FilenameFilterState } from "./FilenameFilter.tsx"
import { countFiles, filterTreeByFilename } from "./filename-filter.ts"

import { openDirectoryPaths, type PageData, type TreeNode } from "./page-data.ts"
import { projectShortcutLabel } from "./ProjectPicker.tsx"
import { useSettings } from "./components/SettingsProvider.tsx"
import { uiScale } from "./settings.ts"

function treePaths(nodes: readonly TreeNode[]): string[] {
  return nodes.flatMap((node) => node.isDir
    ? [`${node.path}/`, ...treePaths(node.children)]
    : [node.path])
}

interface FileTreeProps {
  hasData: boolean
  onNavigate(path: string): void
  onOpenProjects(): void
  // onOpenWorktrees lists the current repository's worktrees.
  onOpenWorktrees(): void
  page: PageData
  registerOpen(open: (() => void) | null): void
}

interface ExplorerState {
  model: FileTreeModel | null
  expanded: string[] | null
  search: string | null
}

export function FileTree(props: FileTreeProps) {
  const filter = useFilenameFilter()
  const dialog = useRef<HTMLDialogElement>(null)
  const page = useRef(props.page)
  const state = useRef<ExplorerState>({ model: null, expanded: null, search: null })
  const [mobile, setMobile] = useState(() => window.matchMedia("(max-width: 48rem)").matches)
  const scale = uiScale(useSettings().settings)
  const [rowScale, setRowScale] = useState(scale)
  useLayoutEffect(() => { page.current = props.page }, [props.page])
  // saveState keeps expansion and search across an Explorer remount.
  const saveState = useCallback(() => {
    const model = state.current.model
    if (!model) return
    const expanded = new Set(state.current.expanded)
    for (const path of treePaths(page.current.tree)) {
      const item = model.getItem(path)
      if (item && "isExpanded" in item) {
        if (item.isExpanded()) expanded.add(path)
        else expanded.delete(path)
      }
    }
    state.current.expanded = [...expanded]
    state.current.search = model.isSearchOpen() ? model.getSearchValue() : null
  }, [])
  // Pierre fixes row height at creation, so a font size change remounts it.
  useEffect(() => {
    if (scale === rowScale) return
    saveState()
    setRowScale(scale)
  }, [rowScale, saveState, scale])
  useEffect(() => {
    const query = window.matchMedia("(max-width: 48rem)")
    const update = () => {
      saveState()
      setMobile(query.matches)
    }
    query.addEventListener("change", update)
    return () => query.removeEventListener("change", update)
  }, [saveState])

  const itemHeight = mobile ? Math.max(40, Math.round(40 * rowScale)) : Math.round(24 * rowScale)
  const explorer = <Explorer {...props} key={`${mobile ? "mobile" : "desktop"}-${itemHeight}`} mobile={mobile}
    itemHeight={itemHeight} filter={filter} dialog={dialog} state={state} />
  return mobile ? <dialog ref={dialog} className="explorer-drawer" aria-label="Files" onClick={(event) => {
    if (event.target === dialog.current) dialog.current.close()
  }}>{explorer}</dialog> : explorer
}

function Explorer({ hasData, onNavigate, onOpenProjects, onOpenWorktrees, page, registerOpen, mobile, itemHeight, dialog, state, filter }: FileTreeProps & {
  mobile: boolean
  itemHeight: number
  filter: FilenameFilterState
  dialog: RefObject<HTMLDialogElement | null>
  state: RefObject<ExplorerState>
}) {
  const visibleTree = useMemo(() => filterTreeByFilename(page.tree, filter.matcher), [page.tree, filter.matcher])
  const totalCount = useMemo(() => countFiles(page.tree), [page.tree])
  const visibleCount = useMemo(() => countFiles(visibleTree), [visibleTree])
  const paths = useMemo(() => treePaths(visibleTree), [visibleTree])
  const directoryPaths = useMemo(() => treePaths(page.tree).filter((path) => path.endsWith("/")), [page.tree])
  const expandedPaths = useRef(new Set(state.current.expanded ?? openDirectoryPaths(page.tree).map((path) => `${path}/`)))
  const [hasExpanded, setHasExpanded] = useState(expandedPaths.current.size > 0)
  const previousPaths = useRef(paths)
  const previousSelected = useRef(page.selected)
  const searchInput = useRef<HTMLInputElement>(null)
  const searchButton = useRef<HTMLButtonElement>(null)
  const syncing = useRef(false)
  const current = useRef({ onNavigate, page })
  const { model } = useFileTree({
    paths,
    initialExpandedPaths: state.current.expanded ?? openDirectoryPaths(page.tree),
    initialSearchQuery: state.current.search,
    itemHeight,
    initialSelectedPaths: paths.includes(page.selected) ? [page.selected] : [],
    density: "compact",
    search: false,
    unsafeCSS: '[data-icon-name="file-tree-icon-chevron"] { width: 12px; height: 12px; }',
    fileTreeSearchMode: "hide-non-matches",
    icons: { set: "complete", colored: true },
    onSelectionChange(selected) {
      if (syncing.current) return
      const focused = model.getFocusedPath()
      const path = focused && selected.includes(focused) ? focused : selected.at(-1)
      if (!path || model.getItem(path)?.isDirectory()) return
      dialog.current?.close()
      if (path !== current.current.page.selected) current.current.onNavigate(path)
    },
  })
  const search = useFileTreeSearch(model)
  useLayoutEffect(() => {
    if (search.isOpen) searchInput.current?.focus()
  }, [search.isOpen])
  const syncExpandedPaths = useCallback(() => {
    for (const path of directoryPaths) {
      const item = model.getItem(path)
      if (item && "isExpanded" in item) {
        if (item.isExpanded()) expandedPaths.current.add(path)
        else expandedPaths.current.delete(path)
      }
    }
    state.current.expanded = [...expandedPaths.current]
    setHasExpanded(directoryPaths.some((path) => expandedPaths.current.has(path)))
  }, [directoryPaths, model, state])

  useLayoutEffect(() => { current.current = { onNavigate, page } }, [onNavigate, page])

  useLayoutEffect(() => { state.current.model = model }, [model, state])

  useLayoutEffect(() => {
    syncing.current = true
    const changed = paths.length !== previousPaths.current.length
      || paths.some((path, index) => path !== previousPaths.current[index])
    if (changed) {
      for (const path of previousPaths.current) {
        const item = model.getItem(path)
        if (item && "isExpanded" in item) {
          if (item.isExpanded()) expandedPaths.current.add(path)
          else expandedPaths.current.delete(path)
        }
      }
      state.current.expanded = [...expandedPaths.current]
      const query = model.isSearchOpen() ? model.getSearchValue() : null
      model.resetPaths(paths, { initialExpandedPaths: state.current.expanded })
      if (query) {
        // Reset paths keeps the query but requires rebuilding its matches.
        model.setSearch("")
        model.setSearch(query)
      }
      previousPaths.current = paths
    }
    for (const path of model.getSelectedPaths()) {
      if (path !== page.selected) model.getItem(path)?.deselect()
    }
    if (page.selected && model.getItem(page.selected)) {
      if (page.selected !== previousSelected.current) {
        const segments = page.selected.split("/")
        for (let index = 1; index < segments.length; index++) {
          const item = model.getItem(segments.slice(0, index).join("/"))
          if (item && "expand" in item) item.expand()
        }
        model.scrollToPath(page.selected, { focus: false })
      }
      model.getItem(page.selected)?.select()
    }
    previousSelected.current = page.selected
    syncing.current = false
    syncExpandedPaths()
  }, [model, page.selected, paths, syncExpandedPaths])

  useLayoutEffect(() => model.subscribe(() => {
    if (syncing.current) return
    syncExpandedPaths()
  }), [model, syncExpandedPaths])

  const toggleDirectories = () => {
    const expand = !hasExpanded
    syncing.current = true
    for (const path of directoryPaths) {
      if (expand) expandedPaths.current.add(path)
      else expandedPaths.current.delete(path)
      const item = model.getItem(path)
      if (item && "expand" in item) {
        if (expand) item.expand()
        else item.collapse()
      }
    }
    state.current.expanded = [...expandedPaths.current]
    setHasExpanded(expand)
    syncing.current = false
  }

  const focusFirstRow = () => {
    const first = model.getVisibleRows(0, 0)[0]
    if (!first) return
    model.scrollToPath(first.path)
    requestAnimationFrame(() => {
      const row = model.getFileTreeContainer()?.shadowRoot
        ?.querySelector(`[role="treeitem"][data-item-path="${CSS.escape(first.path)}"]`)
      if (row instanceof HTMLElement) row.focus()
    })
  }

  useEffect(() => {
    const open = () => {
      if (mobile && !dialog.current?.open) dialog.current?.showModal()
      if (model.isSearchOpen()) searchInput.current?.focus()
    }
    registerOpen(open)
    const shortcut = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.altKey || event.shiftKey || !(event.metaKey || event.ctrlKey)) return
      if (event.key.toLowerCase() !== "p" && event.key.toLowerCase() !== "k") return
      event.preventDefault()
      open()
      model.openSearch()
      searchInput.current?.focus()
    }
    document.addEventListener("keydown", shortcut)
    return () => {
      registerOpen(null)
      document.removeEventListener("keydown", shortcut)
    }
  }, [mobile, model, registerOpen])

  // A linked worktree shows a colored fork; other checkouts show a branch.
  const projectName = page.repository || page.rootName || page.project
  const branchIcon = page.projectKind === "worktree"
    ? <GitForkIcon className="worktree-icon" aria-hidden="true" />
    : <GitBranchIcon aria-hidden="true" />

  return (
    <aside className="file-explorer" aria-label="Files">
      <div className="explorer-header">
        <button type="button" className="project-trigger" aria-haspopup="dialog"
          aria-label={`Switch project (${projectName || "none"})`}
          aria-keyshortcuts="Control+O Meta+O" title={`Switch project (${projectShortcutLabel})`} onClick={onOpenProjects}>
          <span>{projectName || "Files"}</span>
          <ChevronDownIcon aria-hidden="true" />
        </button>
        {page.branch && (page.worktrees > 1
          ? (
            <button type="button" className="project-branch" aria-haspopup="dialog"
              aria-label={`Switch worktree (${page.branch})`} title={`Switch worktree (${page.branch})`} onClick={onOpenWorktrees}>
              {branchIcon}
              <span>{page.branch}</span>
            </button>
          )
          : (
            <span className="project-branch" title={`${page.projectKind === "worktree" ? "Worktree" : "Branch"} ${page.branch}`}>
              {branchIcon}
              <span>{page.branch}</span>
            </span>
          ))}
        <button ref={searchButton} type="button" aria-label="Search files" aria-expanded={search.isOpen} title="Search files (⌘K / Ctrl+K)"
          onClick={() => { if (model.isSearchOpen()) model.closeSearch(); else model.openSearch() }}>
          <SearchIcon aria-hidden="true" />
        </button>
        <button type="button" aria-label={hasExpanded ? "Collapse all folders" : "Expand all folders"}
          disabled={!hasData || directoryPaths.length === 0 || search.value.length > 0}
          title={search.value.length > 0 ? "Clear filename search to collapse or expand folders" : hasExpanded ? "Collapse all folders" : "Expand all folders"}
          onClick={toggleDirectories}>
          {hasExpanded ? <FoldVerticalIcon aria-hidden="true" /> : <UnfoldVerticalIcon aria-hidden="true" />}
        </button>
        {mobile && <button type="button" aria-label="Close files" onClick={() => dialog.current?.close()}><XIcon aria-hidden="true" /></button>}
      </div>
      {hasData && !page.projectUnavailable && <FilenameFilter filter={filter} scopePatterns={page.filenamePatterns}
        totalCount={totalCount} visibleCount={visibleCount} onFocusTree={focusFirstRow} />}
      <div className="file-search" hidden={!search.isOpen}>
        <input ref={searchInput} className="file-search-input" aria-label="Search…" placeholder="Search…"
          value={search.value} onChange={(event) => model.setSearch(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "ArrowDown") model.focusNextSearchMatch()
            else if (event.key === "ArrowUp") model.focusPreviousSearchMatch()
            else if (event.key === "Escape") {
              model.closeSearch()
              searchButton.current?.focus()
            } else if (event.key === "Enter") {
              const focused = model.getFocusedPath()
              if (focused) model.getItem(focused)?.select()
              model.closeSearch()
              searchButton.current?.focus()
            } else return
            event.preventDefault()
            event.stopPropagation()
          }} />
      </div>
      {!hasData ? <p className="muted">Loading files…</p>
        : page.projectUnavailable ? <p className="muted">Project unavailable.</p>
        : page.empty ? <p className="muted">No files found.</p>
        : paths.length === 0 ? <p className="muted explorer-empty">
          No files match <code>{filter.applied}</code>
          <button type="button" className="explorer-empty-action" onClick={filter.clear}>Clear filter</button>
        </p>
        : <PierreFileTree model={model} className="file-tree-host" aria-label="Files" onClick={() => {
          const focused = model.getFocusedPath()
          if (focused && !model.getItem(focused)?.isDirectory() && model.getSelectedPaths().includes(focused)) dialog.current?.close()
        }} />}
      {page.warnings.length > 0 && (
        <details className="warnings">
          <summary>Some paths could not be read</summary>
          <ul>{page.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>
        </details>
      )}
    </aside>
  )

}
