import { CheckIcon, ChevronRightIcon, CopyIcon, GitCompareArrowsIcon, XIcon } from "lucide-react"
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type MouseEvent } from "react"

import { useClipboardCopy } from "./clipboard.ts"
import { Button } from "./components/ui/button.tsx"
import {
  commitAge, commitDate, commitRows, compareRowKey, folderKey, parentRowKey, tabRowKey,
  type CommitFiles, type CommitRow, type Compare, type PinnedCompare,
} from "./commit-rows.ts"
import type { Commit, CommitFileStatus, CommitRange } from "./commits-contract.ts"
import { FileIcon } from "./FileIcon.tsx"
import type { Commits } from "./use-commits.ts"
import { shortHash, type WorkspaceTab } from "./workspace-tabs.ts"

const statusLetters: Record<CommitFileStatus, string> = { modified: "M", added: "A", deleted: "D", renamed: "R" }
const statusNames: Record<CommitFileStatus, string> = { modified: "Modified", added: "Added", deleted: "Deleted", renamed: "Renamed" }

// indent steps folders and files under their commit; level 2 aligns with
// the details block.
function indent(depth: number): string {
  return `${0.5 + (depth - 1) * 0.75}rem`
}

const mac = typeof navigator !== "undefined" && navigator.platform.includes("Mac")
const copyShortcut = mac ? "⌘C" : "Ctrl+C"

interface CommitsViewProps {
  commits: Commits
  range: CommitRange
  // active is the open commit, branch, or range diff tab, selected in the
  // tree.
  active: WorkspaceTab | null
  // expanded holds the open commits' hashes and compareRowKey while the
  // pinned row is open; Source Control owns it so the Commits header can
  // collapse them.
  expanded: ReadonlySet<string>
  mobile: boolean
  // pinned is the compare row above the commits: the branch, or the selected
  // range (decision 0030); null shows none.
  pinned: PinnedCompare | null
  // selected lists the Shift-selected commits' hashes, null without a range.
  selected: readonly string[] | null
  onClearSelection(): void
  onDismiss(): void
  onExpandedChange(update: (current: ReadonlySet<string>) => ReadonlySet<string>): void
  // onOpen opens a file's diff tab; focusDocument moves focus to it.
  onOpen(tab: WorkspaceTab, focusDocument: boolean): void
  // onSelect selects the listed commits from anchor through focus.
  onSelect(anchor: string, focus: string): void
  onShowAll(): void
}

