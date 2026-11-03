import type { HighlightRange } from "./search-engine.ts"

export interface HighlightedPart {
  highlighted: boolean
  text: string
}

export function highlightedParts(text: string, highlights: readonly HighlightRange[] = []): HighlightedPart[] {
  const parts: HighlightedPart[] = []
  let cursor = 0
  for (const range of highlights) {
    const start = Math.max(cursor, Math.min(text.length, range.start))
    const end = Math.max(start, Math.min(text.length, range.end))
    if (start > cursor) parts.push({ highlighted: false, text: text.slice(cursor, start) })
    if (end > start) parts.push({ highlighted: true, text: text.slice(start, end) })
    cursor = end
  }
  if (cursor < text.length) parts.push({ highlighted: false, text: text.slice(cursor) })
  return parts
}
