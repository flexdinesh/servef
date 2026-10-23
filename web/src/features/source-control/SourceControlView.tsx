import { RefreshCwIcon, XIcon } from "lucide-react"
import { useRef, useState, type RefObject } from "react"

import { IconButton } from "../../components/ui/icon-button.tsx"
import { ProgressBar } from "../../components/ui/progress-bar.tsx"
import { SegmentedControl, SegmentedItem } from "../../components/ui/segmented-control.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import { treeRowHeight } from "../../components/ui/tree-rows.ts"
import { ViewHeader } from "../../components/ui/view-header.tsx"
import { useSettings } from "../../settings/SettingsProvider.tsx"
import { uiScale } from "../../settings/settings.ts"
import { CompareList } from "./CompareList.tsx"
import { compareKey, type Compare } from "./compare.ts"
import { FilesPane } from "./FilesPane.tsx"
import { maximumShare, minimumShare } from "./source-control-layout.ts"
import type { Changes } from "./use-changes.ts"
import type { Commits } from "./use-commits.ts"
import type { CompareData } from "./use-compare-data.ts"
import type { Refs } from "./use-refs.ts"
import type { SourceControlLayoutState } from "./use-source-control-layout.ts"

interface SourceControlViewProps {
  changes: Changes
  commits: Commits
  layout: SourceControlLayoutState
  refs: Refs
  // selection is the compare the Files pane shows, with its data.
  selection: Compare | null
  data: CompareData | null
  // activePath is the file in view when the open review is the selection's.
  activePath: string
  mobile: boolean
  onDismiss(): void
  onSelect(compare: Compare): void
  // onOpenFile reviews a file of the selection; focusDocument moves focus.
  onOpenFile(path: string, focusDocument: boolean): void
  // onOpenReview opens a compare's review tab.
  onOpenReview(compare: Compare, focusDocument: boolean): void
}

// SourceControlView lists what can be compared above the selected compare's
// files (decision 0035). On desktop a separator resizes the two; phones show
// one at a time, drilling from the list into the files.
export function SourceControlView({
  activePath, changes, commits, data, layout, mobile, refs, selection, onDismiss, onOpenFile, onOpenReview, onSelect,
}: SourceControlViewProps) {
  const scale = uiScale(useSettings().settings)
  const itemHeight = treeRowHeight(mobile, scale)
  const [drilled, setDrilled] = useState(false)
  const sections = useRef<HTMLDivElement>(null)
  const repository = !changes.result || changes.result.repository
  const history = commits.result
  const branchState = commits.branches.get(layout.base)
  const branchResult = branchState?.result
  const showFiles = !mobile || drilled
  const showList = !mobile || !drilled
  const refreshAll = () => {
    changes.refresh()
    commits.refresh()
  }
  const setRange = (range: typeof layout.range) => {
    // A range of commits the other list may not hold falls back to the branch.
    if (range !== layout.range && selection?.kind === "range") onSelect({ kind: "branch", base: layout.base })
    layout.setRange(range)
  }

  return (
    <section className="source-control-view" aria-label="Source Control">
      <ViewHeader className="explorer-header sidebar-header" title="Source Control" titleClassName="sidebar-title" actions={<>
        <IconButton label="Refresh" icon={<RefreshCwIcon aria-hidden="true" />} onClick={refreshAll} />
        {mobile && <IconButton label="Close source control" icon={<XIcon aria-hidden="true" />} onClick={onDismiss} />}
      </>} />
      <ProgressBar active={changes.loading || commits.loading} />
      {!repository ? (
        <div className="source-control-message" role="status" aria-live="polite">
          <StatusMessage>This folder isn't a git repository.</StatusMessage>
        </div>
      ) : (
        <div ref={sections} className="source-control-sections">
          {showList && (
            <div className="source-control-section" style={mobile ? undefined : { flexGrow: layout.listShare }}>
              <div className="source-control-group">
                <h3 className="source-control-list-title">Commits</h3>
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
              </div>
              <div className="source-control-section-body">
                <CompareList commits={commits} selection={selection} base={layout.base} mobile={mobile} refs={refs}
                  changes={changes.result ? changes.result.files.length : null}
                  branch={{
                    base: branchResult?.base ?? history?.base ?? "",
                    files: branchResult ? branchResult.files.length : null,
                    truncated: branchResult?.truncated ?? false,
                  }}
                  onBaseChange={(base) => {
                    layout.setBase(base)
                    onSelect({ kind: "branch", base })
                  }}
                  onDrill={() => setDrilled(true)}
                  onSelect={onSelect} onOpen={onOpenReview} onShowAll={() => setRange("all")} />
              </div>
            </div>
          )}
          {!mobile && <SectionSeparator container={sections} share={layout.listShare} onChange={layout.setShare} onReset={layout.resetShare} />}
          {showFiles && (
            <div className="source-control-section" style={mobile ? undefined : { flexGrow: 1 - layout.listShare }}>
              {selection && data ? (
                <FilesPane key={compareKey(selection)} compare={selection} details={data.details} list={data.list} stats={data.stats} activePath={activePath}
                  itemHeight={itemHeight} mobile={mobile}
                  onBack={() => setDrilled(false)}
                  onClear={() => { if (selection.kind === "range") onSelect({ kind: "commit", hash: selection.to }) }}
                  onOpen={(path, focusDocument) => {
                    onOpenFile(path, focusDocument)
                    if (mobile) onDismiss()
                  }}
                  onOpenReview={() => {
                    onOpenReview(selection, true)
                    if (mobile) onDismiss()
                  }}
                  onRetry={data.retry} />
              ) : (
                <div className="source-control-message">
                  <StatusMessage>Select changes or a commit to see its files.</StatusMessage>
                </div>
              )}
            </div>
          )}
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

// SectionSeparator resizes the list against the Files pane by pointer or
// arrow keys; double-click restores an even split.
function SectionSeparator({ container, share, onChange, onReset }: SectionSeparatorProps) {
  const dragging = useRef<number | null>(null)
  const shareAt = (clientY: number) => {
    const bounds = container.current?.getBoundingClientRect()
    return bounds && bounds.height > 0 ? (clientY - bounds.top) / bounds.height : share
  }
  return (
    <div className="source-control-separator" role="separator" aria-orientation="horizontal" aria-label="Resize commits and files"
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
