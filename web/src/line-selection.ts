// LineRange is an inclusive, 1-based range of source lines.
export interface LineRange {
  start: number
  end: number
}

// splitSourceLines splits text into viewer rows, dropping one final newline.
export function splitSourceLines(source: string): string[] {
  const lines = source.split(/\r\n|\r|\n/)
  return lines.length > 1 && lines.at(-1) === "" ? lines.slice(0, -1) : lines
}

const lineAnchorPattern = /^#?L([1-9]\d*)(?:-L?([1-9]\d*))?$/

// parseLineAnchor reads GitHub-style fragments: #L12 or #L12-L20 (#L12-20 too).
export function parseLineAnchor(hash: string): LineRange | null {
  const match = lineAnchorPattern.exec(hash)
  if (!match) return null
  const first = Number(match[1])
  const second = match[2] === undefined ? first : Number(match[2])
  if (!Number.isSafeInteger(first) || !Number.isSafeInteger(second)) return null
  return { start: Math.min(first, second), end: Math.max(first, second) }
}

// lineAnchor formats a range as a fragment without the leading #.
export function lineAnchor(range: LineRange): string {
  return range.start === range.end ? `L${range.start}` : `L${range.start}-L${range.end}`
}

// clampLineRange fits a range to a document, or returns null when it starts past the end.
export function clampLineRange(range: LineRange | null, lineCount: number): LineRange | null {
  if (!range || range.start > lineCount) return null
  return { start: range.start, end: Math.min(range.end, lineCount) }
}

// spanLines is the inclusive range between two lines, in either order.
export function spanLines(anchor: number, line: number): LineRange {
  return { start: Math.min(anchor, line), end: Math.max(anchor, line) }
}

// startLineSelection applies a gutter press: plain presses anchor a new
// selection; Shift extends from the anchor (the last plain press, else the
// current start). Dragging then spans from the returned anchor.
export function startLineSelection(current: LineRange | null, anchor: number | null, line: number, extend: boolean): { anchor: number; range: LineRange } {
  const from = extend ? anchor ?? current?.start ?? line : line
  return { anchor: from, range: spanLines(from, line) }
}

// lineReference formats editor-style references: path:12 or path:12-20.
export function lineReference(path: string, range: LineRange): string {
  return range.start === range.end ? `${path}:${range.start}` : `${path}:${range.start}-${range.end}`
}

// lineRangeLabel formats the status bar position: Ln 12 or Ln 12–20.
export function lineRangeLabel(range: LineRange): string {
  return range.start === range.end ? `Ln ${range.start}` : `Ln ${range.start}–${range.end}`
}
