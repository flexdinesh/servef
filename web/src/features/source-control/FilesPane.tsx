import { ArrowLeftIcon, CheckIcon, CopyIcon, FileDiffIcon, FoldVerticalIcon, UnfoldVerticalIcon, XIcon } from "lucide-react"
import { useLayoutEffect, useRef, useState } from "react"

import { Badge } from "../../components/ui/badge.tsx"
import { Button } from "../../components/ui/button.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import type { Commit } from "../../api/commits-contract.ts"
import { shortHash } from "../../lib/commit-hash.ts"
import { useClipboardCopy } from "../../lib/clipboard.ts"
import { commitDate, type Compare } from "./compare.ts"
import type { FileList } from "./compare-files.ts"
import { FilesTree, type CollapseRequest } from "./FilesTree.tsx"
import type { CompareStats } from "./use-compare-stats.ts"

const mac = typeof navigator !== "undefined" && navigator.platform.includes("Mac")
const copyShortcut = mac ? "⌘C" : "Ctrl+C"

// CompareDetails is what the Files pane says about its compare.
export type CompareDetails =
  | { kind: "working"; warnings: readonly string[] }
  | { kind: "branch"; base: string; ahead: number; mergeBase: string }
  | { kind: "commit"; commit: Commit | null }
  | { kind: "range"; count: number }

interface FilesPaneProps {
  compare: Compare
  details: CompareDetails
  list: FileList
  stats: CompareStats | null
  activePath: string
  itemHeight: number
  mobile: boolean
  onBack(): void
  // onClear clears a range back to its newest commit.
  onClear(): void
  // onOpen reviews a file of the compare; focusDocument moves focus there.
  onOpen(path: string, focusDocument: boolean): void
  onOpenReview(): void
  onRetry(): void
}

function plural(count: number, noun: string): string {
  return `${count.toLocaleString()} ${noun}${count === 1 ? "" : "s"}`
}

function paneTitle(compare: Compare, details: CompareDetails): string {
  if (details.kind === "working") return "Uncommitted"
  if (details.kind === "branch") return details.base ? `vs ${details.base}` : "Branch"
  if (details.kind === "commit") return details.commit?.subject || (compare.kind === "commit" ? shortHash(compare.hash) : "Commit")
  return details.count > 0 ? plural(details.count, "commit") : "Commit range"
}

