import { useCallback, useEffect, useState } from "react"

import type { CommitRange } from "../../api/commits-contract.ts"
import {
  clampRailWidth, defaultLayout, readLayout, resizePair, withBase, writeLayout,
  type ChangesView, type PaneId, type SourceControlLayout,
} from "./source-control-layout.ts"

// useSourceControlLayout owns Source Control's panes, commit range, change
// list view, review Files rail, and the project's chosen base branch,
// remembered in this browser.
export function useSourceControlLayout(project: string) {
  const [layout, setLayout] = useState<SourceControlLayout>(readLayout)
  useEffect(() => writeLayout(layout), [layout])

  const setRange = useCallback((range: CommitRange) => setLayout((current) => current.range === range ? current : { ...current, range }), [])
  // setPaneCollapsed records the user's choice for a pane.
  const setPaneCollapsed = useCallback((pane: PaneId, collapsed: boolean) => setLayout((current) => (
    { ...current, collapsed: { ...current.collapsed, [pane]: collapsed } }
  )), [])
  const resizePanes = useCallback((open: readonly PaneId[], above: PaneId, below: PaneId, delta: number) => setLayout((current) => (
    { ...current, weights: resizePair(current.weights, open, above, below, delta) }
  )), [])
  const resetPanes = useCallback(() => setLayout((current) => ({ ...current, weights: defaultLayout.weights })), [])
  const setChangesView = useCallback((changesView: ChangesView) => setLayout((current) => ({ ...current, changesView })), [])
  const setRailWidth = useCallback((width: number) => setLayout((current) => {
    const railWidth = clampRailWidth(width)
    return current.railWidth === railWidth ? current : { ...current, railWidth }
  }), [])
  const setRailHidden = useCallback((railHidden: boolean) => setLayout((current) => ({ ...current, railHidden })), [])
  const base = Object.hasOwn(layout.bases, project) ? layout.bases[project] ?? "" : ""
  // setBase chooses project's base; "" returns to the default.
  const setBase = useCallback((next: string) => setLayout((current) => withBase(current, project, next)), [project])

  return {
    range: layout.range, weights: layout.weights, collapsed: layout.collapsed, changesView: layout.changesView,
    railWidth: layout.railWidth, railHidden: layout.railHidden, base,
    setRange, setPaneCollapsed, resizePanes, resetPanes, setChangesView, setRailWidth, setRailHidden, setBase,
  }
}

export type SourceControlLayoutState = ReturnType<typeof useSourceControlLayout>
