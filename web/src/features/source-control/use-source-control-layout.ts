import { useCallback, useEffect, useState } from "react"

import type { CommitRange } from "../../api/commits-contract.ts"
import { clampShare, defaultLayout, readLayout, withBase, writeLayout, type SourceControlLayout } from "./source-control-layout.ts"

// useSourceControlLayout owns Source Control's split, commit range, and the
// project's chosen base branch, remembered in this browser.
export function useSourceControlLayout(project: string) {
  const [layout, setLayout] = useState<SourceControlLayout>(readLayout)
  useEffect(() => writeLayout(layout), [layout])

  const setRange = useCallback((range: CommitRange) => setLayout((current) => current.range === range ? current : { ...current, range }), [])
  const setShare = useCallback((share: number) => setLayout((current) => {
    const next = clampShare(share)
    return current.listShare === next ? current : { ...current, listShare: next }
  }), [])
  const resetShare = useCallback(() => setShare(defaultLayout.listShare), [setShare])
  const base = Object.hasOwn(layout.bases, project) ? layout.bases[project] ?? "" : ""
  // setBase chooses project's base; "" returns to the default.
  const setBase = useCallback((next: string) => setLayout((current) => withBase(current, project, next)), [project])

  return { range: layout.range, listShare: layout.listShare, base, setRange, setShare, resetShare, setBase }
}

export type SourceControlLayoutState = ReturnType<typeof useSourceControlLayout>
