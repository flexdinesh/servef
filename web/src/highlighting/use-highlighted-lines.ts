import { useEffect, useState } from "react"

import { createSourceHighlighter } from "./source-highlighter.ts"

// One worker highlights large Source and Format text for every tab.
const highlighter = createSourceHighlighter(() => new Worker(new URL("./highlight-worker.ts", import.meta.url), { type: "module" }))

// releaseHighlightResources ends the highlight worker once no tab needs it;
// the next large file starts a fresh one.
export function releaseHighlightResources(): void {
  highlighter.release()
}

const noLines: readonly string[] = []

interface Highlighted {
  source: string
  language: string
  lines: readonly string[]
}

// useHighlightedLines returns escaped HTML per row of source once highlighted,
// and no lines meanwhile, when language is null, or when highlighting is not
// possible; the viewer then shows plain text.
export function useHighlightedLines(source: string, language: string | null): readonly string[] {
  const [highlighted, setHighlighted] = useState<Highlighted | null>(null)

  useEffect(() => {
    if (language === null) return
    const controller = new AbortController()
    void highlighter.highlight(source, language, controller.signal).then((lines) => {
      if (!controller.signal.aborted) setHighlighted({ source, language, lines: lines ?? noLines })
    })
    return () => controller.abort()
  }, [language, source])

  return highlighted && highlighted.source === source && highlighted.language === language ? highlighted.lines : noLines
}
