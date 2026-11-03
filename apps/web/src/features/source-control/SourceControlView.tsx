import { FileDiffIcon, FoldVerticalIcon, ListIcon, ListTreeIcon, RefreshCwIcon, UnfoldVerticalIcon, XIcon } from "lucide-react"
import { Fragment, useMemo, useRef, useState, type ReactNode } from "react"

import { Badge } from "../../components/ui/badge.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { ProgressBar } from "../../components/ui/progress-bar.tsx"
import { SegmentedControl, SegmentedItem } from "../../components/ui/segmented-control.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import { treeRowHeight } from "../../components/ui/tree-rows.ts"
import { ViewHeader } from "../../components/ui/view-header.tsx"
import { useSettings } from "../../settings/SettingsProvider.tsx"
import { uiScale } from "../../settings/settings.ts"
import { BasePicker } from "./BasePicker.tsx"
import { BranchesPane } from "./BranchesPane.tsx"
import { CommitsPane } from "./CommitsPane.tsx"
import { workingCompare, type Compare } from "./compare.ts"
import type { CollapseRequest } from "./FilesTree.tsx"
import { Pane, PaneSash } from "./Pane.tsx"
import { paneIds, paneOpen, paneShares, type PaneId } from "./source-control-layout.ts"
import { refChips } from "./source-control-panes.ts"
import { UncommittedPane } from "./UncommittedPane.tsx"
import type { BranchStack } from "./use-branch-stack.ts"
import type { Changes } from "./use-changes.ts"
import type { Commits } from "./use-commits.ts"
import type { CompareData } from "./use-compare-data.ts"
import type { Refs } from "./use-refs.ts"
import type { SourceControlLayoutState } from "./use-source-control-layout.ts"

interface SourceControlViewProps {
  changes: Changes
  commits: Commits
  stack: BranchStack
  layout: SourceControlLayoutState
  refs: Refs
  // working is the uncommitted changes' list and stats.
  working: CompareData | null
  // workingPath is the file in view when the open review is Uncommitted.
  workingPath: string
  // commitSelection is the selected commit or range.
  commitSelection: Compare | null
  // allOnly is set when HEAD is the base branch; commits list all history.
  allOnly: boolean
  mobile: boolean
  onDismiss(): void
  onSelectCommits(compare: Compare): void
  // onOpenReview opens a compare's review tab, at path when given;
  // focusDocument moves focus to it.
  onOpenReview(compare: Compare, focusDocument: boolean, path?: string): void
}

const paneLabels: Record<PaneId, string> = { working: "Uncommitted", commits: "Commits", branches: "Branches" }

