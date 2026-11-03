import { Popover } from "@base-ui/react/popover"
import { CheckIcon, ChevronDownIcon, SearchIcon } from "lucide-react"
import { useId, useMemo, useRef, useState, type KeyboardEvent } from "react"

import { IconButton } from "../../components/ui/icon-button.tsx"
import { ListRow } from "../../components/ui/list-row.tsx"
import { menuPositionerClass } from "../../components/ui/menu.ts"
import { StatusMessage } from "../../components/ui/notice.tsx"
import { TextField } from "../../components/ui/text-field.tsx"
import type { Refs } from "./use-refs.ts"

interface BasePickerProps {
  // base is the chosen base, "" for the default.
  base: string
  refs: Refs
  onChange(base: string): void
}

interface Choice {
  // value is "" for the default base.
  value: string
  label: string
  detail: string
}

// BasePicker chooses the branch the branch compare starts from (decision
// 0035): local and remote-tracking branches, filtered as you type, or the
// default the server finds.
export function BasePicker({ base, refs, onChange }: BasePickerProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [cursor, setCursor] = useState(0)
  const input = useRef<HTMLInputElement>(null)
  const listId = useId()
  const result = refs.result
  const choices = useMemo((): Choice[] => {
    const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
    const matches = (name: string) => terms.every((term) => name.toLowerCase().includes(term))
    const fallback: Choice = { value: "", label: "Default", detail: result?.defaultBase || "none found" }
    const branches = (result?.branches ?? [])
      .filter((branch) => matches(branch.name))
      .map((branch): Choice => ({ value: branch.name, label: branch.name, detail: branch.remote ? "remote" : "" }))
    return terms.length === 0 ? [fallback, ...branches] : branches
  }, [query, result])
  const active = Math.min(cursor, Math.max(choices.length - 1, 0))

  function choose(choice: Choice | undefined) {
    if (!choice) return
    onChange(choice.value)
    setOpen(false)
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      const step = event.key === "ArrowDown" ? 1 : -1
      setCursor((active + step + choices.length) % Math.max(choices.length, 1))
    } else if (event.key === "Enter") {
      event.preventDefault()
      choose(choices[active])
    }
  }

  return (
    <Popover.Root open={open} onOpenChange={(next) => {
      setOpen(next)
      if (next) {
        setQuery("")
        setCursor(0)
        refs.load()
      }
    }}>
      <Popover.Trigger render={<IconButton size="xs" className="compare-base-trigger" label="Change base branch"
        icon={<ChevronDownIcon aria-hidden="true" />} />} />
      <Popover.Portal>
        <Popover.Positioner className={menuPositionerClass} sideOffset={4} align="start">
          <Popover.Popup className="ui-menu base-picker" initialFocus={input} aria-label="Base branch">
            <TextField inputRef={input} icon={<SearchIcon />} placeholder="Filter branches" value={query}
              aria-label="Filter branches" role="combobox" aria-expanded="true" aria-controls={listId}
              aria-activedescendant={choices[active] ? `${listId}-${active}` : undefined}
              onChange={(event) => {
                setQuery(event.target.value)
                setCursor(0)
              }} onKeyDown={onKeyDown} />
            <div id={listId} className="base-picker-list" role="listbox" aria-label="Branches">
              {choices.map((choice, index) => (
                <ListRow key={choice.value || "default"} id={`${listId}-${index}`} density="menu" role="option"
                  className="base-picker-option" active={index === active} aria-selected={choice.value === base}
                  onMouseEnter={() => setCursor(index)} onClick={() => choose(choice)}>
                  <span className="base-picker-check" aria-hidden="true">{choice.value === base && <CheckIcon />}</span>
                  <span className="base-picker-name">{choice.label}</span>
                  {choice.detail && <span className="base-picker-detail">{choice.detail}</span>}
                </ListRow>
              ))}
            </div>
            {refs.loading && !result && <StatusMessage className="base-picker-note">Loading branches…</StatusMessage>}
            {refs.error && <StatusMessage className="base-picker-note" tone="error" role="alert">{refs.error}</StatusMessage>}
            {result && choices.length === 0 && <StatusMessage className="base-picker-note">No branches match.</StatusMessage>}
            {result?.truncated && <StatusMessage className="base-picker-note">Showing the first {result.branches.length.toLocaleString()} branches.</StatusMessage>}
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  )
}
