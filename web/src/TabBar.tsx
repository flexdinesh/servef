import { XIcon } from "lucide-react"
import type { KeyboardEvent } from "react"

import { FileIcon } from "./FileIcon.tsx"

interface TabBarProps {
  activePath: string
  tabs: readonly string[]
  onClose(path: string): void
  onSelect(path: string): void
}

function fileName(path: string): string {
  return path.split("/").at(-1) ?? path
}

export function TabBar({ activePath, onClose, onSelect, tabs }: TabBarProps) {
  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    if (event.key === "Delete" || event.key === "Backspace") {
      event.preventDefault()
      onClose(tabs[index] ?? "")
      return
    }
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return
    event.preventDefault()
    const offset = event.key === "ArrowLeft" ? -1 : 1
    const nextIndex = (index + offset + tabs.length) % tabs.length
    const next = tabs[nextIndex]
    if (next) onSelect(next)
  }

  return (
    <div className="buffer-tabs" role="tablist" aria-label="Open documents">
      {tabs.map((path, index) => {
        const active = path === activePath
        return (
          <div className={`buffer-tab${active ? " active" : ""}`} key={path}>
            <button
              className="buffer-tab-target"
              type="button"
              role="tab"
              aria-controls="document-pane"
              aria-selected={active}
              title={path}
              onClick={() => onSelect(path)}
              onKeyDown={(event) => handleKeyDown(event, index)}
            >
              <FileIcon path={path} />
              <span>{fileName(path)}</span>
            </button>
            <button
              className="buffer-tab-close"
              type="button"
              aria-label={`Close ${path}`}
              title={`Close ${path}`}
              onClick={() => onClose(path)}
            >
              <XIcon aria-hidden="true" />
            </button>
          </div>
        )
      })}
    </div>
  )
}