// SourceControlView stacks three panes in a fixed order (decision 0040):
// Uncommitted, Commits, then Branches. Empty panes start closed; the user's
// choice wins. On desktop open panes split the height with sashes between
// them; phones stack them and the view scrolls.
export function SourceControlView({
  allOnly, changes, commitSelection, commits, layout, mobile, refs, stack, working, workingPath,
  onDismiss, onOpenReview, onSelectCommits,
}: SourceControlViewProps) {
  const scale = uiScale(useSettings().settings)
  const itemHeight = treeRowHeight(mobile, scale)
  const panes = useRef<HTMLDivElement>(null)
  const [anyExpanded, setAnyExpanded] = useState(true)
  const [collapseRequest, setCollapseRequest] = useState<CollapseRequest | null>(null)
  const repository = (!changes.result || changes.result.repository) && (!stack.result || stack.result.repository)
  const history = commits.result
  const chips = useMemo(() => refChips(stack.result), [stack.result])
  const changeCount = changes.result ? changes.result.files.length : null
  const empty: Record<PaneId, boolean | null> = {
    working: changeCount === null ? null : changeCount === 0,
    commits: history ? history.commits.length === 0 : null,
    branches: stack.result ? stack.result.layers.length === 0 : null,
  }
  const open = paneIds.filter((id) => paneOpen(layout.collapsed[id], empty[id]))
  const shares = paneShares(layout.weights, open)
  const refreshAll = () => {
    changes.refresh()
    commits.refresh()
    stack.refresh()
  }
  const review = (compare: Compare, focusDocument: boolean, path?: string) => {
    onOpenReview(compare, focusDocument, path)
    if (mobile) onDismiss()
  }
  const setRange = (range: typeof layout.range) => {
    // A range of commits the other list may not hold is dropped.
    if (range !== layout.range && commitSelection?.kind === "range") onSelectCommits({ kind: "commit", hash: commitSelection.to })
    layout.setRange(range)
  }
  const fileCount = working?.list.kind === "ready" ? working.list.files.length : 0

  const bodies: Record<PaneId, { note?: ReactNode; actions?: ReactNode; body: ReactNode }> = {
    working: {
      note: changeCount !== null && changeCount > 0 && (
        <Badge className="scm-pane-count" aria-label={`${changeCount.toLocaleString()} ${changeCount === 1 ? "file" : "files"}`}>
          {changeCount.toLocaleString()}{changes.result?.truncated ? "+" : ""}
        </Badge>
      ),
      actions: <>
        {layout.changesView === "tree" && (
          <IconButton size="xs" label={anyExpanded ? "Collapse All Folders" : "Expand All Folders"}
            icon={anyExpanded ? <FoldVerticalIcon aria-hidden="true" /> : <UnfoldVerticalIcon aria-hidden="true" />}
            disabled={fileCount === 0}
            onClick={() => setCollapseRequest({ expand: !anyExpanded, id: (collapseRequest?.id ?? 0) + 1 })} />
        )}
        <IconButton size="xs" label={layout.changesView === "list" ? "View as Tree" : "View as List"}
          icon={layout.changesView === "list" ? <ListTreeIcon aria-hidden="true" /> : <ListIcon aria-hidden="true" />}
          onClick={() => layout.setChangesView(layout.changesView === "list" ? "tree" : "list")} />
        <IconButton size="xs" label="Open Changes" title="Open all uncommitted changes in a review tab"
          icon={<FileDiffIcon aria-hidden="true" />} disabled={fileCount === 0} onClick={() => review(workingCompare, true)} />
      </>,
      body: working && (
        <UncommittedPane list={working.list} stats={working.stats} warnings={working.details.kind === "working" ? working.details.warnings : []}
          view={layout.changesView} activePath={workingPath} itemHeight={itemHeight} collapseRequest={collapseRequest}
          onExpandedChange={setAnyExpanded} onRetry={working.retry}
          onOpen={(path, focusDocument) => review(workingCompare, focusDocument, path)} />
      ),
    },
    commits: {
      note: history && history.range === "branch" && history.base && (
        <span className="scm-pane-note" title={`${history.ahead.toLocaleString()} commits on ${history.branch || "HEAD"} that ${history.base} does not have`}>
          {history.ahead.toLocaleString()} ahead of {history.base}
        </span>
      ),
      actions: (
        <SegmentedControl size="sm" className="source-control-range" aria-label="Commit range">
          <SegmentedItem pressed={!allOnly && layout.range === "branch"} disabled={allOnly}
            title={allOnly ? "HEAD is the base branch, so all history shows" : "Commits on this branch only"}
            onClick={() => setRange("branch")}>Branch</SegmentedItem>
          <SegmentedItem pressed={allOnly || layout.range === "all"} disabled={allOnly}
            title={allOnly ? "HEAD is the base branch, so all history shows" : "All history of HEAD"}
            onClick={() => setRange("all")}>All</SegmentedItem>
        </SegmentedControl>
      ),
      body: <CommitsPane commits={commits} selection={commitSelection} chips={chips} allOnly={allOnly} mobile={mobile}
        onSelect={onSelectCommits} onOpen={review} onShowAll={() => setRange("all")} />,
    },
    branches: {
      note: stack.result?.base && <span className="scm-pane-note">base {stack.result.base}</span>,
      actions: <BasePicker base={layout.base} refs={refs} onChange={layout.setBase} />,
      body: <BranchesPane stack={stack} base={layout.base} onOpen={review} />,
    },
  }

  return (
    <section className="source-control-view" aria-label="Source Control">
      <ViewHeader className="explorer-header sidebar-header" title="Source Control" titleClassName="sidebar-title" actions={<>
        <IconButton label="Refresh" icon={<RefreshCwIcon aria-hidden="true" />} onClick={refreshAll} />
        {mobile && <IconButton label="Close source control" icon={<XIcon aria-hidden="true" />} onClick={onDismiss} />}
      </>} />
      <ProgressBar active={changes.loading || commits.loading || stack.loading} />
      {!repository ? (
        <div className="source-control-message" role="status" aria-live="polite">
          <StatusMessage>This folder isn't a git repository.</StatusMessage>
        </div>
      ) : (
        <div ref={panes} className="scm-panes" data-mobile={mobile || undefined}>
          {paneIds.map((id) => {
            const isOpen = open.includes(id)
            // A sash sits above each open pane that follows another open pane.
            const above = open[open.indexOf(id) - 1]
            const pane = bodies[id]
            return (
              <Fragment key={id}>
                {!mobile && isOpen && above && (
                  <PaneSash container={panes} label={`Resize ${paneLabels[above]} and ${paneLabels[id]}`}
                    value={layout.weights[above] / (layout.weights[above] + layout.weights[id])}
                    onResize={(delta) => layout.resizePanes(open, above, id, delta)} onReset={layout.resetPanes} />
                )}
                <Pane title={paneLabels[id]} open={isOpen} weight={shares[id] ?? 0} mobile={mobile}
                  note={pane.note} actions={pane.actions} onOpenChange={(next) => layout.setPaneCollapsed(id, !next)}>
                  {pane.body}
                </Pane>
              </Fragment>
            )
          })}
        </div>
      )}
    </section>
  )
}
