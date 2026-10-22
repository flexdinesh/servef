import { useCallback, useMemo, useState } from "react"

import type { DocumentMode } from "../../page-contract.ts"

// The Preview choice lasts for the browser tab's session, per project. Projects
// are absolute paths. Storage is a convenience; blocked storage keeps the
// in-memory choice.
const storagePrefix = "servef-preview:"

function readPreference(project: string): boolean {
  try {
    return window.sessionStorage.getItem(storagePrefix + project) === "1"
  } catch {
    return false
  }
}

function writePreference(project: string, preview: boolean) {
  try {
    if (preview) window.sessionStorage.setItem(storagePrefix + project, "1")
    else window.sessionStorage.removeItem(storagePrefix + project)
  } catch {
    // Keep the in-memory choice.
  }
}

// usePreviewPreference opens documents without a chosen view in Preview once
// the user chooses Preview. Choosing Source clears it; Format leaves it alone.
export function usePreviewPreference(project: string) {
  const stored = useMemo(() => readPreference(project), [project])
  const [chosen, setChosen] = useState<boolean | null>(null)
  const preview = chosen ?? stored
  const choose = useCallback((mode: DocumentMode) => {
    if (mode === "format") return
    setChosen(mode === "preview")
    writePreference(project, mode === "preview")
  }, [project])
  const preferredMode: DocumentMode | undefined = preview ? "preview" : undefined
  return { preferredMode, choose }
}
