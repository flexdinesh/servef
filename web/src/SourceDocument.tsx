import { useState } from "react"

interface SourceDocumentProps {
  source: string
  highlightedLines: readonly string[]
  label: string
}

function sourceLines(source: string): string[] {
  const lines = source.split(/\r\n|\r|\n/)
  return lines.length > 1 && lines.at(-1) === "" ? lines.slice(0, -1) : lines
}

export function SourceDocument({ source, highlightedLines, label }: SourceDocumentProps) {
  const lines = sourceLines(source)
  const [activeLine, setActiveLine] = useState<number | null>(null)
  const gutterWidth = `calc(${String(lines.length).length}ch + 30px)`

  return (
    <div
      className="source-document syntax-highlight"
      role="textbox"
      aria-label={label}
      aria-multiline="true"
      aria-readonly="true"
      tabIndex={0}
    >
      <code>
        {lines.map((line, index) => {
          const lineNumber = index + 1
          return (
            <span
              className={`source-document-line${activeLine === lineNumber ? " active" : ""}`}
              data-line={lineNumber}
              key={lineNumber}
              onClick={() => setActiveLine(lineNumber)}
            >
              <span
                className="source-document-line-number"
                style={{ width: gutterWidth }}
                aria-hidden="true"
              >
                {lineNumber}
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
