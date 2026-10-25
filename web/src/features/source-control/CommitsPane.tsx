import { ChevronsDownIcon, FileDiffIcon, RefreshCwIcon } from "lucide-react"
import { useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from "react"

import { Button } from "../../components/ui/button.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { ListButton, ListRow } from "../../components/ui/list-row.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import type { Commit } from "../../api/commits-contract.ts"
import { shortHash } from "../../lib/commit-hash.ts"
import { useClipboardCopy } from "../../lib/clipboard.ts"
import { commitAge, commitDate, selectedHashes, selectionCompare, type Compare } from "./compare.ts"
import { rowDiff, type RefChip } from "./source-control-panes.ts"
import type { Commits } from "./use-commits.ts"

// Long-pressing a commit on a phone starts a range; the next tap ends it.
const longPress = 500

interface CommitsPaneProps {
  commits: Commits
  // selection is the selected commit or range, null for none.
  selection: Compare | null
  // chips name the stack's branches at their commits.
  chips: ReadonlyMap<string, readonly RefChip[]>
  // allOnly is set when HEAD is the base branch: the list is all history.
  allOnly: boolean
  mobile: boolean
  onSelect(compare: Compare): void
  // onOpen opens a compare's review; focusDocument moves focus to it.
  onOpen(compare: Compare, focusDocument: boolean): void
  onShowAll(): void
}

// CommitsPane lists the range's commits (decision 0040). Click selects one;
// Shift-click or Shift+Arrow extends a contiguous run; Escape narrows it.
// Diffs open deliberately: a row's Diff action (pinned to the selection's
// newest commit, else shown on hover and focus), Enter, or double-click.
export function CommitsPane({ allOnly, chips, commits, mobile, selection, onOpen, onSelect, onShowAll }: CommitsPaneProps) {
  const { error, loadMore, loadingMore, moreError, refresh, result } = commits
  const list = useMemo(() => result?.commits ?? [], [result])
  const selected = useMemo(() => selectedHashes(list, selection), [list, selection])
  const container = useRef<HTMLDivElement>(null)
  const [focusedKey, setFocusedKey] = useState("")
  // anchor is the commit a Shift-selection extends from: the last commit
  // selected without Shift.
  const [anchor, setAnchor] = useState("")
  // pending is a phone's long-pressed commit, waiting for a second tap.
  const [pending, setPending] = useState("")
  const press = useRef<{ timer: number; hash: string; fired: boolean } | null>(null)
  const hashCopy = useClipboardCopy()
  const tabKey = list.some((commit) => commit.hash === focusedKey) ? focusedKey : selected[0] ?? list[0]?.hash ?? ""
  // now is read once per list so ages don't shift between renders.
  const now = useMemo(() => result ? Date.now() : 0, [result])

  function focusRow(hash: string) {
    setFocusedKey(hash)
    requestAnimationFrame(() => {
      const row = container.current?.querySelector(`[data-row-key="${CSS.escape(hash)}"]`)
      if (row instanceof HTMLElement) row.focus()
    })
  }

  function select(hash: string, extend: boolean) {
    const compare = selectionCompare(list, extend && anchor ? anchor : hash, hash)
    if (!extend) setAnchor(hash)
    if (compare) onSelect(compare)
  }

  // open reviews what a row's Diff action opens.
  function open(hash: string, focusDocument: boolean) {
    const diff = rowDiff(hash, selection, selected)
    onOpen(diff ? diff.compare : { kind: "commit", hash }, focusDocument)
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = list.findIndex((commit) => commit.hash === tabKey)
    const row = list[index]
    if (!row) return
    if ((event.key === "c" || event.key === "C") && (event.metaKey || event.ctrlKey) && !event.shiftKey && !event.altKey) {
      void hashCopy.copy(row.hash)
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      const next = list[index + (event.key === "ArrowDown" ? 1 : -1)]
      if (!next) return
      const extend = event.shiftKey && !mobile
      if (extend && !anchor) setAnchor(row.hash)
      focusRow(next.hash)
      if (extend) {
        const compare = selectionCompare(list, anchor || row.hash, next.hash)
        if (compare) onSelect(compare)
      } else {
        select(next.hash, false)
      }
    } else if (event.key === "Home" || event.key === "End") {
      const next = event.key === "Home" ? list[0] : list.at(-1)
      if (!next) return
      focusRow(next.hash)
      select(next.hash, false)
    } else if (event.key === "Escape" && selection?.kind === "range") {
      // Escape narrows a range back to the commit it started from.
      select(anchor || selection.to, false)
    } else if (event.key === " ") {
      select(row.hash, event.shiftKey && !mobile)
    } else if (event.key === "Enter") {
      if (!selected.includes(row.hash)) {
        select(row.hash, false)
        onOpen({ kind: "commit", hash: row.hash }, true)
      } else {
        open(row.hash, true)
      }
    } else {
      return
    }
    event.preventDefault()
  }

  function onPointerDown(event: PointerEvent<HTMLDivElement>, hash: string) {
    if (!mobile || event.pointerType === "mouse") return
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

  function onClick(event: { shiftKey: boolean }, hash: string) {
    const pressed = press.current
    press.current = null
    if (pressed?.fired) return
    if (mobile && pending) {
      // The second tap ends a long-press range.
      const compare = selectionCompare(list, pending, hash)
      setPending("")
      if (compare) onSelect(compare)
      return
    }
    select(hash, event.shiftKey && !mobile)
  }

  return (
    <div className="compare-list">
      {error && <div className="source-control-message">
        <StatusMessage tone="error" role="alert">{error}</StatusMessage>
        {!result && <Button variant="outline" onClick={refresh}>Retry</Button>}
      </div>}
      {/* Load more ends the list and scrolls with it, as in VS Code. */}
      <div className="compare-scroll">
        <div ref={container} className="compare-rows" role="listbox" aria-label="Commits" aria-multiselectable={!mobile || undefined}
          onKeyDown={onKeyDown}>
          {list.map((commit) => <CommitRow key={commit.hash} commit={commit} now={now}
            selected={selected.includes(commit.hash)} inRange={selection?.kind === "range" && selected.includes(commit.hash)}
            pending={pending === commit.hash} focusable={commit.hash === tabKey} chips={chips.get(commit.hash) ?? []}
            diff={rowDiff(commit.hash, selection, selected)}
            onFocus={() => setFocusedKey(commit.hash)}
            onPointerDown={(event) => onPointerDown(event, commit.hash)} onPointerEnd={endPress}
            onClick={(event) => onClick(event, commit.hash)}
            onDoubleClick={() => open(commit.hash, true)}
            onOpen={(compare) => onOpen(compare, true)} />)}
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
          {result.range === "branch" && !allOnly && <Button variant="outline" onClick={onShowAll}>Show all history</Button>}
        </div>
      )}
      {mobile && pending && <StatusMessage className="compare-list-hint" role="status">Tap another commit to compare a range.</StatusMessage>}
      <div className="visually-hidden" aria-live="polite">
        {hashCopy.status === "copied" ? "Commit hash copied." : hashCopy.status === "failed" ? "Couldn't copy the commit hash." : ""}
      </div>
    </div>
  )
}

