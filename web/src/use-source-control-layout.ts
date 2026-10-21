import { useCallback, useEffect, useState } from "react"

import type { CommitRange } from "./commits-contract.ts"
import { clampShare, defaultLayout, readLayout, writeLayout, type SourceControlLayout } from "./source-control-layout.ts"

// useSourceControlLayout owns Source Control's section layout and commit
// range, remembered in this browser.
export function useSourceControlLayout() {
  const [layout, setLayout] = useState<SourceControlLayout>(readLayout)
  useEffect(() => writeLayout(layout), [layout])

  const setRange = useCallback((range: CommitRange) => setLayout((current) => current.range === range ? current : { ...current, range }), [])
  const toggleChanges = useCallback(() => setLayout((current) => ({ ...current, changesCollapsed: !current.changesCollapsed })), [])
  const toggleCommits = useCallback(() => setLayout((current) => ({ ...current, commitsCollapsed: !current.commitsCollapsed })), [])
  const setShare = useCallback((share: number) => setLayout((current) => {
    const next = clampShare(share)
    return current.changesShare === next ? current : { ...current, changesShare: next }
  }), [])
  const resetShare = useCallback(() => setShare(defaultLayout.changesShare), [setShare])

  return { ...layout, setRange, toggleChanges, toggleCommits, setShare, resetShare }
}

export type SourceControlLayoutState = ReturnType<typeof useSourceControlLayout>
