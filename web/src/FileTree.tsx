import type { FileTree as FileTreeModel } from "@pierre/trees"
import { FileTree as PierreFileTree, useFileTree, useFileTreeSearch } from "@pierre/trees/react"
import { SearchIcon, XIcon } from "lucide-react"
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from "react"

import { openDirectoryPaths, type PageData, type TreeNode } from "./page-data.ts"

function treePaths(nodes: readonly TreeNode[]): string[] {
  return nodes.flatMap((node) => node.isDir
    ? [`${node.path}/`, ...treePaths(node.children)]
    : [node.path])
}

interface FileTreeProps {
  hasData: boolean
  onNavigate(path: string): void
  page: PageData
  registerOpen(open: (() => void) | null): void
}

interface ExplorerState {
  model: FileTreeModel | null
  expanded: string[] | null
  search: string | null
}

export function FileTree(props: FileTreeProps) {
  const dialog = useRef<HTMLDialogElement>(null)
  const page = useRef(props.page)
  const state = useRef<ExplorerState>({ model: null, expanded: null, search: null })
  const [mobile, setMobile] = useState(() => window.matchMedia("(max-width: 48rem)").matches)
  useLayoutEffect(() => { page.current = props.page }, [props.page])
  useEffect(() => {
    const query = window.matchMedia("(max-width: 48rem)")
    const update = () => {
      const model = state.current.model
      if (model) {
        state.current.expanded = treePaths(page.current.tree).filter((path) => {
          const item = model.getItem(path)
          return item !== null && "isExpanded" in item && item.isExpanded()
        })
        state.current.search = model.isSearchOpen() ? model.getSearchValue() : null
      }
      setMobile(query.matches)
    }
    query.addEventListener("change", update)
    return () => query.removeEventListener("change", update)
  }, [])

  const explorer = <Explorer {...props} key={mobile ? "mobile" : "desktop"} mobile={mobile} dialog={dialog} state={state} />
  return mobile ? <dialog ref={dialog} className="explorer-drawer" aria-label="Files" onClick={(event) => {
    if (event.target === dialog.current) dialog.current.close()
  }}>{explorer}</dialog> : explorer
}

function Explorer({ hasData, onNavigate, page, registerOpen, mobile, dialog, state }: FileTreeProps & {
  mobile: boolean
  dialog: RefObject<HTMLDialogElement | null>
  state: RefObject<ExplorerState>
}) {
  const paths = useMemo(() => treePaths(page.tree), [page.tree])
  const previousPaths = useRef(paths)
  const syncing = useRef(false)
  const current = useRef({ onNavigate, page })
  const { model } = useFileTree({
    paths,
    initialExpandedPaths: state.current.expanded ?? openDirectoryPaths(page.tree),
    initialSearchQuery: state.current.search,
    itemHeight: mobile ? 40 : 24,
    initialSelectedPaths: page.selected ? [page.selected] : [],
    density: "compact",
    search: true,
    unsafeCSS: '[data-file-tree-search-container][data-open="false"] { display: none; }',
    fileTreeSearchMode: "hide-non-matches",
    icons: { set: "minimal", colored: false },
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

  useLayoutEffect(() => { current.current = { onNavigate, page } }, [onNavigate, page])

  useLayoutEffect(() => { state.current.model = model }, [model, state])

  useLayoutEffect(() => {
    syncing.current = true
    const changed = paths.length !== previousPaths.current.length
      || paths.some((path, index) => path !== previousPaths.current[index])
    if (changed) {
      const expanded = previousPaths.current.filter((path) => {
        const item = model.getItem(path)
        return item !== null && "isExpanded" in item && item.isExpanded()
      })
      model.resetPaths(paths, { initialExpandedPaths: expanded })
      previousPaths.current = paths
    }
    for (const path of model.getSelectedPaths()) {
      if (path !== page.selected) model.getItem(path)?.deselect()
    }
    if (page.selected) {
      const segments = page.selected.split("/")
      for (let index = 1; index < segments.length; index++) {
        const item = model.getItem(segments.slice(0, index).join("/"))
        if (item && "expand" in item) item.expand()
      }
      model.getItem(page.selected)?.select()
      model.scrollToPath(page.selected, { focus: false })
    }
    syncing.current = false
  }, [model, page.selected, paths])

  useEffect(() => {
    const open = () => {
      if (mobile && !dialog.current?.open) dialog.current?.showModal()
    }
    registerOpen(open)
    const shortcut = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.altKey || event.shiftKey || !(event.metaKey || event.ctrlKey)) return
      if (event.key.toLowerCase() !== "p" && event.key.toLowerCase() !== "k") return
      event.preventDefault()
      open()
      model.openSearch()
    }
    document.addEventListener("keydown", shortcut)
    return () => {
      registerOpen(null)
      document.removeEventListener("keydown", shortcut)
    }
  }, [mobile, model, registerOpen])

  return (
    <aside className="file-explorer" aria-label="Markdown files">
      <div className="explorer-header">
        <span title={page.rootName}>{page.rootName || "Markdown files"}</span>
        <button type="button" aria-label="Search files" aria-expanded={search.isOpen} title="Search files (⌘K / Ctrl+K)"
          onPointerDown={(event) => {
            // Pierre closes search on blur; preserve its state until the toggle click.
            if (event.button === 0) event.preventDefault()
          }}
          onClick={() => { if (model.isSearchOpen()) model.closeSearch(); else model.openSearch() }}>
          <SearchIcon aria-hidden="true" />
        </button>
        {mobile && <button type="button" aria-label="Close files" onClick={() => dialog.current?.close()}><XIcon aria-hidden="true" /></button>}
      </div>
      {!hasData ? <p className="muted">Loading Markdown files…</p>
        : page.empty ? <p className="muted">No Markdown files found.</p>
        : <PierreFileTree model={model} className="file-tree-host" aria-label="Markdown files" onClick={() => {
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
