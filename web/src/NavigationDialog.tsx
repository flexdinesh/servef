import { useEffect, useRef, useState, type KeyboardEvent } from "react"

import { groupLocations, navigationTitle } from "./code-navigation.ts"
import type { NavigationLocation } from "./navigation-contract.ts"
import { highlightedParts } from "./search-highlights.ts"
import type { NavigationPanel } from "./use-code-navigation.ts"

interface NavigationDialogProps {
  panel: NavigationPanel
  onClose(): void
  onOpen(location: NavigationLocation): void
}

function Snippet({ text, symbol }: { text: string; symbol: string }) {
  const start = symbol ? text.indexOf(symbol) : -1
  const ranges = start >= 0 ? [{ start, end: start + symbol.length }] : []
  return highlightedParts(text, ranges).map((part, index) => (
    part.highlighted ? <mark key={index}>{part.text}</mark> : part.text
  ))
}

// NavigationDialog lists definitions to choose from or references, grouped
// by file like VS Code's peek view: arrows move, Enter opens, Escape closes.
export function NavigationDialog({ panel, onClose, onOpen }: NavigationDialogProps) {
  const dialog = useRef<HTMLDialogElement>(null)
  const list = useRef<HTMLDivElement>(null)
  const returnFocus = useRef<HTMLElement | null>(null)
  const [activeIndex, setActiveIndex] = useState(0)
  const open = panel.status !== "closed"
  const locations = panel.status === "results" ? panel.result.locations : []
  const symbol = panel.status === "results" ? panel.result.symbol : panel.status === "loading" ? panel.symbol : ""
  const active = Math.min(activeIndex, Math.max(locations.length - 1, 0))

  useEffect(() => {
    const element = dialog.current
    if (!element) return
    if (open && !element.open) {
      returnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
      setActiveIndex(0)
      element.showModal()
    } else if (!open && element.open) {
      element.close()
      returnFocus.current?.focus({ preventScroll: true })
    }
    if (open) list.current?.focus({ preventScroll: true })
  }, [open, panel.status])

  useEffect(() => {
    list.current?.querySelector(`[data-location="${active}"]`)?.scrollIntoView({ block: "nearest" })
  }, [active])

  function choose(location: NavigationLocation | undefined) {
    if (!location) return
    // Opening moves focus to the document; do not restore it here.
    returnFocus.current = null
    onOpen(location)
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      if (!locations.length) return
      const step = event.key === "ArrowDown" ? 1 : -1
      setActiveIndex((active + step + locations.length) % locations.length)
    } else if (event.key === "Enter") {
      event.preventDefault()
      choose(locations[active])
    }
  }

  const kind = panel.status === "closed" ? "definition" : panel.kind
  const heading = panel.status === "results"
    ? navigationTitle(kind, symbol, locations.length)
    : panel.status === "loading"
      ? `Finding references${symbol ? ` to ${symbol}` : ""}…`
      : kind === "definition" ? "Go to Definition" : "Find All References"
  let offset = 0
  const groups = groupLocations(locations).map((group) => {
    const start = offset
    offset += group.locations.length
    return { ...group, start }
  })

  return (
    <dialog
      ref={dialog}
      className="search-dialog navigation-dialog"
      aria-labelledby="navigation-title"
      onCancel={(event) => { event.preventDefault(); onClose() }}
      onClick={(event) => { if (event.target === dialog.current) onClose() }}
    >
      <div className="search-shell">
        <div className="search-field navigation-field">
          <h2 id="navigation-title" className="navigation-title">{heading}</h2>
          <kbd>Esc</kbd>
        </div>
        {panel.status === "message" && (
          <div className={`search-status search-status-${panel.tone}`} role="status">{panel.message}</div>
        )}
        {panel.status === "loading" && <div className="search-status" role="status" aria-live="polite">Searching the project…</div>}
        {panel.status === "results" && panel.result.indexing && (
          <div className="search-status search-status-warning" role="status">Still indexing; results may be incomplete.</div>
        )}
        <div
          ref={list}
          className="search-results navigation-results"
          role="listbox"
          tabIndex={-1}
          aria-label={heading}
          aria-activedescendant={locations.length ? `navigation-option-${active}` : undefined}
          onKeyDown={onKeyDown}
        >
          {groups.map((group) => (
            <div key={group.path} className="search-group" role="group" aria-label={group.path}>
              <div className="search-group-title navigation-group-title" aria-hidden="true">
                {group.path} ({group.locations.length})
              </div>
              <div className="search-list">
                {group.locations.map((location, index) => {
                  const current = group.start + index
                  return (
                    <div
                      key={`${location.path}:${location.line}:${location.column}`}
                      id={`navigation-option-${current}`}
                      data-location={current}
                      className={`search-result navigation-result${current === active ? " active" : ""}`}
                      role="option"
                      aria-selected={current === active}
                      onClick={() => choose(location)}
                      onMouseMove={current === active ? undefined : () => setActiveIndex(current)}
                    >
                      <span className="navigation-result-line">{location.line}:{location.column + 1}</span>
                      <span className="search-result-snippet navigation-result-snippet">
                        <Snippet text={location.snippet} symbol={symbol} />
                      </span>
                      {location.kind !== "reference" && <span className="navigation-result-kind">{location.kind}</span>}
                    </div>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      </div>
    </dialog>
  )
}
