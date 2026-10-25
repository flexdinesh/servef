import { FileDiffIcon, GitBranchIcon, LayersIcon } from "lucide-react"
import { useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react"

import { Button } from "../../components/ui/button.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { ListRow } from "../../components/ui/list-row.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import { shortHash } from "../../lib/commit-hash.ts"
import type { Compare } from "./compare.ts"
import { stackRows, type StackRow } from "./source-control-panes.ts"
import type { BranchStack } from "./use-branch-stack.ts"

interface BranchesPaneProps {
  stack: BranchStack
  // base is the chosen base branch, "" for the default.
  base: string
  // onOpen opens a compare's review; focusDocument moves focus to it.
  onOpen(compare: Compare, focusDocument: boolean): void
}

function plural(count: number, noun: string): string {
  return `${count.toLocaleString()} ${noun}${count === 1 ? "" : "s"}`
}

// BranchesPane shows the branch stack (decision 0040): the layers above
// HEAD, HEAD's layer, the layers below, then the base at the merge-base.
// A layer diffs its own commits against the layer below, or everything since
// the base; Enter opens the base diff.
export function BranchesPane({ base, stack, onOpen }: BranchesPaneProps) {
  const { error, refresh, result } = stack
  const rows = useMemo(() => stackRows(result, base), [base, result])
  const container = useRef<HTMLDivElement>(null)
  const [focused, setFocused] = useState(-1)
  const current = rows.findIndex((row) => row.layer.current)
  const tabIndex = focused >= 0 && focused < rows.length ? focused : Math.max(0, current)
  const baseName = result?.base ?? ""

  function focusRow(index: number) {
    setFocused(index)
    const row = container.current?.querySelectorAll<HTMLElement>("[role=option]")[index]
    row?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      const next = tabIndex + (event.key === "ArrowDown" ? 1 : -1)
      if (next >= 0 && next < rows.length) focusRow(next)
    } else if (event.key === "Home" || event.key === "End") {
      focusRow(event.key === "Home" ? 0 : rows.length - 1)
    } else if (event.key === "Enter") {
      const row = rows[tabIndex]
      if (row) onOpen(row.baseDiff, true)
    } else {
      return
    }
    event.preventDefault()
  }

  return (
    <div className="branches-pane">
      {error && <div className="source-control-message">
        <StatusMessage tone="error" role="alert">{error}</StatusMessage>
        {!result && <Button variant="outline" onClick={refresh}>Retry</Button>}
      </div>}
      {result && rows.length === 0 && (
        <div className="source-control-message">
          <StatusMessage>{!result.base ? "No base branch found to compare with."
            : result.branch && result.branch === result.base ? `On ${result.base}.` : `Nothing ahead of ${result.base}.`}</StatusMessage>
        </div>
      )}
      {rows.length > 0 && (
        <div className="branches-scroll">
          <div ref={container} className="branch-rows" role="listbox" aria-label="Branches" onKeyDown={onKeyDown}>
            {rows.map((row, index) => (
              <BranchRow key={row.layer.name || row.layer.head} row={row} baseName={baseName} focusable={index === tabIndex}
                onFocus={() => setFocused(index)} onOpen={(compare) => onOpen(compare, true)} />
            ))}
          </div>
          <div className="branch-row branch-row-base" title={result?.mergeBase ? `${baseName} at the merge-base ${result.mergeBase}` : baseName}>
            <GitBranchIcon className="compare-row-icon" aria-hidden="true" />
            <span className="compare-row-label">{baseName}</span>
            {result?.mergeBase && <code className="ref-chip ref-chip-hash">{shortHash(result.mergeBase)}</code>}
            <span className="compare-row-age">merge-base</span>
          </div>
          {result?.truncated && <StatusMessage className="compare-list-hint">Some local branches weren't checked.</StatusMessage>}
        </div>
      )}
    </div>
  )
}

interface BranchRowProps {
  row: StackRow
  baseName: string
  focusable: boolean
  onFocus(): void
  onOpen(compare: Compare): void
}

function BranchRow({ baseName, focusable, row, onFocus, onOpen }: BranchRowProps) {
  const { layer, layerDiff, baseDiff } = row
  const name = layer.name || `HEAD ${shortHash(layer.head)}`
  const aliases = layer.aliases.length ? ` (also ${layer.aliases.join(", ")})` : ""
  const action = (label: string, icon: ReactNode, compare: Compare) => (
    <IconButton size="xs" className="row-action" tabIndex={-1} label={label} icon={icon}
      onClick={(event) => {
        event.stopPropagation()
        onOpen(compare)
      }} />
  )
  return (
    <ListRow role="option" tabIndex={focusable ? 0 : -1} aria-selected={false} className="compare-row branch-row"
      data-current={layer.current || undefined}
      aria-label={`${name}${layer.current ? ", checked out" : ""}, ${plural(layer.ahead, "commit")} on ${layer.parent}${aliases}`}
      title={`${name}${aliases}\n${plural(layer.ahead, "commit")} on ${layer.parent}`}
      onFocus={onFocus}
      onDoubleClick={() => onOpen(baseDiff)}>
      <GitBranchIcon className="compare-row-icon branch-row-icon" aria-hidden="true" />
      <span className="compare-row-label">{name}</span>
      {layer.current && layer.name && <span className="compare-row-age">HEAD</span>}
      <span className="branch-row-ahead" aria-hidden="true">+{layer.ahead.toLocaleString()}</span>
      <span className="row-actions">
        {layerDiff && action(`Open changes on ${name} since ${layer.parent}`, <LayersIcon aria-hidden="true" />, layerDiff)}
        {action(`Open changes on ${name} since ${baseName || "the base"}`, <FileDiffIcon aria-hidden="true" />, baseDiff)}
      </span>
    </ListRow>
  )
}
