import { FoldVerticalIcon, PanelRightCloseIcon, UnfoldVerticalIcon, XIcon } from "lucide-react"
import { useEffect, useRef, useState, type ReactNode } from "react"

import { Badge } from "../../components/ui/badge.tsx"
import { Button } from "../../components/ui/button.tsx"
import { DialogShell } from "../../components/ui/dialog-shell.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import { FilesTree, type CollapseRequest } from "./FilesTree.tsx"
import type { CompareData } from "./use-compare-data.ts"

interface FilesListProps {
  data: CompareData | null
  activePath: string
  itemHeight: number
  // marked are files with uncommitted edits in a branch review that
  // includes them (decision 0051).
  marked?: ReadonlySet<string>
  // close is the header's dismiss action: hide the rail, or close the sheet.
  close: ReactNode
  onOpen(path: string, focusDocument: boolean): void
}

// FilesList is a review's files as a tree under a header with a count and
// Expand/Collapse All Folders (decision 0040).
function FilesList({ activePath, close, data, itemHeight, marked, onOpen }: FilesListProps) {
  const [anyExpanded, setAnyExpanded] = useState(true)
  const [collapseRequest, setCollapseRequest] = useState<CollapseRequest | null>(null)
  const list = data?.list
  const files = list?.kind === "ready" ? list.files : []
  return <>
    <div className="review-rail-header">
      <h2 className="review-rail-title">Files</h2>
      {list?.kind === "ready" && (
        <Badge aria-label={`${files.length.toLocaleString()} ${files.length === 1 ? "file" : "files"}`}>
          {files.length.toLocaleString()}{list.truncated ? "+" : ""}
        </Badge>
      )}
      <span className="review-rail-actions">
        <IconButton size="xs" label={anyExpanded ? "Collapse All Folders" : "Expand All Folders"}
          icon={anyExpanded ? <FoldVerticalIcon aria-hidden="true" /> : <UnfoldVerticalIcon aria-hidden="true" />}
          disabled={files.length === 0}
          onClick={() => setCollapseRequest({ expand: !anyExpanded, id: (collapseRequest?.id ?? 0) + 1 })} />
        {close}
      </span>
    </div>
    <div className="review-rail-body">
      {(!list || list.kind === "loading") && <div className="source-control-message"><StatusMessage>Loading files…</StatusMessage></div>}
      {list?.kind === "error" && (
        <div className="source-control-message" role="alert">
          <StatusMessage tone="error">{list.message}</StatusMessage>
          <Button variant="outline" onClick={data?.retry}>Retry</Button>
        </div>
      )}
      {list?.kind === "ready" && files.length === 0 && <div className="source-control-message"><StatusMessage>No file changes.</StatusMessage></div>}
      {files.length > 0 && (
        <FilesTree key={itemHeight} files={files} stats={data?.stats?.byPath ?? null} marked={marked} activePath={activePath} itemHeight={itemHeight}
          label="Files in this review" collapseRequest={collapseRequest} onExpandedChange={setAnyExpanded} onOpen={onOpen} />
      )}
    </div>
  </>
}

const widthStep = 1

interface ReviewFilesRailProps extends Omit<FilesListProps, "close"> {
  // width is the rail's width in rem.
  width: number
  onResize(width: number): void
  onResetWidth(): void
  onHide(): void
}

function remSize(): number {
  const size = Number.parseFloat(getComputedStyle(document.documentElement).fontSize)
  return Number.isFinite(size) && size > 0 ? size : 16
}

// ReviewFilesRail is a commit, range, or branch review's files beside its
// diffs: full height, resizable from its leading edge, and hideable.
export function ReviewFilesRail({ width, onHide, onResetWidth, onResize, ...list }: ReviewFilesRailProps) {
  const dragging = useRef<{ pointer: number; x: number; width: number } | null>(null)
  return (
    <aside className="review-rail" aria-label="Review files" style={{ width: `${width}rem` }}>
      <div className="review-rail-resizer" role="separator" aria-orientation="vertical" aria-label="Resize review files"
        aria-valuenow={Math.round(width)} tabIndex={0}
        onDoubleClick={onResetWidth}
        onKeyDown={(event) => {
          if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return
          event.preventDefault()
          onResize(width + (event.key === "ArrowLeft" ? widthStep : -widthStep))
        }}
        onPointerDown={(event) => {
          dragging.current = { pointer: event.pointerId, x: event.clientX, width }
          event.currentTarget.setPointerCapture(event.pointerId)
        }}
        onPointerMove={(event) => {
          const drag = dragging.current
          if (!drag || drag.pointer !== event.pointerId) return
          // The rail grows as its leading edge moves left.
          onResize(drag.width + (drag.x - event.clientX) / remSize())
        }}
        onPointerUp={(event) => {
          if (dragging.current?.pointer !== event.pointerId) return
          dragging.current = null
          if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
        }}
        onPointerCancel={() => { dragging.current = null }} />
      <div className="review-rail-panel">
        <FilesList {...list}
          close={<IconButton size="xs" label="Hide Files" icon={<PanelRightCloseIcon aria-hidden="true" />} onClick={onHide} />} />
      </div>
    </aside>
  )
}

interface ReviewFilesSheetProps extends Omit<FilesListProps, "close"> {
  open: boolean
  onClose(): void
}

// ReviewFilesSheet is the rail on phones: a bottom sheet the toolbar's Files
// button opens; choosing a file jumps to it and closes the sheet.
export function ReviewFilesSheet({ open, onClose, onOpen, ...list }: ReviewFilesSheetProps) {
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const element = dialog.current
    if (!element) return
    if (open && !element.open) element.showModal()
    if (!open && element.open) element.close()
  }, [open])
  return (
    <DialogShell ref={dialog} variant="sheet" className="review-sheet" aria-label="Review files"
      onClose={onClose} onClick={(event) => { if (event.target === dialog.current) onClose() }}>
      {open && <FilesList {...list}
        close={<IconButton size="xs" label="Close files" icon={<XIcon aria-hidden="true" />} onClick={onClose} />}
        onOpen={(path, focusDocument) => {
          onOpen(path, focusDocument)
          onClose()
        }} />}
    </DialogShell>
  )
}
