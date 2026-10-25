import { useRef, useState, type KeyboardEvent } from "react"

import { Button } from "../../components/ui/button.tsx"
import { ListRow } from "../../components/ui/list-row.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import { FileIcon } from "../../components/FileIcon.tsx"
import type { CompareFile, FileList } from "./compare-files.ts"
import type { ChangesView } from "./source-control-layout.ts"
import { fileParts } from "./source-control-panes.ts"
import { FilesTree, type CollapseRequest } from "./FilesTree.tsx"
import { statsLabel, type CompareStats } from "./use-compare-stats.ts"

const statusLetters: Record<CompareFile["status"], string> = { modified: "M", added: "A", untracked: "U", deleted: "D", renamed: "R" }
const statusNames: Record<CompareFile["status"], string> = {
  modified: "Modified", added: "Added", untracked: "Untracked", deleted: "Deleted", renamed: "Renamed",
}

interface UncommittedPaneProps {
  list: FileList
  stats: CompareStats | null
  warnings: readonly string[]
  view: ChangesView
  // activePath is the file in view in the Uncommitted review.
  activePath: string
  itemHeight: number
  collapseRequest: CollapseRequest | null
  onExpandedChange(expanded: boolean): void
  // onOpen reviews a file; focusDocument moves focus to the review.
  onOpen(path: string, focusDocument: boolean): void
  onRetry(): void
}

// UncommittedPane lists the working tree's changes (decision 0040): a flat
// list of names and folders by default, or a tree. Opening a file shows it in
// the Uncommitted review.
export function UncommittedPane({
  activePath, collapseRequest, itemHeight, list, stats, view, warnings, onExpandedChange, onOpen, onRetry,
}: UncommittedPaneProps) {
  return (
    <div className="changes-pane">
      {list.kind === "loading" && <div className="source-control-message"><StatusMessage>Reading changes…</StatusMessage></div>}
      {list.kind === "error" && (
        <div className="source-control-message" role="alert">
          <StatusMessage tone="error">{list.message}</StatusMessage>
          <Button variant="outline" onClick={onRetry}>Retry</Button>
        </div>
      )}
      {list.kind === "ready" && list.files.length === 0 && (
        <div className="source-control-message"><StatusMessage>No changes since last commit.</StatusMessage></div>
      )}
      {list.kind === "ready" && list.truncated && (
        <div className="source-control-message"><StatusMessage>Showing the first {list.files.length.toLocaleString()} changes.</StatusMessage></div>
      )}
      {list.kind === "ready" && list.files.length > 0 && (view === "tree"
        ? <FilesTree key={itemHeight} files={list.files} stats={stats?.byPath ?? null} activePath={activePath} itemHeight={itemHeight}
          label="Uncommitted changes" collapseRequest={collapseRequest} onExpandedChange={onExpandedChange} onOpen={onOpen} />
        : <FlatFiles files={list.files} stats={stats} activePath={activePath} onOpen={onOpen} />)}
      {warnings.length > 0 && (
        <details className="warnings">
          <summary>Some paths could not be read</summary>
          <ul>{warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>
        </details>
      )}
    </div>
  )
}

interface FlatFilesProps {
  files: readonly CompareFile[]
  stats: CompareStats | null
  activePath: string
  onOpen(path: string, focusDocument: boolean): void
}

// FlatFiles lists changed files by path, as VS Code's list view does: click
// or Space opens and keeps focus; Enter and double-click move it to the
// review.
function FlatFiles({ activePath, files, stats, onOpen }: FlatFilesProps) {
  const container = useRef<HTMLDivElement>(null)
  const [focused, setFocused] = useState("")
  const tabPath = files.some((file) => file.path === focused) ? focused
    : files.some((file) => file.path === activePath) ? activePath : files[0]?.path ?? ""

  function focusRow(path: string) {
    setFocused(path)
    const row = container.current?.querySelector(`[data-path="${CSS.escape(path)}"]`)
    if (row instanceof HTMLElement) row.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = files.findIndex((file) => file.path === tabPath)
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      const next = files[index + (event.key === "ArrowDown" ? 1 : -1)]
      if (next) focusRow(next.path)
    } else if (event.key === "Home" || event.key === "End") {
      const next = event.key === "Home" ? files[0] : files.at(-1)
      if (next) focusRow(next.path)
    } else if (event.key === " " || event.key === "Enter") {
      if (tabPath) onOpen(tabPath, event.key === "Enter")
    } else {
      return
    }
    event.preventDefault()
  }

  return (
    <div ref={container} className="changes-list" role="listbox" aria-label="Uncommitted changes" onKeyDown={onKeyDown}>
      {files.map((file) => {
        const { name, folder } = fileParts(file.path)
        const lines = statsLabel(stats?.byPath.get(file.path))
        const selected = file.path === activePath
        return (
          <ListRow key={file.path} data-path={file.path} role="option" aria-selected={selected} selected={selected}
            tabIndex={file.path === tabPath ? 0 : -1} className="changes-row" data-status={file.status}
            aria-label={`${file.path}, ${statusNames[file.status]}${lines ? `, ${lines}` : ""}`}
            title={file.oldPath ? `${file.path}\nRenamed from ${file.oldPath}` : file.path}
            onFocus={() => setFocused(file.path)}
            onClick={() => onOpen(file.path, false)}
            onDoubleClick={() => onOpen(file.path, true)}>
            <FileIcon path={file.path} />
            <span className="changes-row-name">{name}</span>
            <span className="changes-row-folder">{folder}</span>
            {lines && <span className="changes-row-stats">{lines}</span>}
            <span className="commit-file-status" data-status={file.status} aria-hidden="true">{statusLetters[file.status]}</span>
          </ListRow>
        )
      })}
    </div>
  )
}
