import { identifierAt, type ColumnRange, type SourcePosition } from "./code-navigation.ts"

// SourceTarget is the word under the pointer in Source. position points at
// the word's start when there is one, so the server sees the whole name.
export interface SourceTarget {
  position: SourcePosition
  word: ColumnRange | null
  text: string
}

interface Caret {
  node: Node
  offset: number
}

function caretAt(x: number, y: number): Caret | null {
  if (typeof document.caretPositionFromPoint === "function") {
    const caret = document.caretPositionFromPoint(x, y)
    return caret ? { node: caret.offsetNode, offset: caret.offset } : null
  }
  if (typeof document.caretRangeFromPoint === "function") {
    const range = document.caretRangeFromPoint(x, y)
    return range ? { node: range.startContainer, offset: range.startOffset } : null
  }
  return null
}

// lineContent is the code of the line holding node, outside the gutter.
function lineContent(node: Node): { content: Element; line: number } | null {
  const element = node instanceof Element ? node : node.parentElement
  const content = element?.closest(".source-document-line-content")
  const line = Number(content?.closest("[data-line]")?.getAttribute("data-line"))
  return content && Number.isInteger(line) && line > 0 ? { content, line } : null
}

// targetAt reads the line and UTF-16 column under a point within root.
export function targetAt(root: Element, x: number, y: number): SourceTarget | null {
  const caret = caretAt(x, y)
  if (!caret || !root.contains(caret.node)) return null
  const found = lineContent(caret.node)
  if (!found) return null
  const before = document.createRange()
  before.setStart(found.content, 0)
  before.setEnd(caret.node, caret.offset)
  const column = before.toString().length
  const text = found.content.textContent ?? ""
  const word = identifierAt(text, column)
  return {
    position: { line: found.line, column: word?.start ?? column },
    word,
    text: word ? text.slice(word.start, word.end) : "",
  }
}

// wordRange spans columns of a line's text across its highlight spans.
export function wordRange(root: Element, line: number, word: ColumnRange): Range | null {
  const content = root.querySelector(`[data-line="${line}"] .source-document-line-content`)
  if (!content) return null
  const walker = document.createTreeWalker(content, NodeFilter.SHOW_TEXT)
  const range = document.createRange()
  let seen = 0
  let started = false
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const length = node.textContent?.length ?? 0
    if (!started && word.start <= seen + length) {
      range.setStart(node, word.start - seen)
      started = true
    }
    if (started && word.end <= seen + length) {
      range.setEnd(node, word.end - seen)
      return range
    }
    seen += length
  }
  return null
}

const highlightName = "servef-navigation-link"

// showLink underlines a navigable word with the CSS Custom Highlight API,
// leaving the highlighted HTML untouched; browsers without it show none.
export function showLink(range: Range | null) {
  if (typeof CSS === "undefined" || !("highlights" in CSS)) return
  if (range) CSS.highlights.set(highlightName, new Highlight(range))
  else CSS.highlights.delete(highlightName)
}
