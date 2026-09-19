import { useState } from "react"

interface MarkdownSourceProps {
  source: string
}

function sourceLines(source: string): string[] {
  const lines = source.split(/\r\n|\r|\n/)
  return lines.length > 1 && lines.at(-1) === "" ? lines.slice(0, -1) : lines
}

export function MarkdownSource({ source }: MarkdownSourceProps) {
  const lines = sourceLines(source)
  const [activeLine, setActiveLine] = useState(1)
  const gutterWidth = `calc(${String(lines.length).length}ch + 30px)`

  return (
    <div
      className="markdown-source"
      role="textbox"
      aria-label="Markdown source"
      aria-multiline="true"
      aria-readonly="true"
      tabIndex={0}
    >
      <code>
        {lines.map((line, index) => {
          const lineNumber = index + 1
          return (
            <span
              className={`markdown-source-line${activeLine === lineNumber ? " active" : ""}`}
              data-line={lineNumber}
              key={lineNumber}
              onClick={() => setActiveLine(lineNumber)}
            >
              <span
                className="markdown-source-line-number"
                style={{ width: gutterWidth }}
                aria-hidden="true"
              >
                {lineNumber}
              </span>
              <span className="markdown-source-line-content">{line}</span>
            </span>
          )
        })}
      </code>
    </div>
  )
}
