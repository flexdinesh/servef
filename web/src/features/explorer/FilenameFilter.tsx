import { FunnelIcon, XIcon } from "lucide-react"
import { useCallback, useEffect, useId, useRef, useState } from "react"

import { isDefaultFilenameScope, parseFilenameFilter, type FilenameMatcher } from "./filename-filter.ts"

import { IconButton } from "../../components/ui/icon-button.tsx"
import { TextField } from "../../components/ui/text-field.tsx"

const debounceMs = 150

interface SettledFilter {
  // Text most recently evaluated; the tree keeps the last valid filter while text is invalid.
  text: string
  applied: string
  matcher: FilenameMatcher | null
  invalidPattern: string | null
}

const emptyFilter: SettledFilter = { text: "", applied: "", matcher: null, invalidPattern: null }

export interface FilenameFilterState {
  text: string
  applied: string
  matcher: FilenameMatcher | null
  invalidPattern: string | null
  setText(text: string): void
  clear(): void
}

// Explorer-owned visibility filter. Owned above responsive remounts and never persisted.
export function useFilenameFilter(): FilenameFilterState {
  const [text, setTextValue] = useState("")
  const [settled, setSettled] = useState(emptyFilter)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  useEffect(() => () => clearTimeout(timer.current), [])
  const apply = useCallback((value: string) => {
    const result = parseFilenameFilter(value)
    setSettled((previous) => result.valid
      ? { text: value, applied: value.trim(), matcher: result.matcher, invalidPattern: null }
      : { ...previous, text: value, invalidPattern: result.pattern })
  }, [])
  const setText = useCallback((value: string) => {
    setTextValue(value)
    clearTimeout(timer.current)
    timer.current = setTimeout(() => apply(value), debounceMs)
  }, [apply])
  const clear = useCallback(() => {
    clearTimeout(timer.current)
    setTextValue("")
    apply("")
  }, [apply])
  return {
    text,
    applied: settled.applied,
    matcher: settled.matcher,
    invalidPattern: settled.text === text ? settled.invalidPattern : null,
    setText,
    clear,
  }
}

interface FilenameFilterProps {
  filter: FilenameFilterState
  scopePatterns: readonly string[]
  totalCount: number
  visibleCount: number
  onFocusTree(): void
}

export function FilenameFilter({ filter, scopePatterns, totalCount, visibleCount, onFocusTree }: FilenameFilterProps) {
  const errorId = useId()
  const scopeId = useId()
  const input = useRef<HTMLInputElement>(null)
  const scoped = !isDefaultFilenameScope(scopePatterns)
  const describedBy = [filter.invalidPattern !== null ? errorId : "", scoped ? scopeId : ""].filter(Boolean).join(" ")
  const active = filter.matcher !== null

  return <div className="filename-filter">
    <TextField className="filename-filter-field" inputClassName="filename-filter-input" inputRef={input}
      icon={<FunnelIcon />} invalid={filter.invalidPattern !== null} type="text" aria-label="Filter files by name"
      placeholder="Filter: *.md, *.go" value={filter.text} spellCheck={false} autoComplete="off" autoCapitalize="off"
      aria-describedby={describedBy || undefined}
      onChange={(event) => filter.setText(event.target.value)}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          if (filter.text) filter.clear()
          else input.current?.blur()
        } else if (event.key === "Enter" || event.key === "ArrowDown") {
          onFocusTree()
        } else return
        event.preventDefault()
        event.stopPropagation()
      }}
      trailing={<>
        <span className="filename-filter-count" role="status">
          {active && <>
            <span aria-hidden="true">{visibleCount} / {totalCount}</span>
            <span className="visually-hidden">{visibleCount} of {totalCount} files match</span>
          </>}
        </span>
        {filter.text && <IconButton className="filename-filter-clear" size="xs" label="Clear filter" icon={<XIcon aria-hidden="true" />}
          onClick={() => {
            filter.clear()
            input.current?.focus()
          }} />}
      </>} />
    {filter.invalidPattern !== null && <p id={errorId} className="filename-filter-error">Invalid pattern "{filter.invalidPattern}"</p>}
    {scoped && <p id={scopeId} className="filename-filter-scope" title={scopePatterns.join(", ")}>Serving {scopePatterns.join(", ")}</p>}
  </div>
}
