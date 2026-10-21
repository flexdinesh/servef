import type { GitStatusEntry } from "@pierre/trees"
import { FileTree as PierreFileTree, useFileTree } from "@pierre/trees/react"
import { FoldVerticalIcon, RefreshCwIcon, UnfoldVerticalIcon, XIcon } from "lucide-react"
import { useCallback, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from "react"

import { Button } from "./components/ui/button.tsx"
import { useSettings } from "./components/SettingsProvider.tsx"
import type { ChangedFile } from "./changes-contract.ts"
import { uiScale } from "./settings.ts"
import { useDelayedFlag } from "./use-document-format.ts"
import type { Changes } from "./use-changes.ts"

// Changed files sit inside folders; Pierre takes folder paths with a slash.
function ancestorDirectories(paths: readonly string[]): string[] {
  const directories = new Set<string>()
  for (const path of paths) {
    const segments = path.split("/")
    for (let index = 1; index < segments.length; index++) directories.add(`${segments.slice(0, index).join("/")}/`)
  }
  return [...directories]
}

// Every Source Control folder holds changes, so folder dots add no signal;
// deleted files read as removed.
const treeCSS = `[data-icon-name="file-tree-icon-chevron"] { width: 12px; height: 12px; }
[data-item-contains-git-change="true"] > [data-item-section="git"] { visibility: hidden; }
[data-item-git-status="deleted"] > [data-item-section="content"] { text-decoration: line-through; }`

interface SourceControlViewProps {
  changes: Changes
  // activePath is the open diff tab's path, selected in the list.
  activePath: string
  mobile: boolean
  onDismiss(): void
  // onOpen opens a diff tab; focusDocument moves focus to it.
  onOpen(path: string, focusDocument: boolean): void
}

export function SourceControlView({ activePath, changes, mobile, onDismiss, onOpen }: SourceControlViewProps) {
  const { error, loading, refresh, result } = changes
  const files = result?.files
  const reading = useDelayedFlag(loading, 150)
  const scale = uiScale(useSettings().settings)
  const itemHeight = Math.round((mobile ? 32 : 24) * scale)
  const [anyExpanded, setAnyExpanded] = useState(true)
  const [collapseRequest, setCollapseRequest] = useState<{ expand: boolean; id: number } | null>(null)
  const count = files?.length ?? 0

  return (
    <section className="source-control-view" aria-label="Source Control">
      <div className="explorer-header sidebar-header">
        <h2 className="sidebar-title">Source Control</h2>
        <button type="button" aria-label="Refresh" title="Refresh" onClick={refresh}>
          <RefreshCwIcon aria-hidden="true" />
        </button>
        <button type="button" aria-label={anyExpanded ? "Collapse All" : "Expand All"} title={anyExpanded ? "Collapse All" : "Expand All"}
          disabled={count === 0} onClick={() => setCollapseRequest({ expand: !anyExpanded, id: (collapseRequest?.id ?? 0) + 1 })}>
          {anyExpanded ? <FoldVerticalIcon aria-hidden="true" /> : <UnfoldVerticalIcon aria-hidden="true" />}
        </button>
        {mobile && <button type="button" aria-label="Close source control" onClick={onDismiss}><XIcon aria-hidden="true" /></button>}
      </div>
      <div className={`search-progress${reading ? " active" : ""}`} aria-hidden="true" />
      <div className="source-control-message" role="status" aria-live="polite">
        {error ? <p className="search-status-error">{error}</p>
          : result && !result.repository ? <p>This folder isn't a git repository.</p>
          : result && count === 0 ? <p>No changes since last commit.</p>
          : null}
        {error && <Button type="button" size="sm" variant="outline" onClick={refresh}>Retry</Button>}
        {result?.truncated && <p>Showing the first {count.toLocaleString()} changes. Commit or ignore files to see the rest.</p>}
      </div>
      {files && count > 0 && <>
        <h3 className="source-control-group" aria-label={`Changes, ${count === 1 ? "1 change" : `${count.toLocaleString()} changes`}`}>
          <span>Changes</span>
          <span className="search-count">{count.toLocaleString()}</span>
        </h3>
        <ChangeTree key={itemHeight} files={files} byPath={changes.byPath} activePath={activePath} itemHeight={itemHeight}
          collapseRequest={collapseRequest} onExpandedChange={setAnyExpanded} onOpen={(path, focusDocument) => {
            onOpen(path, focusDocument)
            if (mobile) onDismiss()
          }} />
      </>}
      {result && result.warnings.length > 0 && (
        <details className="warnings">
          <summary>Some paths could not be read</summary>
          <ul>{result.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>
        </details>
      )}
    </section>
  )
}

interface ChangeTreeProps {
  activePath: string
  byPath: ReadonlyMap<string, ChangedFile>
  collapseRequest: { expand: boolean; id: number } | null
  files: readonly ChangedFile[]
  itemHeight: number
  onExpandedChange(expanded: boolean): void
  onOpen(path: string, focusDocument: boolean): void
}

function ChangeTree({ activePath, byPath, collapseRequest, files, itemHeight, onExpandedChange, onOpen }: ChangeTreeProps) {
  const paths = useMemo(() => files.map((file) => file.path), [files])
  const gitStatus = useMemo(() => files.map((file): GitStatusEntry => ({ path: file.path, status: file.status })), [files])
  const directories = useMemo(() => ancestorDirectories(paths), [paths])
  // Folders open by default; only ones the user collapsed stay closed.
  const collapsed = useRef(new Set<string>())
  const previousPaths = useRef(paths)
  const syncing = useRef(false)
  const current = useRef({ activePath, byPath, onOpen })
  useLayoutEffect(() => { current.current = { activePath, byPath, onOpen } }, [activePath, byPath, onOpen])

  const { model } = useFileTree({
    paths,
    flattenEmptyDirectories: true,
    initialExpansion: "open",
    itemHeight,
    initialSelectedPaths: paths.includes(activePath) ? [activePath] : [],
    density: "compact",
    search: false,
    unsafeCSS: treeCSS,
    icons: { set: "complete", colored: true },
    gitStatus,
    renderRowDecoration({ item }) {
      const oldPath = item.kind === "file" ? current.current.byPath.get(item.path)?.oldPath : undefined
      // Pierre truncates decorations from the start; the R badge says "renamed".
      return oldPath ? { text: `from ${oldPath}`, title: `Renamed from ${oldPath}` } : null
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
  // the diff, as in the Search view.
  const openFocused = () => {
    const focused = model.getFocusedPath()
    if (focused && !model.getItem(focused)?.isDirectory()) current.current.onOpen(focused, true)
  }

  return <PierreFileTree model={model} className="file-tree-host" aria-label="Changes"
    onKeyDown={(event: KeyboardEvent<HTMLElement>) => { if (event.key === "Enter") openFocused() }}
    onDoubleClick={openFocused} />
}
