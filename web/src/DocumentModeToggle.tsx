import type { DocumentMode, FileType } from "./page-contract.ts"
export type { DocumentMode } from "./page-contract.ts"

interface DocumentModeToggleProps {
  fileType: FileType
  mode: DocumentMode
  onChange(mode: DocumentMode): void
}

const modes: readonly DocumentMode[] = ["preview", "source"]

export function DocumentModeToggle({ fileType, mode, onChange }: DocumentModeToggleProps) {
  if (!fileType.previewKind) return null
  return (
    <div className="document-mode-toggle" role="group" aria-label="Document view">
      {modes.map((value) => (
        <button
          key={value}
          type="button"
          aria-pressed={mode === value}
          onClick={() => onChange(value)}
        >
          {value === "preview" ? fileType.previewLabel : "Source"}
        </button>
      ))}
    </div>
  )
}
