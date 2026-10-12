import { documentModes, modeDisabledReason } from "./document-view.ts"
import type { DocumentMode, PageData } from "./page-contract.ts"
export type { DocumentMode } from "./page-contract.ts"

interface DocumentModeToggleProps {
  page: PageData
  mode: DocumentMode | null
  busyMode?: DocumentMode
  onChange(mode: DocumentMode): void
}

function modeLabel(page: PageData, mode: DocumentMode): string {
  if (mode === "preview") return page.documentType?.previewLabel || "Preview"
  return mode === "format" ? "Format" : "Source"
}

export function DocumentModeToggle({ page, mode, busyMode, onChange }: DocumentModeToggleProps) {
  const modes = documentModes(page)
  if (modes.length < 2) return null
  return (
    <div className="document-mode-toggle" role="group" aria-label="Document view">
      {modes.map((value) => {
        const disabledReason = modeDisabledReason(page, value)
        const busy = busyMode === value
        return (
          <button
            key={value}
            type="button"
            aria-pressed={mode === value}
            aria-busy={busy || undefined}
            aria-disabled={disabledReason ? true : undefined}
            title={disabledReason || undefined}
            onClick={() => { if (!disabledReason) onChange(value) }}
          >
            {modeLabel(page, value)}
          </button>
        )
      })}
    </div>
  )
}
