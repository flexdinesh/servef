import { CheckIcon, CopyIcon } from "lucide-react"
import { useRef, useState, type MouseEvent } from "react"

import { useClipboardCopy } from "./clipboard.ts"
import { clampLineRange, lineReference, selectLine, splitSourceLines, type LineRange } from "./line-selection.ts"

// LineSelection connects gutter selection to its owner (the URL fragment).
export interface LineSelection {
  path: string
  range: LineRange | null
  onChange(range: LineRange | null): void
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
  const gutterWidth = `calc(${String(lines.length).length}ch + 36px)`
  const range = clampLineRange(selection?.range ?? null, lines.length)

  function selectFromGutter(event: MouseEvent, lineNumber: number) {
    if (!selection) return
    const extend = event.shiftKey
    if (!extend || selectionAnchor.current === null && !range) selectionAnchor.current = lineNumber
    selection.onChange(selectLine(range, selectionAnchor.current, lineNumber, extend))
  }

  return (
    <div
      className="source-document syntax-highlight"
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
      <code>
        {lines.map((line, index) => {
          const lineNumber = index + 1
          const selected = range !== null && lineNumber >= range.start && lineNumber <= range.end
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
                  // Shift-click extends lines, not the native text selection.
                  if (event.shiftKey && selection) event.preventDefault()
                }}
                onClick={(event) => selectFromGutter(event, lineNumber)}
              >
                <span aria-hidden="true">{lineNumber}</span>
                {selection && range?.start === lineNumber && (
                  <LineCopyButton reference={lineReference(selection.path, range)} />
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
