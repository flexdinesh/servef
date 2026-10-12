import type { FormatState } from "./document-format.ts"
import { formatBytes } from "./format-bytes.ts"
import type { DocumentMode, PageData } from "./page-contract.ts"

export type DocumentView =
  | { kind: "none" }
  | { kind: "binary" }
  | { kind: "tooLarge" }
  | { kind: "image" }
  | { kind: "preview" }
  | { kind: "format"; lines: readonly string[] }
  | { kind: "source"; formatPending: boolean; formatError: string }

// documentModes lists the views a document declares, in toolbar order.
export function documentModes(page: PageData): DocumentMode[] {
  const type = page.hasFile ? page.documentType : null
  if (!type || page.tooLarge) return []
  const modes: DocumentMode[] = []
  // Only images preview binary bytes; other binary files show the Binary file note.
  if (type.previewKind && (!page.binary || type.previewKind === "image")) modes.push("preview")
  if (!page.binary) {
    modes.push("source")
    if (type.formattable) modes.push("format")
  }
  return modes
}

// modeDisabledReason explains why a declared view is unavailable, or "".
export function modeDisabledReason(page: PageData, mode: DocumentMode): string {
  if (mode === "source" || !page.previewDisabled) return ""
  const feature = mode === "format" ? "formatting is" : "preview is"
  return `Large file (${formatBytes(page.fileSize)}): ${feature} off`
}

export function resolveDocumentMode(page: PageData, requested: DocumentMode | undefined): DocumentMode | null {
  const enabled = documentModes(page).filter((mode) => !modeDisabledReason(page, mode))
  if (requested && enabled.includes(requested)) return requested
  const fallback = page.documentType?.defaultMode
  if (fallback && enabled.includes(fallback)) return fallback
  return enabled[0] ?? null
}

export function documentView(page: PageData, mode: DocumentMode | null, format: FormatState | undefined): DocumentView {
  if (!page.hasFile || !page.documentType) return { kind: "none" }
  if (page.tooLarge) return { kind: "tooLarge" }
  if (mode === null) return { kind: "binary" }
  if (mode === "preview") return page.documentType.previewKind === "image" ? { kind: "image" } : { kind: "preview" }
  if (mode === "format" && format?.kind === "ready") return { kind: "format", lines: format.lines }
  return {
    kind: "source",
    formatPending: mode === "format" && format?.kind !== "error",
    formatError: format?.kind === "error" ? format.message : "",
  }
}