interface CommitRowProps {
  commit: Commit
  now: number
  selected: boolean
  inRange: boolean
  pending: boolean
  focusable: boolean
  chips: readonly RefChip[]
  diff: ReturnType<typeof rowDiff>
  onFocus(): void
  onPointerDown(event: PointerEvent<HTMLDivElement>): void
  onPointerEnd(): void
  onClick(event: { shiftKey: boolean }): void
  onDoubleClick(): void
  onOpen(compare: Compare): void
}

function CommitRow({
  chips, commit, diff, focusable, inRange, now, pending, selected, onClick, onDoubleClick, onFocus, onOpen, onPointerDown, onPointerEnd,
}: CommitRowProps) {
  const age = commitAge(commit.authorTime, now)
  const diffLabel = diff?.compare.kind === "range" ? "Open changes in the selected commits" : "Open changes"
  return (
    <ListRow data-row-key={commit.hash} role="option" tabIndex={focusable ? 0 : -1} aria-selected={selected} selected={selected}
      className="compare-row compare-row-commit" data-range={inRange || undefined} data-pending={pending || undefined}
      data-diff-pinned={diff?.pinned || undefined}
      aria-label={`${commit.subject}, ${commit.author.name}, ${age.long}${chips.length ? `, ${chips.map((chip) => chip.name).join(", ")}` : ""}`}
      title={`${commit.subject}\n${commit.author.name} · ${commitDate(commit.authorTime)} · ${shortHash(commit.hash)}`}
      onFocus={onFocus} onPointerDown={onPointerDown} onPointerUp={onPointerEnd} onPointerCancel={onPointerEnd} onPointerLeave={onPointerEnd}
      onClick={onClick} onDoubleClick={onDoubleClick}>
      <span className="compare-row-label">{commit.subject || "(no message)"}</span>
      {chips.map((chip) => <span key={chip.name} className="ref-chip" data-current={chip.current || undefined}>{chip.name}</span>)}
      <span className="compare-row-age">{age.short}</span>
      {diff && (
        // The row keeps its own keyboard; Enter opens the same diff.
        <IconButton size="xs" className="row-action" tabIndex={-1} label={diffLabel} icon={<FileDiffIcon aria-hidden="true" />}
          onClick={(event) => {
            event.stopPropagation()
            onOpen(diff.compare)
          }}
          onPointerDown={(event) => event.stopPropagation()}
          onDoubleClick={(event) => event.stopPropagation()} />
      )}
    </ListRow>
  )
}
