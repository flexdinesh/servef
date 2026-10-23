import type { GitStatusEntry } from "@pierre/trees"
import { FileTree as PierreFileTree, useFileTree } from "@pierre/trees/react"
import { FoldVerticalIcon, RefreshCwIcon, UnfoldVerticalIcon, XIcon } from "lucide-react"
import { useCallback, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type RefObject } from "react"

import { Badge } from "../../components/ui/badge.tsx"
import { Button } from "../../components/ui/button.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import { ProgressBar } from "../../components/ui/progress-bar.tsx"
import { SectionToggle } from "../../components/ui/section-toggle.tsx"
import { SegmentedControl, SegmentedItem } from "../../components/ui/segmented-control.tsx"
import { treeRowHeight } from "../../components/ui/tree-rows.ts"
import { ViewHeader } from "../../components/ui/view-header.tsx"
import { useSettings } from "../../settings/SettingsProvider.tsx"
import type { ChangedFile } from "../../api/changes-contract.ts"
import { compareRowKey, rangeSelection, selectedCommits, type CommitSelection, type PinnedCompare } from "./commit-rows.ts"
import { CommitsView } from "./CommitsView.tsx"
import { uiScale } from "../../settings/settings.ts"
import { maximumShare, minimumShare } from "./source-control-layout.ts"
import type { Changes } from "./use-changes.ts"
import { rangeKey, type Commits } from "./use-commits.ts"
import type { SourceControlLayoutState } from "./use-source-control-layout.ts"
import type { WorkspaceTab } from "../../workspace/workspace-tabs.ts"

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
  commits: Commits
  layout: SourceControlLayoutState
  // activePath is the open working-tree diff tab's path, selected in Changes.
  activePath: string
  // activeHistory is the open commit, branch, or range diff tab, selected in
  // Commits.
  activeHistory: WorkspaceTab | null
  mobile: boolean
  onDismiss(): void
  // onOpen opens a diff tab; focusDocument moves focus to it.
  onOpen(path: string, focusDocument: boolean): void
  // onOpenTab opens a commit, branch, or range file's diff tab.
  onOpenTab(tab: WorkspaceTab, focusDocument: boolean): void
}

