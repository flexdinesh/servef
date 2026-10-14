import { CheckIcon, CopyIcon } from "lucide-react"
import { useEffect, useRef, useState, type PointerEvent } from "react"

import { useClipboardCopy } from "./clipboard.ts"
import { clampLineRange, lineReference, spanLines, splitSourceLines, startLineSelection, type LineRange } from "./line-selection.ts"

// LineSelection connects gutter selection to its owner (the URL fragment).
export interface LineSelection {
  path: string
  range: LineRange | null
  onChange(range: LineRange | null): void
}

// LineDrag previews a gutter selection until pointer release commits it.
interface LineDrag {
  anchor: number
  range: LineRange
  active: boolean
}

function rangeKey(range: LineRange | null): string {
  return range ? `${range.start}-${range.end}` : ""
}

interface SourceDocumentProps {
  source: string
  highlightedLines: readonly string[]
  label: string
  selection?: LineSelection
}

function LineCopyButton({ reference }: { reference: string }) {
  const { status, copy } = useClipboardCopy()
  const Icon = status === "copied" ? CheckIcon : CopyIcon
  return (
    <>
      <button
        type="button"
        className="source-line-copy-button"
        onPointerDown={(event) => event.stopPropagation()}
        onMouseDown={(event) => event.stopPropagation()}
        aria-label={`Copy ${reference}`}
        title={`Copy ${reference}`}
        onClick={(event) => {
          event.stopPropagation()
          void copy(reference)
        }}
      >
        <Icon aria-hidden="true" />
      </button>
      {status !== "idle" && (
        <span className={`source-line-copy-status ${status}`} role="status">
          {status === "copied" ? "Copied" : "Copy failed"}
        </span>
      )}
    </>
  )
}

export function SourceDocument({ source, highlightedLines, label, selection }: SourceDocumentProps) {
  const lines = splitSourceLines(source)
  const [activeLine, setActiveLine] = useState<number | null>(null)
  const selectionAnchor = useRef<number | null>(null)
  const container = useRef<HTMLDivElement>(null)
  const gutterWidth = `calc(${String(lines.length).length}ch + 36px)`
  const range = clampLineRange(selection?.range ?? null, lines.length)
  const [drag, setDrag] = useState<LineDrag | null>(null)
  // A committed drag shows until the URL catches up, then the URL rules again.
  const [seenRange, setSeenRange] = useState(rangeKey(range))
  if (seenRange !== rangeKey(range)) {
    setSeenRange(rangeKey(range))
    if (drag && !drag.active) setDrag(null)
  }
  const shown = drag?.range ?? range

  useEffect(() => {
    if (!drag?.active || !selection) return
    const finish = () => {
      if (rangeKey(drag.range) === rangeKey(range)) {
        setDrag(null)
        return
      }
      setDrag({ ...drag, active: false })
      selection.onChange(drag.range)
    }
    const cancel = () => setDrag(null)
    window.addEventListener("pointerup", finish)
    window.addEventListener("pointercancel", cancel)
    return () => {
      window.removeEventListener("pointerup", finish)
      window.removeEventListener("pointercancel", cancel)
    }
  }, [drag, range, selection])

  function pressGutter(event: PointerEvent, lineNumber: number) {
    if (!selection || event.button !== 0) return
    container.current?.focus({ preventScroll: true })
    const next = startLineSelection(range, selectionAnchor.current, lineNumber, event.shiftKey)
    selectionAnchor.current = next.anchor
    // Touch keeps scrolling the page; it selects on tap without dragging.
    if (event.pointerType === "touch") selection.onChange(next.range)
    else setDrag({ ...next, active: true })
  }

  function dragOver(event: PointerEvent) {
    if (!drag?.active) return
    const row = event.target instanceof Element ? event.target.closest("[data-line]") : null
    const line = Number(row?.getAttribute("data-line"))
    if (!Number.isInteger(line) || line < 1) return
    const next = spanLines(drag.anchor, line)
    if (rangeKey(next) !== rangeKey(drag.range)) setDrag({ ...drag, range: next })
  }

  return (
    <div
      ref={container}
      className={`source-document syntax-highlight${drag?.active ? " selecting-lines" : ""}`}
      role="textbox"
      aria-label={label}
      aria-multiline="true"
      aria-readonly="true"
      tabIndex={0}
      onKeyDown={(event) => {
        if (event.key !== "Escape" || !range || !selection) return
        event.preventDefault()
        selectionAnchor.current = null
        selection.onChange(null)
      }}
    >
      <code onPointerOver={dragOver}>
        {lines.map((line, index) => {
          const lineNumber = index + 1
          const selected = shown !== null && lineNumber >= shown.start && lineNumber <= shown.end
          return (
            <span
              className={`source-document-line${activeLine === lineNumber ? " active" : ""}${selected ? " selected" : ""}`}
              data-line={lineNumber}
              key={lineNumber}
              onClick={() => setActiveLine(lineNumber)}
            >
              <span
                className="source-document-line-number"
                style={{ width: gutterWidth }}
                onMouseDown={(event) => {
                  // Gutter presses select lines, not text; focus moves explicitly.
                  if (selection) event.preventDefault()
                }}
                onPointerDown={(event) => pressGutter(event, lineNumber)}
              >
                <span aria-hidden="true">{lineNumber}</span>
                {selection && !drag?.active && shown?.start === lineNumber && (
                  <LineCopyButton reference={lineReference(selection.path, shown)} />
                )}
              </span>
              {highlightedLines[index] !== undefined
                ? <span className="source-document-line-content" dangerouslySetInnerHTML={{ __html: highlightedLines[index] ?? "" }} />
                : <span className="source-document-line-content">{line}</span>}
            </span>
          )
        })}
      </code>
    </div>
  )
}
