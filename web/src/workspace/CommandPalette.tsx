import { ChevronRightIcon } from "lucide-react"
import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react"

import { commands, matchCommands, type CommandAction } from "./commands.ts"
import type { HighlightRange } from "../features/search/search-engine.ts"
import { highlightedParts } from "../features/search/search-highlights.ts"
import { settingsShortcutLabel } from "../settings/SettingsDialog.tsx"
import { useSettingValues } from "../settings/use-setting-values.ts"

function Highlighted({ text, ranges }: { text: string; ranges: readonly HighlightRange[] }) {
  return highlightedParts(text, ranges).map((part, index) => (
    part.highlighted ? <mark key={index}>{part.text}</mark> : part.text
  ))
}

interface CommandPaletteProps {
  registerOpen(open: (() => void) | null): void
  onRun(action: CommandAction): void
}

// CommandPalette lists commands like VS Code's: Ctrl/Cmd+Shift+P or F1 opens
// it, typing filters, arrows move, Enter runs, Escape closes.
export function CommandPalette({ registerOpen, onRun }: CommandPaletteProps) {
  const { values } = useSettingValues()
  const dialog = useRef<HTMLDialogElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLDivElement>(null)
  const returnFocus = useRef<HTMLElement | null>(null)
  const [query, setQuery] = useState("")
  const [activeIndex, setActiveIndex] = useState(0)
  const matches = matchCommands(commands, query)
  const active = matches[Math.min(activeIndex, matches.length - 1)]

  const show = useCallback(() => {
    if (!dialog.current || dialog.current.open) return
    returnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    setQuery("")
    setActiveIndex(0)
    dialog.current.showModal()
    input.current?.focus()
  }, [])

  const close = useCallback((restoreFocus: boolean) => {
    if (dialog.current?.open) dialog.current.close()
    if (restoreFocus) returnFocus.current?.focus({ preventScroll: true })
  }, [])

  useEffect(() => {
    registerOpen(show)
    const shortcut = (event: globalThis.KeyboardEvent) => {
      if (event.defaultPrevented || event.altKey) return
      const palette = (event.metaKey || event.ctrlKey) && event.shiftKey && event.key.toLowerCase() === "p"
      const f1 = event.key === "F1" && !event.metaKey && !event.ctrlKey && !event.shiftKey
      if (!palette && !f1) return
      event.preventDefault()
      show()
    }
    document.addEventListener("keydown", shortcut)
    return () => {
      registerOpen(null)
      document.removeEventListener("keydown", shortcut)
    }
  }, [registerOpen, show])

  useEffect(() => {
    if (!active) return
    list.current?.querySelector(`[data-command="${CSS.escape(active.command.id)}"]`)?.scrollIntoView({ block: "nearest" })
  }, [active])

  function run(action: CommandAction | undefined) {
    if (!action) return
    close(false)
    onRun(action)
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      if (!matches.length) return
      const current = active ? matches.indexOf(active) : 0
      const step = event.key === "ArrowDown" ? 1 : -1
      setActiveIndex((current + step + matches.length) % matches.length)
    } else if (event.key === "Enter") {
      event.preventDefault()
      run(active?.command.action)
    } else if (event.key === "Escape") {
      event.preventDefault()
      close(true)
    }
  }

  const activeID = active ? `command-option-${active.command.id}` : undefined

  return (
    <dialog
      ref={dialog}
      className="search-dialog command-palette"
      aria-labelledby="command-palette-title"
      onCancel={(event) => { event.preventDefault(); close(true) }}
      onClick={(event) => { if (event.target === dialog.current) close(true) }}
    >
      <div className="search-shell">
        <h2 id="command-palette-title" className="visually-hidden">Command palette</h2>
        <div className="search-field command-field">
          <span className="command-prompt" aria-hidden="true">&gt;</span>
          <input
            ref={input}
            className="search-input"
            type="text"
            autoComplete="off"
            spellCheck={false}
            placeholder="Type a command"
            aria-label="Command"
            role="combobox"
            aria-controls="command-results"
            aria-expanded="true"
            aria-autocomplete="list"
            aria-activedescendant={activeID}
            value={query}
            onChange={(event) => {
              setQuery(event.target.value)
              setActiveIndex(0)
            }}
            onKeyDown={onKeyDown}
          />
          <kbd>Esc</kbd>
        </div>
        {matches.length === 0 && (
          <div className="search-status" role="status">No matching commands</div>
        )}
        <div ref={list} id="command-results" className="search-results command-results" role="listbox" aria-label="Commands">
          {matches.map((match, index) => {
            const { command } = match
            // Settings show their current value; other commands their shortcut.
            const detail = command.action.kind === "open-setting"
              ? values[command.action.setting].label
              : command.shortcut ?? settingsShortcutLabel
            return (
              <div
                key={command.id}
                id={`command-option-${command.id}`}
                data-command={command.id}
                className={`command-option${match === active ? " active" : ""}`}
                role="option"
                aria-selected={match === active}
                aria-label={`${command.category}: ${command.title}, ${detail}`}
                onClick={() => run(command.action)}
                onMouseMove={match === active ? undefined : () => setActiveIndex(index)}
              >
                <span className="command-category"><Highlighted text={command.category} ranges={match.category} /></span>
                <ChevronRightIcon className="command-separator" aria-hidden="true" />
                <span className="command-title"><Highlighted text={command.title} ranges={match.title} /></span>
                {command.action.kind === "open-setting"
                  ? <span className="command-value">{detail}</span>
                  : <kbd className="command-value">{detail}</kbd>}
              </div>
            )
          })}
        </div>
      </div>
    </dialog>
  )
}
