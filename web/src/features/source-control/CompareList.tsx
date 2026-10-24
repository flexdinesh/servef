import { ChevronsDownIcon, CircleDotIcon, GitCompareArrowsIcon, RefreshCwIcon } from "lucide-react"
import { useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from "react"

import { Badge } from "../../components/ui/badge.tsx"
import { Button } from "../../components/ui/button.tsx"
import { ListButton, ListRow } from "../../components/ui/list-row.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import type { Commit } from "../../api/commits-contract.ts"
import { shortHash } from "../../lib/commit-hash.ts"
import { useClipboardCopy } from "../../lib/clipboard.ts"
import { BasePicker } from "./BasePicker.tsx"
import { commitAge, commitDate, selectedHashes, selectionCompare, type Compare } from "./compare.ts"
import type { Commits } from "./use-commits.ts"
import type { Refs } from "./use-refs.ts"

// Long-pressing a commit on a phone starts a range; the next tap ends it.
const longPress = 500

interface CompareListProps {
  commits: Commits
  // selection is the compare the Files pane shows.
  selection: Compare | null
  // changes counts uncommitted changes, null until read.
  changes: number | null
  // branch describes the branch row: its resolved base ("" when none) and
  // file count (null until read).
  branch: { base: string; files: number | null; truncated: boolean }
  // base is the chosen base, "" for the default.
  base: string
  mobile: boolean
  refs: Refs
  onBaseChange(base: string): void
  // onDrill shows the Files pane on phones.
  onDrill(): void
  onSelect(compare: Compare): void
  // onOpen opens the compare's review; focusDocument moves focus to it.
  onOpen(compare: Compare, focusDocument: boolean): void
  onShowAll(): void
}

type Row =
  | { key: "working"; compare: Compare }
  | { key: "branch"; compare: Compare }
  | { key: string; compare: Compare; commit: Commit }

function rowSelected(row: Row, selection: Compare | null, selected: ReadonlySet<string>): boolean {
  if (!("commit" in row)) return selection?.kind === row.compare.kind
  return selected.has(row.commit.hash)
}

// CompareList lists what Source Control can compare (decision 0035):
// uncommitted changes, the branch against its base, then the range's
// commits. Click selects one; Shift-click or Shift+Arrow extends a
// contiguous run of commits; Enter or double-click opens the review.
export function CompareList({
  base, branch, changes, commits, mobile, refs, selection, onBaseChange, onDrill, onOpen, onSelect, onShowAll,
}: CompareListProps) {
  const { error, loadMore, loadingMore, moreError, refresh, result } = commits
  const list = useMemo(() => result?.commits ?? [], [result])
  const rows = useMemo((): Row[] => [
    { key: "working", compare: { kind: "working" } },
    { key: "branch", compare: { kind: "branch", base } },
    ...list.map((commit): Row => ({ key: commit.hash, compare: { kind: "commit", hash: commit.hash }, commit })),
  ], [base, list])
  const selected = useMemo(() => new Set(selectedHashes(list, selection)), [list, selection])
  const container = useRef<HTMLDivElement>(null)
  const [focusedKey, setFocusedKey] = useState("")
  // anchor is the commit a Shift-selection extends from: the last commit
  // selected without Shift.
  const [anchor, setAnchor] = useState("")
  // pending is a phone's long-pressed commit, waiting for a second tap.
  const [pending, setPending] = useState("")
  const press = useRef<{ timer: number; hash: string; fired: boolean } | null>(null)
  const hashCopy = useClipboardCopy()
  const selectedKey = rows.find((row) => rowSelected(row, selection, selected))?.key ?? ""
  const tabKey = rows.some((row) => row.key === focusedKey) ? focusedKey : selectedKey || rows[0]?.key || ""
  // now is read once per list so ages don't shift between renders.
  const now = useMemo(() => result ? Date.now() : 0, [result])

  function focusRow(key: string) {
    setFocusedKey(key)
    requestAnimationFrame(() => {
      const row = container.current?.querySelector(`[data-row-key="${CSS.escape(key)}"]`)
      if (row instanceof HTMLElement) row.focus()
    })
  }

  function select(row: Row, extend: boolean) {
    if ("commit" in row) {
      const from = extend && anchor ? anchor : row.commit.hash
      const compare = selectionCompare(list, from, row.commit.hash)
      if (!extend) setAnchor(row.commit.hash)
      if (compare) onSelect(compare)
      return
    }
    setAnchor("")
    onSelect(row.compare)
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = rows.findIndex((row) => row.key === tabKey)
    const row = rows[index]
    if (!row) return
    if ((event.key === "c" || event.key === "C") && (event.metaKey || event.ctrlKey) && !event.shiftKey && !event.altKey && "commit" in row) {
      void hashCopy.copy(row.commit.hash)
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      const next = rows[index + (event.key === "ArrowDown" ? 1 : -1)]
      if (!next) return
      // Shift extends over commits only.
      const extend = event.shiftKey && !mobile && "commit" in next && "commit" in row
      if (extend && !anchor) setAnchor(row.commit.hash)
      focusRow(next.key)
      if (extend) {
        const compare = selectionCompare(list, anchor || row.commit.hash, next.commit.hash)
        if (compare) onSelect(compare)
      } else {
        select(next, false)
      }
    } else if (event.key === "Home" || event.key === "End") {
      const next = event.key === "Home" ? rows[0] : rows.at(-1)
      if (!next) return
      focusRow(next.key)
      select(next, false)
    } else if (event.key === "Escape" && selection?.kind === "range") {
      // Escape narrows a range back to the commit it started from.
      const start = rows.find((entry) => entry.key === (anchor || selection.to))
      if (start) select(start, false)
    } else if (event.key === " ") {
      select(row, event.shiftKey && !mobile)
    } else if (event.key === "Enter") {
      if (!rowSelected(row, selection, selected)) select(row, false)
      onOpen(rowSelected(row, selection, selected) && selection ? selection : row.compare, true)
    } else {
      return
    }
    event.preventDefault()
  }

  function onPointerDown(event: PointerEvent<HTMLDivElement>, row: Row) {
    if (!mobile || !("commit" in row) || event.pointerType === "mouse") return
    const hash = row.commit.hash
    const timer = window.setTimeout(() => {
      if (press.current?.hash !== hash) return
      press.current.fired = true
      setPending(hash)
      setAnchor(hash)
      onSelect({ kind: "commit", hash })
    }, longPress)
    press.current = { timer, hash, fired: false }
  }

  function endPress() {
    if (press.current) window.clearTimeout(press.current.timer)
  }

  function onClick(event: { shiftKey: boolean }, row: Row) {
    const pressed = press.current
    press.current = null
    if (pressed?.fired) return
    if (mobile && pending && "commit" in row) {
      // The second tap ends a long-press range and shows its files.
      const compare = selectionCompare(list, pending, row.commit.hash)
      setPending("")
      if (compare) {
        onSelect(compare)
        onDrill()
      }
      return
    }
    select(row, event.shiftKey && !mobile)
    // Phones drill into the Files pane on tap.
    if (mobile) onDrill()
  }

  const workingLabel = changes === null ? "" : changes === 0 ? "No changes" : changes.toLocaleString()
  const branchCount = branch.files === null ? "" : `${branch.files.toLocaleString()}${branch.truncated ? "+" : ""}`

  return (
    <div className="compare-list">
      {error && <div className="source-control-message">
        <StatusMessage tone="error" role="alert">{error}</StatusMessage>
        {!result && <Button variant="outline" onClick={refresh}>Retry</Button>}
      </div>}
      {/* Load more ends the list and scrolls with it, as in VS Code. */}
      <div className="compare-scroll">
        <div ref={container} className="compare-rows" role="listbox" aria-label="Compare" aria-multiselectable={!mobile || undefined}
          onKeyDown={onKeyDown}>
          {rows.map((row) => {
            const isSelected = rowSelected(row, selection, selected)
            const common = {
              "data-row-key": row.key,
              role: "option",
              tabIndex: row.key === tabKey ? 0 : -1,
              "aria-selected": isSelected,
              selected: isSelected,
              onFocus: () => setFocusedKey(row.key),
              onPointerDown: (event: PointerEvent<HTMLDivElement>) => onPointerDown(event, row),
              onPointerUp: endPress,
              onPointerCancel: endPress,
              onPointerLeave: endPress,
              onClick: (event: { shiftKey: boolean }) => onClick(event, row),
              onDoubleClick: () => onOpen(selection && isSelected ? selection : row.compare, true),
            }
            if (row.key === "working") {
              return (
                <ListRow key={row.key} {...common} className="compare-row compare-row-pinned"
                  aria-label={`Uncommitted changes${changes === null ? "" : `, ${changes.toLocaleString()} ${changes === 1 ? "file" : "files"}`}`}
                  title="Changes in the working tree since the last commit">
                  <CircleDotIcon className="compare-row-icon" aria-hidden="true" />
                  <span className="compare-row-label">Uncommitted</span>
                  {workingLabel && <Badge aria-hidden="true">{workingLabel}</Badge>}
                </ListRow>
              )
            }
            if (row.key === "branch") {
              return (
                <ListRow key={row.key} {...common} className="compare-row compare-row-pinned compare-row-branch"
                  aria-label={branch.base ? `Branch vs ${branch.base}${branchCount ? `, ${branchCount} files` : ""}` : "Branch, no base branch"}
                  title={branch.base ? `Net changes on this branch since it left ${branch.base}` : "No base branch found; choose one"}>
                  <GitCompareArrowsIcon className="compare-row-icon" aria-hidden="true" />
                  <span className="compare-row-label">{branch.base ? `vs ${branch.base}` : "No base branch"}</span>
                  {/* The picker keeps its own events; the space it fills still selects the row. */}
                  <span className="compare-row-picker" onClick={(event) => { if (event.target !== event.currentTarget) event.stopPropagation() }}
                    onDoubleClick={(event) => { if (event.target !== event.currentTarget) event.stopPropagation() }}
                    onKeyDown={(event) => event.stopPropagation()}>
                    <BasePicker base={base} refs={refs} onChange={onBaseChange} />
                  </span>
                  {branchCount && <Badge aria-hidden="true">{branchCount}</Badge>}
                </ListRow>
              )
            }
            if (!("commit" in row)) return null
            const { commit } = row
            const age = commitAge(commit.authorTime, now)
            return (
              <ListRow key={row.key} {...common} className="compare-row compare-row-commit"
                data-range={selection?.kind === "range" && isSelected ? "" : undefined}
                data-pending={pending === commit.hash || undefined}
                aria-label={`${commit.subject}, ${commit.author.name}, ${age.long}`}
                title={`${commit.subject}\n${commit.author.name} · ${commitDate(commit.authorTime)} · ${shortHash(commit.hash)}`}>
                <span className="compare-row-label">{commit.subject || "(no message)"}</span>
                <span className="compare-row-age">{age.short}</span>
              </ListRow>
            )
          })}
        </div>
        {(result?.more || moreError) && (
          <div className="commit-more">
            {moreError && <StatusMessage className="commit-more-error" tone="error" role="alert">{moreError}</StatusMessage>}
            <ListButton density="list" className="compare-row commit-more-row" aria-disabled={loadingMore || undefined}
              onClick={() => { if (!loadingMore) loadMore() }}>
              {moreError ? <RefreshCwIcon className="compare-row-icon" aria-hidden="true" />
                : <ChevronsDownIcon className="compare-row-icon" aria-hidden="true" />}
              <span className="compare-row-label">{loadingMore ? "Loading…" : moreError ? "Retry" : "Load more"}</span>
            </ListButton>
          </div>
        )}
      </div>
      {result && list.length === 0 && (
        <div className="source-control-message">
          <StatusMessage>{result.range === "all" ? "No commits yet."
            : result.base ? `No commits ahead of ${result.base}.` : "No base branch found to compare with."}</StatusMessage>
          {result.range === "branch" && <Button variant="outline" onClick={onShowAll}>Show all history</Button>}
        </div>
      )}
      {mobile && pending && <StatusMessage className="compare-list-hint" role="status">Tap another commit to compare a range.</StatusMessage>}
      <div className="visually-hidden" aria-live="polite">
        {hashCopy.status === "copied" ? "Commit hash copied." : hashCopy.status === "failed" ? "Couldn't copy the commit hash." : ""}
      </div>
    </div>
  )
}