// FilesPane shows the selected compare's files (decision 0035): a header
// naming the compare, its details, then a tree with Expand/Collapse All.
export function FilesPane({ activePath, compare, details, itemHeight, list, mobile, stats, onBack, onClear, onOpen, onOpenReview, onRetry }: FilesPaneProps) {
  const [anyExpanded, setAnyExpanded] = useState(true)
  const [collapseRequest, setCollapseRequest] = useState<CollapseRequest | null>(null)
  const files = list.kind === "ready" ? list.files : []
  const title = paneTitle(compare, details)
  const count = list.kind === "ready" ? `${list.files.length.toLocaleString()}${list.truncated ? "+" : ""}` : ""

  return (
    <section className="files-pane" aria-label={`Files: ${title}`}>
      <div className="source-control-group files-pane-header">
        {mobile && <IconButton size="xs" label="Back to list" icon={<ArrowLeftIcon aria-hidden="true" />} onClick={onBack} />}
        <h3 className="files-pane-title" title={title}>{title}</h3>
        {compare.kind === "range" && <span className="files-pane-span">{shortHash(compare.from)}..{shortHash(compare.to)}</span>}
        {count && <Badge className="source-control-count" aria-label={plural(files.length, "file")}>{count}</Badge>}
        {compare.kind === "range" && (
          <IconButton size="xs" label="Clear range" title="Clear range (Escape)" icon={<XIcon aria-hidden="true" />} onClick={onClear} />
        )}
        <IconButton size="xs" className={count ? undefined : "source-control-count"} label="Open Changes" title="Open all changes in a review tab"
          icon={<FileDiffIcon aria-hidden="true" />} disabled={files.length === 0} onClick={onOpenReview} />
        <IconButton size="xs" label={anyExpanded ? "Collapse All Folders" : "Expand All Folders"}
          icon={anyExpanded ? <FoldVerticalIcon aria-hidden="true" /> : <UnfoldVerticalIcon aria-hidden="true" />}
          disabled={files.length === 0}
          onClick={() => setCollapseRequest({ expand: !anyExpanded, id: (collapseRequest?.id ?? 0) + 1 })} />
      </div>
      <div className="files-pane-body">
        <Details compare={compare} details={details} />
        <div className="source-control-message" role="status" aria-live="polite">
          {list.kind === "loading" && <StatusMessage>Loading files…</StatusMessage>}
          {list.kind === "error" && <>
            <StatusMessage tone="error">{list.message}</StatusMessage>
            <Button variant="outline" onClick={onRetry}>Retry</Button>
          </>}
          {list.kind === "ready" && list.files.length === 0 && (
            <StatusMessage>{compare.kind === "working" ? "No changes since last commit." : "No file changes."}</StatusMessage>
          )}
          {list.kind === "ready" && list.truncated && <StatusMessage>Showing the first {list.files.length.toLocaleString()} files.</StatusMessage>}
          {stats?.budgetExceeded && <StatusMessage>Line counts stop past the first files; open them to see their changes.</StatusMessage>}
          {compare.kind === "commit" && !mobile && (
            <StatusMessage className="files-pane-hint">Shift-click commits to compare a range.</StatusMessage>
          )}
        </div>
        {files.length > 0 && (
          <FilesTree key={itemHeight} files={files} stats={stats?.byPath ?? null} activePath={activePath} itemHeight={itemHeight}
            label={`Files in ${title}`} collapseRequest={collapseRequest} onExpandedChange={setAnyExpanded} onOpen={onOpen} />
        )}
        {details.kind === "working" && details.warnings.length > 0 && (
          <details className="warnings">
            <summary>Some paths could not be read</summary>
            <ul>{details.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>
          </details>
        )}
      </div>
    </section>
  )
}

// Details shows a commit's message and metadata, a range's span, or the
// branch's merge-base.
function Details({ compare, details }: { compare: Compare; details: CompareDetails }) {
  if (details.kind === "commit" && details.commit) return <CommitDetails commit={details.commit} />
  if (details.kind === "branch" && details.base) {
    return (
      <div className="commit-details files-pane-details">
        <div className="commit-meta commit-compare-meta">
          <span>{plural(details.ahead, "commit")} ahead</span>
          {details.mergeBase && <>
            <span aria-hidden="true">·</span>
            <span>merge-base</span>
            <code className="commit-compare-hash" title={details.mergeBase}>{shortHash(details.mergeBase)}</code>
          </>}
        </div>
      </div>
    )
  }
  if (compare.kind === "range") {
    return (
      <div className="commit-details files-pane-details">
        <div className="commit-meta commit-compare-meta">
          <code className="commit-compare-hash" title={compare.from}>{shortHash(compare.from)}</code>
          <span>through</span>
          <code className="commit-compare-hash" title={compare.to}>{shortHash(compare.to)}</code>
        </div>
      </div>
    )
  }
  return null
}

function CommitDetails({ commit }: { commit: Commit }) {
  const body = useRef<HTMLParagraphElement>(null)
  const [open, setOpen] = useState(false)
  const [clamped, setClamped] = useState(false)
  const copy = useClipboardCopy()
  // Measure whether the closed body overflows its clamp; resizing the
  // sidebar rewraps it.
  useLayoutEffect(() => {
    const element = body.current
    if (!element || open) return
    const measure = () => setClamped(element.scrollHeight > element.clientHeight + 1)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [open, commit.body])
  const date = commitDate(commit.authorTime)
  return (
    <div className="commit-details files-pane-details">
      {commit.body && <>
        <p ref={body} className="commit-body" data-open={open || undefined}>{commit.body}</p>
        {(clamped || open) && (
          <Button variant="link" size="inline" onClick={() => setOpen(!open)}>{open ? "Less" : "More"}</Button>
        )}
      </>}
      <div className="commit-meta">
        <span className="commit-author" title={commit.author.email}>{commit.author.name}</span>
        <span aria-hidden="true">·</span>
        <time dateTime={commit.authorTime}>{date}</time>
        <Button variant="ghost" size="inline" className="commit-hash" aria-label={`Copy commit hash ${commit.hash}`}
          title={`Copy commit hash (${copyShortcut})`} onClick={() => { void copy.copy(commit.hash) }}>
          <span>{shortHash(commit.hash)}</span>
          {copy.status === "copied" ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
        </Button>
      </div>
      <div className="visually-hidden" aria-live="polite">
        {copy.status === "copied" ? "Commit hash copied." : copy.status === "failed" ? "Couldn't copy the commit hash." : ""}
      </div>
    </div>
  )
}