// CommitsView lists the range's commits as a tree: each expands to its
// message and metadata, then its changed files under compact folders. A
// pinned row above them compares the branch with its base, or spans the
// Shift-selected commits.
export function CommitsView({
  active, commits, expanded, mobile, pinned, selected, onClearSelection, onDismiss, onExpandedChange, onOpen, onSelect, onShowAll, range,
}: CommitsViewProps) {
  const { error, files, loadFiles, loadMore, loadRangeFiles, loadingMore, moreError, refresh, result, retryBranch } = commits
  const tree = useRef<HTMLDivElement>(null)
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set())
  const [focusedKey, setFocusedKey] = useState("")
  const [openBodies, setOpenBodies] = useState<ReadonlySet<string>>(() => new Set())
  const hashCopy = useClipboardCopy()
  const [copied, setCopied] = useState("")
  const list = useMemo(() => result?.commits ?? [], [result])
  const rows = useMemo(() => commitRows(list, expanded, files, collapsed, pinned), [collapsed, expanded, files, list, pinned])
  const activeKey = active ? tabRowKey(active) : ""
  const selectedSet = useMemo(() => new Set(selected ?? []), [selected])
  // compareEnd is the pinned group's last row, which carries its divider.
  const compareEnd = rows.findLastIndex((row) => row.kind === "compare" || row.kind === "compare-details"
    || ((row.kind === "folder" || row.kind === "file") && row.parent === compareRowKey))
  // anchor is the commit a Shift-selection extends from: the last commit
  // clicked or reached without Shift. extending skips the focus that a
  // Shift-click or Shift+Arrow causes.
  const [anchor, setAnchor] = useState("")
  const extending = useRef(false)
  // An open range reads its files once.
  const rangeToLoad = pinned?.compare.kind === "range" && pinned.expanded && !pinned.files ? pinned.compare : null
  const loadFrom = rangeToLoad?.from ?? ""
  const loadTo = rangeToLoad?.to ?? ""
  useEffect(() => {
    if (loadFrom && loadTo) loadRangeFiles(loadFrom, loadTo)
  }, [loadFrom, loadRangeFiles, loadTo])
  // The roving tab stop: the focused row if still shown, else the open file, else the first row.
  const tabKey = rows.some((row) => row.key === focusedKey) ? focusedKey
    : rows.some((row) => row.key === activeKey) ? activeKey : rows[0]?.key ?? ""
  // now is read once per list so ages don't shift between renders.
  const now = useMemo(() => result ? Date.now() : 0, [result])

  function focusRow(key: string) {
    setFocusedKey(key)
    requestAnimationFrame(() => {
      const row = tree.current?.querySelector(`[data-row-key="${CSS.escape(key)}"]`)
      if (row instanceof HTMLElement) row.focus()
    })
  }

  function toggleExpanded(key: string, open: boolean) {
    onExpandedChange((current) => {
      if (current.has(key) === open) return current
      const next = new Set(current)
      if (open) next.add(key)
      else next.delete(key)
      return next
    })
  }

  function toggleCommit(hash: string, open = !expanded.has(hash)) {
    if (open) loadFiles(hash)
    toggleExpanded(hash, open)
  }

  function retryCompare(compare: Compare) {
    if (compare.kind === "branch") retryBranch()
    else loadRangeFiles(compare.from, compare.to, true)
  }

  // extendTo selects from the anchor through hash; without an anchor, hash
  // becomes it.
  function extendTo(hash: string, from = anchor) {
    if (!from) {
      setAnchor(hash)
      return
    }
    if (from !== anchor) setAnchor(from)
    onSelect(from, hash)
  }

  function toggleFolder(owner: string, path: string, open: boolean) {
    const key = folderKey(owner, path)
    setCollapsed((current) => {
      if (current.has(key) === !open) return current
      const next = new Set(current)
      if (open) next.delete(key)
      else next.add(key)
      return next
    })
  }

  function toggleBody(hash: string) {
    setOpenBodies((current) => {
      const next = new Set(current)
      if (next.has(hash)) next.delete(hash)
      else next.add(hash)
      return next
    })
  }

  function copyHash(hash: string) {
    setCopied(hash)
    void hashCopy.copy(hash)
  }

  function open(tab: WorkspaceTab, focusDocument: boolean) {
    onOpen(tab, focusDocument)
    if (mobile) onDismiss()
  }

  function activate(row: CommitRow, focusDocument: boolean) {
    if (row.kind === "commit") toggleCommit(row.commit.hash)
    else if (row.kind === "compare") toggleExpanded(compareRowKey, !row.expanded)
    else if (row.kind === "compare-details") {
      if (row.files?.kind === "error") retryCompare(row.compare)
    } else if (row.kind === "folder") toggleFolder(row.owner, row.path, !row.expanded)
    else if (row.kind === "file") open(row.tab, focusDocument)
    else if (row.files?.kind === "error") loadFiles(row.commit.hash, true)
    else if (row.commit.body) toggleBody(row.commit.hash)
  }

  // nextCommit is the commit row step rows away from index, skipping details,
  // folders, and files.
  function nextCommit(index: number, step: 1 | -1): CommitRow | undefined {
    for (let at = index + step; at >= 0 && at < rows.length; at += step) {
      const row = rows[at]
      if (row?.kind === "commit") return row
    }
    return undefined
  }

  function onTreeKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = rows.findIndex((row) => row.key === tabKey)
    const row = rows[index]
    if (!row) return
    if ((event.key === "c" || event.key === "C") && (event.metaKey || event.ctrlKey) && !event.shiftKey && !event.altKey
      && (row.kind === "commit" || row.kind === "details")) {
      copyHash(row.commit.hash)
    } else if (event.key === "Escape" && selected) {
      onClearSelection()
    } else if ((event.key === "ArrowDown" || event.key === "ArrowUp") && event.shiftKey && !mobile && row.kind === "commit") {
      // Shift+Arrow extends the selection to the next or previous commit.
      const next = nextCommit(index, event.key === "ArrowDown" ? 1 : -1)
      if (next?.kind === "commit") {
        extending.current = true
        focusRow(next.key)
        extendTo(next.commit.hash, anchor || row.commit.hash)
      }
    } else if (event.key === "ArrowDown") {
      const next = rows[index + 1]
      if (next) focusRow(next.key)
    } else if (event.key === "ArrowUp") {
      const previous = rows[index - 1]
      if (previous) focusRow(previous.key)
    } else if (event.key === "Home") {
      const first = rows[0]
      if (first) focusRow(first.key)
    } else if (event.key === "End") {
      const last = rows.at(-1)
      if (last) focusRow(last.key)
    } else if (event.key === "ArrowRight") {
      const next = rows[index + 1]
      if (row.kind === "commit" && !row.expanded) toggleCommit(row.commit.hash, true)
      else if (row.kind === "compare" && !row.expanded) toggleExpanded(compareRowKey, true)
      else if (row.kind === "folder" && !row.expanded) toggleFolder(row.owner, row.path, true)
      else if ((row.kind === "commit" || row.kind === "compare" || row.kind === "folder") && next) focusRow(next.key)
    } else if (event.key === "ArrowLeft") {
      if (row.kind === "commit" && row.expanded) toggleCommit(row.commit.hash, false)
      else if (row.kind === "compare" && row.expanded) toggleExpanded(compareRowKey, false)
      else if (row.kind === "folder" && row.expanded) toggleFolder(row.owner, row.path, false)
      else {
        const parent = parentRowKey(rows, index)
        if (parent) focusRow(parent)
      }
    } else if (event.key === "Enter" || event.key === " ") {
      // As in Changes and Search, Space opens a file and keeps focus here;
      // Enter moves focus to its diff.
      activate(row, event.key === "Enter")
    } else {
      return
    }
    event.preventDefault()
  }

  // Changes owns the view's status region; history failures alert instead.
  const failure = error && <p className="search-status-error" role="alert">{error}</p>
  if (error && !result) {
    return (
      <div className="source-control-message">
        {failure}
        <Button type="button" size="sm" variant="outline" onClick={refresh}>Retry</Button>
      </div>
    )
  }
  if (!result) return null
  if (list.length === 0) {
    const reason = result.range === "all" ? "No commits yet."
      : result.base ? `No commits ahead of ${result.base}.` : "No base branch found to compare with."
    return (
      <div className="source-control-message">
        {failure}
        <p>{reason}</p>
        {result.range === "branch" && range === "branch" && (
          <Button type="button" size="sm" variant="outline" onClick={onShowAll}>Show all history</Button>
        )}
      </div>
    )
  }

  return (
    <div className="commit-list">
      {failure && <div className="source-control-message">{failure}</div>}
      <div ref={tree} className="commit-tree" role="tree" aria-label="Commits" aria-multiselectable={!mobile || undefined}
        onKeyDown={onTreeKeyDown}>
        {rows.map((row, index) => {
          const rowProps = {
            "data-row-key": row.key,
            "data-compare-end": index === compareEnd || undefined,
            role: "treeitem",
            tabIndex: row.key === tabKey ? 0 : -1,
            onFocus: () => setFocusedKey(row.key),
          }
          if (row.kind === "compare") {
            return (
              <CompareRow key={row.key} rowProps={rowProps} compare={row.compare} expanded={row.expanded} files={row.files}
                onActivate={() => activate(row, false)} onClear={(event) => {
                  event.stopPropagation()
                  onClearSelection()
                  focusRow(compareRowKey)
                }} />
            )
          }
          if (row.kind === "compare-details") {
            return (
              <CompareDetails key={row.key} rowProps={rowProps} compare={row.compare} files={row.files}
                onRetry={(event) => {
                  event.stopPropagation()
                  focusRow(row.key)
                  retryCompare(row.compare)
                }} />
            )
          }
          if (row.kind === "commit") {
            const age = commitAge(row.commit.authorTime, now)
            const { hash } = row.commit
            return (
              <div key={row.key} {...rowProps} className="commit-row" aria-level={1} aria-expanded={row.expanded}
                aria-selected={selectedSet.has(hash)}
                aria-label={`${row.commit.subject}, ${row.commit.author.name}, ${age.long}`}
                title={`${row.commit.subject}\n${row.commit.author.name} · ${commitDate(row.commit.authorTime)} · ${shortHash(hash)}`}
                onFocus={() => {
                  setFocusedKey(row.key)
                  if (extending.current) extending.current = false
                  else setAnchor(hash)
                }}
                onMouseDown={(event) => {
                  if (event.shiftKey && !mobile) extending.current = true
                }}
                onClick={(event) => {
                  if (event.shiftKey && !mobile) {
                    extending.current = false
                    extendTo(hash)
                  } else {
                    setAnchor(hash)
                    activate(row, false)
                  }
                }}>
                <ChevronRightIcon className="search-chevron" aria-hidden="true" />
                <span className="commit-subject">{row.commit.subject || "(no message)"}</span>
                <span className="commit-age">{age.short}</span>
              </div>
            )
          }
          if (row.kind === "details") {
            return (
              <CommitDetails key={row.key} rowProps={rowProps} commit={row.commit} files={row.files}
                bodyOpen={openBodies.has(row.commit.hash)} copied={copied === row.commit.hash && hashCopy.status === "copied"}
                onActivate={() => focusRow(row.key)} onCopy={() => copyHash(row.commit.hash)}
                onRetry={() => loadFiles(row.commit.hash, true)} onToggleBody={() => toggleBody(row.commit.hash)} />
            )
          }
          if (row.kind === "folder") {
            return (
              <div key={row.key} {...rowProps} className="commit-row commit-folder" aria-level={row.depth} aria-expanded={row.expanded}
                aria-selected={false} style={{ paddingLeft: indent(row.depth) }} title={row.path}
                onClick={() => activate(row, false)}>
                <ChevronRightIcon className="search-chevron" aria-hidden="true" />
                <span className="commit-file-name">{row.name}</span>
              </div>
            )
          }
          const { file } = row
          return (
            <div key={row.key} {...rowProps} className="commit-row commit-file" aria-level={row.depth}
              aria-selected={row.key === activeKey} aria-label={`${row.name}, ${statusNames[file.status]}`}
              style={{ paddingLeft: indent(row.depth) }}
              title={file.oldPath ? `${file.path} • Renamed from ${file.oldPath}` : `${file.path} • ${statusNames[file.status]}`}
              onClick={() => activate(row, false)} onDoubleClick={() => activate(row, true)}>
              <FileIcon path={file.path} />
              <span className="commit-file-name" data-status={file.status}>{row.name}</span>
              {file.oldPath && <span className="commit-file-from">from {file.oldPath}</span>}
              <span className="commit-file-status" data-status={file.status} aria-hidden="true">{statusLetters[file.status]}</span>
            </div>
          )
        })}
      </div>
      {(result.more || moreError) && (
        <div className="commit-more">
          {moreError && <p className="search-status-error" role="alert">{moreError}</p>}
          <Button type="button" size="sm" variant="outline" disabled={loadingMore} onClick={loadMore}>
            {loadingMore ? "Loading…" : moreError ? "Retry" : "Load more"}
          </Button>
        </div>
      )}
      <div className="visually-hidden" aria-live="polite">
        {hashCopy.status === "copied" ? "Commit hash copied." : hashCopy.status === "failed" ? "Couldn't copy the commit hash." : ""}
      </div>
    </div>
  )
}

