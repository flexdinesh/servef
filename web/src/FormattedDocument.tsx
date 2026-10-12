import { useMemo } from "react"

import { SourceDocument } from "./SourceDocument.tsx"

interface FormattedDocumentProps {
  lines: readonly string[]
  label: string
}

export function FormattedDocument({ lines, label }: FormattedDocumentProps) {
  const source = useMemo(() => {
    // Formatted rows are escaped backend HTML with complete syntax spans per
    // line. Parse them inertly to recover plain text for line numbering; the
    // trailing newline keeps final blank rows in the source viewer.
    const parsed = new DOMParser().parseFromString(`<pre>${lines.join("\n")}</pre>`, "text/html")
    return `${parsed.querySelector("pre")?.textContent ?? ""}\n`
  }, [lines])

  return <SourceDocument source={source} highlightedLines={lines} label={label} />
}
