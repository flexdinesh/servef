import { Fragment, memo, useCallback, useEffect, useLayoutEffect, useRef, useState, type MouseEvent, type PointerEvent, type ReactNode } from "react"

import { CopyButton } from "../../components/ui/copy-button.tsx"
import type { ColumnRange, NavigationKind } from "../navigation/code-navigation.ts"
import { clampLineRange, lineReference, spanLines, splitSourceLines, startLineSelection, type LineRange } from "../../lib/line-selection.ts"
import { showLink, targetAt, wordRange, type SourceTarget } from "../navigation/source-position.ts"
import { useHighlightedLines } from "../../highlighting/use-highlighted-lines.ts"

// LineSelection connects gutter selection to its owner (the URL fragment).
export interface LineSelection {
  path: string
  range: LineRange | null
  onChange(range: LineRange | null): void
}

// SourceNavigation connects names in code to go to definition and find
// references. A plain click remembers the name for F12 and the command
// palette; Ctrl/Cmd+click goes to its definition.
export interface SourceNavigation {
  onPoint(target: SourceTarget): void
  // onNavigate runs from target, or from the remembered name when null.
  onNavigate(kind: NavigationKind, target: SourceTarget | null): void
}

// SourceAnnotation is a full-width zone inserted after a line, or before the
// first when line is 0, e.g. a walkthrough step's narration. render reads the
// source's lines by number; views without them pass null and show the zone
// at their top.
export interface SourceAnnotation {
  line: number
  render(lineAt: ((line: number) => string | undefined) | null): ReactNode
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
  // language is the server's Highlights language; null shows plain text.
  language: string | null
  label: string
  selection?: LineSelection
  navigation?: SourceNavigation
  annotation?: SourceAnnotation
}

// LineCopyButton copies a line reference from the gutter. Its presses stay
// out of the gutter, which selects lines.
export function LineCopyButton({ reference }: { reference: string }) {
  const stop = (event: { stopPropagation(): void }) => event.stopPropagation()
  return (
    <span className="source-line-copy" onPointerDown={stop} onMouseDown={stop} onClick={stop}>
      <CopyButton text={reference} label={`Copy ${reference}`} variant="gutter" className="source-line-copy-button" />
    </span>
  )
}

function modified(event: { metaKey: boolean; ctrlKey: boolean }): boolean {
  return event.metaKey || event.ctrlKey
}

// Keep text nodes stable during hover and selection updates. Reassigning
// highlighted HTML clears the browser's native text selection.
const SourceLineContent = memo(function SourceLineContent({ line, highlighted }: { line: string; highlighted: string | undefined }) {
  return highlighted !== undefined
    ? <span className="source-document-line-content" dangerouslySetInnerHTML={{ __html: highlighted }} />
    : <span className="source-document-line-content">{line}</span>
})

export function SourceDocument({ source, language, label, selection, navigation, annotation }: SourceDocumentProps) {
  const lines = splitSourceLines(source)
  // Rows show plain text until highlighting arrives.
  const highlightedLines = useHighlightedLines(source, language)
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
  const pointer = useRef<{ x: number; y: number } | null>(null)
  const [linked, setLinked] = useState<{ line: number; word: ColumnRange } | null>(null)

  // link marks the name under the pointer while Ctrl/Cmd is held.
  const link = useCallback((active: boolean) => {
    const root = container.current
    const target = active && root && pointer.current ? targetAt(root, pointer.current.x, pointer.current.y) : null
    const word = target?.word ?? null
    setLinked((current) => {
      if (!target || !word) return null
      return current?.line === target.position.line && current.word.start === word.start ? current : { line: target.position.line, word }
    })
  }, [])

  // Rendering may replace the highlighted text nodes, so the underline's
  // range is rebuilt from the DOM after every render.
  useLayoutEffect(() => {
    const root = container.current
    showLink(root && linked ? wordRange(root, linked.line, linked.word) : null)
  })

  useEffect(() => {
    if (!navigation) return
    const key = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Meta" || event.key === "Control") link(event.type === "keydown")
    }
    const clear = () => link(false)
    window.addEventListener("keydown", key)
    window.addEventListener("keyup", key)
    window.addEventListener("blur", clear)
    return () => {
      window.removeEventListener("keydown", key)
      window.removeEventListener("keyup", key)
      window.removeEventListener("blur", clear)
      showLink(null)
    }
  }, [link, navigation])

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

  function clickCode(event: MouseEvent) {
    const root = container.current
    if (!navigation || !root || event.button !== 0) return
    const target = targetAt(root, event.clientX, event.clientY)
    if (!target) return
    if (modified(event)) {
      event.preventDefault()
      link(false)
      navigation.onNavigate("definition", target)
    } else {
      navigation.onPoint(target)
    }
  }

  // The annotation sits after its line, or at the top past the last line.
  const annotationLine = annotation ? (annotation.line <= lines.length ? annotation.line : 0) : -1
  const zone = (line: number) => annotationLine === line && annotation
    ? <span className="source-annotation">{annotation.render((number) => lines[number - 1])}</span>
    : null

  return (
    <div
      ref={container}
      className={`source-document${drag?.active ? " selecting-lines" : ""}${linked ? " navigation-linking" : ""}`}
      role="textbox"
      aria-label={label}
      aria-multiline="true"
      aria-readonly="true"
      tabIndex={0}
      onKeyDown={(event) => {
        if (navigation && event.key === "F12" && !event.altKey && !modified(event)) {
          event.preventDefault()
          navigation.onNavigate(event.shiftKey ? "references" : "definition", null)
          return
        }
        if (event.key !== "Escape" || !range || !selection) return
        event.preventDefault()
        selectionAnchor.current = null
        selection.onChange(null)
      }}
    >
      <code
        onPointerOver={dragOver}
        onClick={clickCode}
        onPointerMove={navigation ? (event) => {
          pointer.current = { x: event.clientX, y: event.clientY }
          link(modified(event))
        } : undefined}
        onPointerLeave={navigation ? () => {
          pointer.current = null
          link(false)
        } : undefined}
      >
        {zone(0)}
        {lines.map((line, index) => {
          const lineNumber = index + 1
          const selected = shown !== null && lineNumber >= shown.start && lineNumber <= shown.end
          return (
            <Fragment key={lineNumber}>
            <span
              className={`source-document-line${activeLine === lineNumber ? " active" : ""}${selected ? " selected" : ""}`}
              data-line={lineNumber}
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
              <SourceLineContent line={line} highlighted={highlightedLines[index]} />
            </span>
            {zone(lineNumber)}
            </Fragment>
          )
        })}
      </code>
    </div>
  )
}