interface CommitDetailsProps {
  bodyOpen: boolean
  commit: Commit
  copied: boolean
  files: CommitFiles | undefined
  rowProps: RowProps
  // onActivate focuses the row when a control inside it is clicked.
  onActivate(): void
  onCopy(): void
  onRetry(): void
  onToggleBody(): void
}

// CommitDetails shows the message body, clamped until expanded, then the
// author, date, and hash, then the file list's state.
function CommitDetails({ bodyOpen, commit, copied, files, rowProps, onActivate, onCopy, onRetry, onToggleBody }: CommitDetailsProps) {
  const body = useRef<HTMLParagraphElement>(null)
  const [clamped, setClamped] = useState(false)
  // Measure whether the closed body overflows its clamp; resizing the
  // sidebar rewraps it.
  useLayoutEffect(() => {
    const element = body.current
    if (!element || bodyOpen) return
    const measure = () => setClamped(element.scrollHeight > element.clientHeight + 1)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [bodyOpen, commit.body])
  const stop = (event: MouseEvent) => {
    event.stopPropagation()
    onActivate()
  }
  const date = commitDate(commit.authorTime)
  return (
    <div {...rowProps} className="commit-details" aria-level={2} aria-selected={false}
      aria-label={[commit.subject, commit.body, `${commit.author.name}, ${date}, commit ${shortHash(commit.hash)}`]
        .filter(Boolean).map((part) => /[.!?]$/.test(part) ? part : `${part}.`).join(" ")}>
      {commit.body && <>
        <p ref={body} className="commit-body" data-open={bodyOpen || undefined}>{commit.body}</p>
        {(clamped || bodyOpen) && (
          <button type="button" className="commit-body-toggle" tabIndex={-1} onClick={(event) => {
            stop(event)
            onToggleBody()
          }}>{bodyOpen ? "Less" : "More"}</button>
        )}
      </>}
      <div className="commit-meta">
        <span className="commit-author" title={commit.author.email}>{commit.author.name}</span>
        <span aria-hidden="true">·</span>
        <time dateTime={commit.authorTime}>{date}</time>
        <button type="button" className="commit-hash" tabIndex={-1} aria-label={`Copy commit hash ${commit.hash}`}
          title={`Copy commit hash (${copyShortcut})`} onClick={(event) => {
            stop(event)
            onCopy()
          }}>
          <span>{shortHash(commit.hash)}</span>
          {copied ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
        </button>
      </div>
      <FilesNote files={files} onRetry={(event) => {
        stop(event)
        onRetry()
      }} />
    </div>
  )
}

// FilesNote reports a file list's loading, failure, emptiness, or cap.
function FilesNote({ files, onRetry }: { files: CommitFiles | undefined; onRetry(event: MouseEvent): void }) {
  const count = files?.kind === "ready" ? files.response.files.length : 0
  return <>
    {files?.kind === "loading" && <p className="commit-files-note">Loading files…</p>}
    {files?.kind === "error" && (
      <p className="commit-files-note search-status-error">
        {files.message}{" "}
        <button type="button" className="commit-body-toggle" tabIndex={-1} onClick={onRetry}>Retry</button>
      </p>
    )}
    {files?.kind === "ready" && count === 0 && <p className="commit-files-note">No file changes.</p>}
    {files?.kind === "ready" && files.response.truncated && (
      <p className="commit-files-note">Showing the first {count.toLocaleString()} files.</p>
    )}
  </>
}

type RowProps = { "data-row-key": string; "data-compare-end": true | undefined; role: string; tabIndex: number; onFocus(): void }

function plural(count: number, noun: string): string {
  return `${count.toLocaleString()} ${noun}${count === 1 ? "" : "s"}`
}

interface CompareRowProps {
  compare: Compare
  expanded: boolean
  files: CommitFiles | undefined
  rowProps: RowProps
  onActivate(): void
  // onClear clears the commit selection the row spans.
  onClear(event: MouseEvent): void
}

// CompareRow is the pinned row: “All changes vs main” with its file count, or
// “3 commits” with the selected span and a Clear action.
function CompareRow({ compare, expanded, files, rowProps, onActivate, onClear }: CompareRowProps) {
  const ready = files?.kind === "ready" ? files.response : null
  if (compare.kind === "branch") {
    const count = ready ? `${ready.files.length.toLocaleString()}${ready.truncated ? "+" : ""}` : ""
    return (
      <div {...rowProps} className="commit-row commit-compare" aria-level={1} aria-expanded={expanded} aria-selected={false}
        aria-label={`All changes vs ${compare.base}${ready ? `, ${plural(ready.files.length, "file")}` : ""}`}
        title={`Net changes on this branch since it left ${compare.base}`} onClick={onActivate}>
        <ChevronRightIcon className="search-chevron" aria-hidden="true" />
        <GitCompareArrowsIcon className="commit-compare-icon" aria-hidden="true" />
        <span className="commit-subject">All changes vs {compare.base}</span>
        {count && <span className="search-count" aria-hidden="true">{count}</span>}
      </div>
    )
  }
  const from = shortHash(compare.from)
  const to = shortHash(compare.to)
  return (
    <div {...rowProps} className="commit-row commit-compare" aria-level={1} aria-expanded={expanded} aria-selected={false}
      aria-label={`${plural(compare.count, "commit")} selected, ${from} through ${to}`}
      title={`Changes in ${plural(compare.count, "commit")}, ${from} through ${to}`} onClick={onActivate}>
      <ChevronRightIcon className="search-chevron" aria-hidden="true" />
      <GitCompareArrowsIcon className="commit-compare-icon" aria-hidden="true" />
      <span className="commit-compare-count">{plural(compare.count, "commit")}</span>
      <span className="commit-compare-span">{from}..{to}</span>
      <button type="button" className="commit-compare-clear" tabIndex={-1} aria-label="Clear commit selection"
        title="Clear selection (Escape)" onClick={onClear}>
        <XIcon aria-hidden="true" />
      </button>
    </div>
  )
}

interface CompareDetailsProps {
  compare: Compare
  files: CommitFiles | undefined
  rowProps: RowProps
  onRetry(event: MouseEvent): void
}

// CompareDetails names what the pinned row compares, then its file list's
// state.
function CompareDetails({ compare, files, rowProps, onRetry }: CompareDetailsProps) {
  const label = compare.kind === "branch"
    ? `${plural(compare.ahead, "commit")} ahead of ${compare.base}${compare.mergeBase ? `, merge-base ${shortHash(compare.mergeBase)}` : ""}.`
    : `${plural(compare.count, "commit")}, ${shortHash(compare.from)} through ${shortHash(compare.to)}.`
  return (
    <div {...rowProps} className="commit-details" aria-level={2} aria-selected={false} aria-label={label}>
      <div className="commit-meta commit-compare-meta">
        {compare.kind === "branch" ? <>
          <span>{plural(compare.ahead, "commit")}</span>
          {compare.mergeBase && <>
            <span aria-hidden="true">·</span>
            <span>merge-base</span>
            <code className="commit-compare-hash" title={compare.mergeBase}>{shortHash(compare.mergeBase)}</code>
          </>}
        </> : <>
          <span>{plural(compare.count, "commit")}</span>
          <span aria-hidden="true">·</span>
          <code className="commit-compare-hash" title={compare.from}>{shortHash(compare.from)}</code>
          <span>through</span>
          <code className="commit-compare-hash" title={compare.to}>{shortHash(compare.to)}</code>
        </>}
      </div>
      <FilesNote files={files} onRetry={onRetry} />
    </div>
  )
}
