import { RotateCcwIcon, SearchIcon, XIcon } from "lucide-react"
import { useCallback, useEffect, useId, useRef, useState } from "react"

import { Button } from "./components/ui/button.tsx"
import { matchSettings, type SettingId } from "./settings.ts"
import { useSettingValues, type SettingValue } from "./use-setting-values.ts"

export const settingsShortcutLabel = navigator.platform.includes("Mac") ? "⌘," : "Ctrl+,"

// How long a setting opened from the command palette stays highlighted.
export const REVEAL_MS = 1600

interface SettingItemProps {
  setting: SettingValue
  revealed: boolean
}

function SettingItem({ setting, revealed }: SettingItemProps) {
  const { definition, modified } = setting
  const id = useId()
  const control = useRef<HTMLElement | null>(null)
  const setControl = (element: HTMLElement | null) => { control.current = element }
  const [draft, setDraft] = useState<string | null>(null)
  const [invalid, setInvalid] = useState(false)

  function change(value: string) {
    setDraft(value)
    setInvalid(!setting.set(value))
  }

  const describedBy = `${id}-description${invalid ? ` ${id}-error` : ""}`
  return (
    <div className="setting-item" data-setting={definition.id} data-modified={modified || undefined}
      data-revealed={revealed || undefined}>
      <div className="setting-heading">
        <label className="setting-title" htmlFor={id}>
          <span className="setting-category">{definition.category}: </span>
          <span className="setting-name">{definition.title}</span>
        </label>
        {modified && (
          <>
            <span className="setting-modified">Modified</span>
            <button
              type="button"
              className="setting-reset"
              title={`Reset to default (${setting.defaultLabel})`}
              aria-label={`Reset ${definition.title}`}
              onClick={() => {
                setDraft(null)
                setInvalid(false)
                setting.reset()
                // The reset control disappears; keep focus on the setting.
                control.current?.focus()
              }}
            >
              <RotateCcwIcon aria-hidden="true" />
            </button>
          </>
        )}
      </div>
      <p className="setting-description" id={`${id}-description`}>{definition.description}</p>
      <div className="setting-control">
        {definition.kind === "number" ? (
          <>
            <input
              ref={setControl}
              id={id}
              type="number"
              inputMode="numeric"
              min={definition.min}
              max={definition.max}
              step={1}
              value={draft ?? setting.value}
              aria-invalid={invalid || undefined}
              aria-describedby={describedBy}
              onChange={(event) => change(event.target.value)}
              onBlur={() => {
                setDraft(null)
                setInvalid(false)
              }}
            />
            <span className="setting-unit">{definition.unit}</span>
          </>
        ) : (
          <select ref={setControl} id={id} value={setting.value} aria-describedby={describedBy}
            onChange={(event) => { setting.set(event.target.value) }}>
            {definition.options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
          </select>
        )}
      </div>
      {invalid && definition.kind === "number" && (
        <p className="setting-error" id={`${id}-error`}>
          Enter a whole number from {definition.min} to {definition.max}.
        </p>
      )}
    </div>
  )
}

// open shows Settings; with a setting, it scrolls to, focuses, and briefly
// highlights that setting.
export type OpenSettings = (setting?: SettingId) => void

interface SettingsDialogProps {
  registerOpen(open: OpenSettings | null): void
}

export function SettingsDialog({ registerOpen }: SettingsDialogProps) {
  const { values, modifiedCount, resetAll } = useSettingValues()
  const dialog = useRef<HTMLDialogElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLDivElement>(null)
  const [query, setQuery] = useState("")
  const [resetKey, setResetKey] = useState(0)
  // token re-runs the reveal when the same setting is opened again.
  const [revealed, setRevealed] = useState<{ setting: SettingId; token: number } | null>(null)
  const results = matchSettings(query)
  const categories = [...new Set(results.map((definition) => definition.category))]

  const show = useCallback((setting?: SettingId) => {
    if (!dialog.current) return
    setQuery("")
    if (!dialog.current.open) dialog.current.showModal()
    if (setting) setRevealed((current) => ({ setting, token: (current?.token ?? 0) + 1 }))
    else search.current?.focus()
  }, [])

  useEffect(() => {
    if (!revealed) return
    const item = list.current?.querySelector<HTMLElement>(`[data-setting="${revealed.setting}"]`)
    item?.scrollIntoView({ block: "nearest" })
    const control = item?.querySelector<HTMLElement>("input, select")
    control?.focus()
    if (control instanceof HTMLInputElement) control.select()
    const timer = window.setTimeout(() => setRevealed(null), REVEAL_MS)
    return () => window.clearTimeout(timer)
  }, [revealed])

  useEffect(() => {
    registerOpen(show)
    const shortcut = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.altKey || event.shiftKey || !(event.metaKey || event.ctrlKey)) return
      if (event.key !== ",") return
      event.preventDefault()
      show()
    }
    document.addEventListener("keydown", shortcut)
    return () => {
      registerOpen(null)
      document.removeEventListener("keydown", shortcut)
    }
  }, [registerOpen, show])

  return (
    <dialog
      ref={dialog}
      className="settings-dialog"
      aria-labelledby="settings-title"
      onClick={(event) => { if (event.target === dialog.current) dialog.current.close() }}
      onClose={() => setRevealed(null)}
    >
      <div className="settings-shell">
        <header className="settings-header">
          <h2 id="settings-title">Settings</h2>
          <button type="button" className="settings-close" aria-label="Close settings" onClick={() => dialog.current?.close()}>
            <XIcon aria-hidden="true" />
          </button>
        </header>
        <div className="settings-search">
          <SearchIcon aria-hidden="true" />
          <input
            ref={search}
            type="search"
            placeholder="Search settings"
            aria-label="Search settings"
            aria-controls="settings-list"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              // Escape clears a query before it closes the dialog.
              if (event.key === "Escape" && query) {
                event.preventDefault()
                setQuery("")
              }
            }}
          />
        </div>
        <div ref={list} className="settings-list" id="settings-list">
          {results.length ? categories.map((category) => (
            <section key={category} aria-label={category}>
              <h3 className="settings-group-title">{category}</h3>
              {results.filter((definition) => definition.category === category).map((definition) => (
                <SettingItem key={`${definition.id}-${resetKey}`} setting={values[definition.id]}
                  revealed={revealed?.setting === definition.id} />
              ))}
            </section>
          )) : (
            <p className="settings-empty">No settings match “{query.trim()}”.</p>
          )}
        </div>
        <footer className="settings-footer">
          <span>Saved in this browser. Changes apply immediately.</span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={modifiedCount === 0}
            onClick={() => {
              resetAll()
              setResetKey((key) => key + 1)
              // Reset all disables itself; return focus to the search field.
              search.current?.focus()
            }}
          >
            <RotateCcwIcon aria-hidden="true" />
            Reset all
          </Button>
        </footer>
      </div>
    </dialog>
  )
}
