import { CheckIcon, ChevronRightIcon, CopyIcon } from "lucide-react"
import { useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type MouseEvent } from "react"

import { useClipboardCopy } from "./clipboard.ts"
import { Button } from "./components/ui/button.tsx"
import { commitAge, commitDate, commitRowKey, commitRows, fileRowKey, folderKey, parentRowKey, type CommitFiles, type CommitRow } from "./commit-rows.ts"
import type { Commit, CommitFileStatus, CommitRange } from "./commits-contract.ts"
import { FileIcon } from "./FileIcon.tsx"
import type { Commits } from "./use-commits.ts"
import { shortHash } from "./workspace-tabs.ts"

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
  // active is the open commit diff tab, selected in the tree.
  active: { commit: string; path: string } | null
  mobile: boolean
  onDismiss(): void
  // onOpen opens a commit file's diff tab; focusDocument moves focus to it.
  onOpen(commit: string, path: string, focusDocument: boolean): void
  onShowAll(): void
}

// CommitsView lists the range's commits as a tree: each expands to its
// message and metadata, then its changed files under compact folders.
export function CommitsView({ active, commits, mobile, onDismiss, onOpen, onShowAll, range }: CommitsViewProps) {
  const { error, files, loadFiles, loadMore, loadingMore, moreError, refresh, result } = commits
  const tree = useRef<HTMLDivElement>(null)
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set())
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set())
  const [focusedKey, setFocusedKey] = useState("")
  const [openBodies, setOpenBodies] = useState<ReadonlySet<string>>(() => new Set())
  const hashCopy = useClipboardCopy()
  const [copied, setCopied] = useState("")
  const list = useMemo(() => result?.commits ?? [], [result])
  const rows = useMemo(() => commitRows(list, expanded, files, collapsed), [collapsed, expanded, files, list])
  const activeKey = active ? fileRowKey(active.commit, active.path) : ""
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

  function toggleCommit(hash: string, open = !expanded.has(hash)) {
    if (open) loadFiles(hash)
    setExpanded((current) => {
      if (current.has(hash) === open) return current
      const next = new Set(current)
      if (open) next.add(hash)
      else next.delete(hash)
      return next
    })
  }

  function toggleFolder(hash: string, path: string, open: boolean) {
    const key = folderKey(hash, path)
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

  function open(hash: string, path: string, focusDocument: boolean) {
    onOpen(hash, path, focusDocument)
    if (mobile) onDismiss()
  }

  function activate(row: CommitRow, focusDocument: boolean) {
    if (row.kind === "commit") toggleCommit(row.commit.hash)
    else if (row.kind === "folder") toggleFolder(row.commit.hash, row.path, !row.expanded)
    else if (row.kind === "file") open(row.commit.hash, row.file.path, focusDocument)
    else if (row.files?.kind === "error") loadFiles(row.commit.hash, true)
    else if (row.commit.body) toggleBody(row.commit.hash)
  }

  function onTreeKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = rows.findIndex((row) => row.key === tabKey)
    const row = rows[index]
    if (!row) return
    if ((event.key === "c" || event.key === "C") && (event.metaKey || event.ctrlKey) && !event.shiftKey && !event.altKey
      && (row.kind === "commit" || row.kind === "details")) {
      copyHash(row.commit.hash)
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
      else if (row.kind === "folder" && !row.expanded) toggleFolder(row.commit.hash, row.path, true)
      else if ((row.kind === "commit" || row.kind === "folder") && next) focusRow(next.key)
    } else if (event.key === "ArrowLeft") {
      if (row.kind === "commit" && row.expanded) toggleCommit(row.commit.hash, false)
      else if (row.kind === "folder" && row.expanded) toggleFolder(row.commit.hash, row.path, false)
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
      <div ref={tree} className="commit-tree" role="tree" aria-label="Commits" onKeyDown={onTreeKeyDown}>
        {rows.map((row) => {
          const rowProps = {
            "data-row-key": row.key,
            role: "treeitem",
            tabIndex: row.key === tabKey ? 0 : -1,
            onFocus: () => setFocusedKey(row.key),
          }
          if (row.kind === "commit") {
            const age = commitAge(row.commit.authorTime, now)
            return (
              <div key={row.key} {...rowProps} className="commit-row" aria-level={1} aria-expanded={row.expanded} aria-selected={false}
                aria-label={`${row.commit.subject}, ${row.commit.author.name}, ${age.long}`}
                title={`${row.commit.subject}\n${row.commit.author.name} · ${commitDate(row.commit.authorTime)} · ${shortHash(row.commit.hash)}`}
                onClick={() => activate(row, false)}>
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
  rowProps: { "data-row-key": string; role: string; tabIndex: number; onFocus(): void }
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
  const count = files?.kind === "ready" ? files.response.files.length : 0
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
      {files?.kind === "loading" && <p className="commit-files-note">Loading files…</p>}
      {files?.kind === "error" && (
        <p className="commit-files-note search-status-error">
          {files.message}{" "}
          <button type="button" className="commit-body-toggle" tabIndex={-1} onClick={(event) => {
            stop(event)
            onRetry()
          }}>Retry</button>
        </p>
      )}
      {files?.kind === "ready" && count === 0 && <p className="commit-files-note">No file changes.</p>}
      {files?.kind === "ready" && files.response.truncated && (
        <p className="commit-files-note">Showing the first {count.toLocaleString()} files.</p>
      )}
    </div>
  )
}