// SourceControlView stacks two collapsible sections: Changes (the working
// tree against HEAD) and Commits (the branch's history). On desktop a
// separator resizes them while both are open.
export function SourceControlView({ activeHistory, activePath, changes, commits, layout, mobile, onDismiss, onOpen, onOpenTab }: SourceControlViewProps) {
  const { error, loading, refresh, result } = changes
  const files = result?.files
  const scale = uiScale(useSettings().settings)
  const itemHeight = treeRowHeight(mobile, scale)
  const [anyExpanded, setAnyExpanded] = useState(true)
  const [collapseRequest, setCollapseRequest] = useState<{ expand: boolean; id: number } | null>(null)
  const [expandedCommits, setExpandedCommits] = useState<ReadonlySet<string>>(() => new Set())
  const sections = useRef<HTMLDivElement>(null)
  const count = files?.length ?? 0
  const repository = !result || result.repository
  const history = commits.result
  // selection is a Shift-selected commit run (decision 0030); it is
  // transient and lapses once either end leaves the list.
  const [selection, setSelection] = useState<CommitSelection | null>(null)
  const listed = useMemo(() => history?.commits ?? [], [history])
  const selected = useMemo(() => selectedCommits(listed, selection), [listed, selection])
  if (selection && !selected) setSelection(null)
  const branch = commits.branch.result
  const pinned = useMemo((): PinnedCompare | null => {
    const expanded = expandedCommits.has(compareRowKey)
    if (selection && selected) {
      const compare = { kind: "range" as const, from: selection.from, to: selection.to, count: selected.length }
      return { compare, expanded, files: commits.rangeFiles.get(rangeKey(selection.from, selection.to)) }
    }
    if (!history || history.range !== "branch" || history.ahead === 0 || !history.base) return null
    const current = branch && branch.head === history.head && branch.base === history.base ? branch : null
    const files = current ? { kind: "ready" as const, response: current }
      : commits.branch.error ? { kind: "error" as const, message: commits.branch.error } : { kind: "loading" as const }
    return { compare: { kind: "branch", base: history.base, ahead: history.ahead, mergeBase: current?.mergeBase ?? "" }, expanded, files }
  }, [branch, commits.branch.error, commits.rangeFiles, expandedCommits, history, selected, selection])
  const anyCommitExpanded = (history?.commits.some((commit) => expandedCommits.has(commit.hash)) ?? false)
    || (pinned?.expanded ?? false)
  // Selecting a run opens the pinned row on it, since its files are the point.
  const select = useCallback((anchor: string, focus: string) => {
    const next = rangeSelection(listed, anchor, focus)
    setSelection(next)
    if (next) setExpandedCommits((current) => current.has(compareRowKey) ? current : new Set(current).add(compareRowKey))
  }, [listed])
  const setRange = (range: typeof layout.range) => {
    if (range !== layout.range) setSelection(null)
    layout.setRange(range)
  }
  const bothOpen = !layout.changesCollapsed && !layout.commitsCollapsed
  // Open sections share the height; the desktop split weights them.
  const share = mobile || !bothOpen ? 0.5 : layout.changesShare
  const refreshAll = () => {
    refresh()
    commits.refresh()
  }

  return (
    <section className="source-control-view" aria-label="Source Control">
      <ViewHeader className="explorer-header sidebar-header" title="Source Control" titleClassName="sidebar-title" actions={<>
        <IconButton label="Refresh" icon={<RefreshCwIcon aria-hidden="true" />} onClick={refreshAll} />
        <IconButton label={anyExpanded ? "Collapse All" : "Expand All"}
          icon={anyExpanded ? <FoldVerticalIcon aria-hidden="true" /> : <UnfoldVerticalIcon aria-hidden="true" />}
          disabled={count === 0 || layout.changesCollapsed} onClick={() => setCollapseRequest({ expand: !anyExpanded, id: (collapseRequest?.id ?? 0) + 1 })} />
        {mobile && <IconButton label="Close source control" icon={<XIcon aria-hidden="true" />} onClick={onDismiss} />}
      </>} />
      <ProgressBar active={loading || commits.loading} />
      {!repository ? (
        <div className="source-control-message" role="status" aria-live="polite">
          <StatusMessage>This folder isn't a git repository.</StatusMessage>
        </div>
      ) : (
        <div ref={sections} className="source-control-sections">
          <div className="source-control-section" data-collapsed={layout.changesCollapsed || undefined}
            style={layout.changesCollapsed ? undefined : { flexGrow: share }}>
            <div className="source-control-group">
              <h3 aria-label={`Changes, ${count === 1 ? "1 change" : `${count.toLocaleString()} changes`}`}>
                <SectionToggle expanded={!layout.changesCollapsed} aria-controls="source-control-changes" onExpandedChange={layout.toggleChanges}>
                  Changes
                </SectionToggle>
              </h3>
              {files && <Badge className="source-control-count" aria-hidden="true">{count.toLocaleString()}</Badge>}
            </div>
            <div id="source-control-changes" className="source-control-section-body" hidden={layout.changesCollapsed}>
              <div className="source-control-message" role="status" aria-live="polite">
                {error ? <StatusMessage tone="error">{error}</StatusMessage>
                  : result && count === 0 ? <StatusMessage>No changes since last commit.</StatusMessage>
                  : null}
                {error && <Button variant="outline" onClick={refresh}>Retry</Button>}
                {result?.truncated && <StatusMessage>Showing the first {count.toLocaleString()} changes. Commit or ignore files to see the rest.</StatusMessage>}
              </div>
              {files && count > 0 && (
                <ChangeTree key={itemHeight} files={files} byPath={changes.byPath} activePath={activePath} itemHeight={itemHeight}
                  collapseRequest={collapseRequest} onExpandedChange={setAnyExpanded} onOpen={(path, focusDocument) => {
                    onOpen(path, focusDocument)
                    if (mobile) onDismiss()
                  }} />
              )}
              {result && result.warnings.length > 0 && (
                <details className="warnings">
                  <summary>Some paths could not be read</summary>
                  <ul>{result.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>
                </details>
              )}
            </div>
          </div>
          {bothOpen && !mobile && <SectionSeparator container={sections} share={layout.changesShare} onChange={layout.setShare} onReset={layout.resetShare} />}
          <div className="source-control-section" data-collapsed={layout.commitsCollapsed || undefined}
            style={layout.commitsCollapsed ? undefined : { flexGrow: 1 - share }}>
            <div className="source-control-group">
              <h3>
                <SectionToggle expanded={!layout.commitsCollapsed} aria-controls="source-control-commits" onExpandedChange={layout.toggleCommits}>
                  Commits
                </SectionToggle>
              </h3>
              {history && history.range === "branch" && history.base && (
                <span className="source-control-range-note" title={`${history.ahead.toLocaleString()} commits on ${history.branch || "HEAD"} that ${history.base} does not have`}>
                  {history.ahead.toLocaleString()} ahead of {history.base}
                </span>
              )}
              <SegmentedControl size="sm" className="source-control-range" aria-label="Commit range">
                <SegmentedItem pressed={layout.range === "branch"} title="Commits on this branch only"
                  onClick={() => setRange("branch")}>Branch</SegmentedItem>
                <SegmentedItem pressed={layout.range === "all"} title="All history of HEAD"
                  onClick={() => setRange("all")}>All</SegmentedItem>
              </SegmentedControl>
              <IconButton size="xs" className="source-control-group-action" label="Collapse All Commits" icon={<FoldVerticalIcon aria-hidden="true" />}
                disabled={layout.commitsCollapsed || !anyCommitExpanded} onClick={() => setExpandedCommits(new Set())} />
            </div>
            <div id="source-control-commits" className="source-control-section-body" hidden={layout.commitsCollapsed}>
              <CommitsView commits={commits} range={layout.range} active={activeHistory} expanded={expandedCommits} mobile={mobile}
                pinned={pinned} selected={selected} onClearSelection={() => setSelection(null)} onSelect={select}
                onDismiss={onDismiss} onExpandedChange={setExpandedCommits} onOpen={onOpenTab} onShowAll={() => setRange("all")} />
            </div>
          </div>
        </div>
      )}
    </section>
  )
}

const shareStep = 0.05

interface SectionSeparatorProps {
  container: RefObject<HTMLDivElement | null>
  share: number
  onChange(share: number): void
  onReset(): void
}

// SectionSeparator resizes Changes against Commits by pointer or arrow keys;
// double-click restores an even split.
function SectionSeparator({ container, share, onChange, onReset }: SectionSeparatorProps) {
  const dragging = useRef<number | null>(null)
  const shareAt = (clientY: number) => {
    const bounds = container.current?.getBoundingClientRect()
    return bounds && bounds.height > 0 ? (clientY - bounds.top) / bounds.height : share
  }
  return (
    <div className="source-control-separator" role="separator" aria-orientation="horizontal" aria-label="Resize Changes and Commits"
      aria-controls="source-control-changes source-control-commits"
      aria-valuemin={Math.round(minimumShare * 100)} aria-valuemax={Math.round(maximumShare * 100)} aria-valuenow={Math.round(share * 100)}
      tabIndex={0}
      onDoubleClick={onReset}
      onKeyDown={(event) => {
        if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return
        event.preventDefault()
        onChange(share + (event.key === "ArrowUp" ? -shareStep : shareStep))
      }}
      onPointerDown={(event) => {
        dragging.current = event.pointerId
        event.currentTarget.setPointerCapture(event.pointerId)
      }}
      onPointerMove={(event) => {
        if (dragging.current === event.pointerId) onChange(shareAt(event.clientY))
      }}
      onPointerUp={(event) => {
        if (dragging.current !== event.pointerId) return
        dragging.current = null
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
      }}
      onPointerCancel={() => { dragging.current = null }} />
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
