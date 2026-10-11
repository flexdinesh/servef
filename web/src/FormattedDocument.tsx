import { useMemo } from "react"

import { SourceDocument } from "./SourceDocument.tsx"

interface FormattedDocumentProps {
  html: string
  label: string
}

export function FormattedDocument({ html, label }: FormattedDocumentProps) {
  const formatted = useMemo(() => {
    // Code previews contain trusted, escaped backend HTML with complete syntax
    // spans per line. Reuse those rows without formatting or detecting languages.
    const preview = new DOMParser().parseFromString(html, "text/html")
    const code = preview.querySelector("pre > code")
    return {
      // HighlightLines already omits the terminal newline. Restore it so the
      // source viewer's newline normalization retains trailing blank rows.
      source: code ? `${code.textContent ?? ""}\n` : "",
      highlightedLines: code?.innerHTML.split("\n") ?? [],
    }
  }, [html])

  return <SourceDocument source={formatted.source} highlightedLines={formatted.highlightedLines} label={label} />
}
