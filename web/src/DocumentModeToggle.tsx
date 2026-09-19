export type DocumentMode = "preview" | "source"

interface DocumentModeToggleProps {
  mode: DocumentMode
  onChange(mode: DocumentMode): void
}

const modes: readonly DocumentMode[] = ["preview", "source"]

export function DocumentModeToggle({ mode, onChange }: DocumentModeToggleProps) {
  return (
    <div className="document-mode-toggle" role="group" aria-label="Document view">
      {modes.map((value) => (
        <button
          key={value}
          type="button"
          aria-pressed={mode === value}
          onClick={() => onChange(value)}
        >
          {value === "preview" ? "Preview" : "Source"}
        </button>
      ))}
    </div>
  )
}
