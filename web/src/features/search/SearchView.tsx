import {
  CaseSensitiveIcon, EllipsisIcon, FoldVerticalIcon, ListXIcon, RefreshCwIcon, RegexIcon,
  UnfoldVerticalIcon, WholeWordIcon, XIcon,
} from "lucide-react"
import { useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react"

import { Badge } from "../../components/ui/badge.tsx"
import { Button } from "../../components/ui/button.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { ListRow } from "../../components/ui/list-row.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import { ProgressBar } from "../../components/ui/progress-bar.tsx"
import { Chevron } from "../../components/ui/section-toggle.tsx"
import { TextField } from "../../components/ui/text-field.tsx"
import { ToggleButton } from "../../components/ui/toggle-button.tsx"
import { ViewHeader } from "../../components/ui/view-header.tsx"
import { FileIcon } from "../../components/FileIcon.tsx"
import type { SearchQuery } from "./search-engine.ts"
import { highlightedParts } from "./search-highlights.ts"
import { fileRowKey, matchRowKey, resultRows, resultSummary, type ResultRow } from "./search-results.ts"
import type { SearchSession } from "./use-search-session.ts"

type Toggle = "matchCase" | "wholeWord" | "regex"

const toggles: ReadonlyArray<{ option: Toggle; label: string; code: string; icon: ReactNode }> = [
  { option: "matchCase", label: "Match Case", code: "KeyC", icon: <CaseSensitiveIcon aria-hidden="true" /> },
  { option: "wholeWord", label: "Match Whole Word", code: "KeyW", icon: <WholeWordIcon aria-hidden="true" /> },
  { option: "regex", label: "Use Regular Expression", code: "KeyR", icon: <RegexIcon aria-hidden="true" /> },
]

const altLabel = typeof navigator !== "undefined" && navigator.platform.includes("Mac") ? "⌥" : "Alt+"

function toggleChange(option: Toggle, value: boolean): Partial<SearchQuery> {
  if (option === "matchCase") return { matchCase: value }
  if (option === "wholeWord") return { wholeWord: value }
  return { regex: value }
}

function directoryOf(path: string): string {
  const slash = path.lastIndexOf("/")
  return slash < 0 ? "" : path.slice(0, slash)
}

interface SearchViewProps {
  mobile: boolean
  onDismiss(): void
  // onLeave moves focus out of the view, to the document.
  onLeave(): void
  session: SearchSession
}

export function SearchView({ mobile, onDismiss, onLeave, session }: SearchViewProps) {
  const { active, collapsed, details, query, results, status } = session
  const tree = useRef<HTMLDivElement>(null)
  const [focusedKey, setFocusedKey] = useState("")
  const rows = useMemo(() => results ? resultRows(results.files, collapsed) : [], [collapsed, results])
  const activeKey = active ? matchRowKey(active.path, active.matchIndex) : ""
  // The roving tab stop: the focused row if still visible, else the active match, else the first row.
  const tabKey = rows.some((row) => row.key === focusedKey) ? focusedKey
    : rows.some((row) => row.key === activeKey) ? activeKey : rows[0]?.key ?? ""
  const anyExpanded = results ? results.files.some((file) => !collapsed.has(file.path)) : false
  const invalidField = status.kind === "invalid" ? status.field : undefined

  function focusRow(key: string) {
    setFocusedKey(key)
    requestAnimationFrame(() => {
      const row = tree.current?.querySelector(`[data-row-key="${CSS.escape(key)}"]`)
      if (row instanceof HTMLElement) row.focus()
    })
  }

  function openRow(row: ResultRow, focusDocument: boolean) {
    if (row.kind === "file") {
      session.toggleFile(row.file.path)
      return
    }
    session.open({ path: row.file.path, matchIndex: row.matchIndex }, focusDocument)
    if (mobile) onDismiss()
  }

  function onTreeKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const index = rows.findIndex((row) => row.key === tabKey)
    const row = rows[index]
    if (!row) return
    if (event.key === "ArrowDown") {
      const next = rows[index + 1]
      if (next) focusRow(next.key)
    } else if (event.key === "ArrowUp") {
      const previous = rows[index - 1]
      if (previous) focusRow(previous.key)
      else session.focusInput()
    } else if (event.key === "Home") {
      const first = rows[0]
      if (first) focusRow(first.key)
    } else if (event.key === "End") {
      const last = rows.at(-1)
      if (last) focusRow(last.key)
    } else if (event.key === "ArrowRight") {
      if (row.kind !== "file") return
      if (collapsed.has(row.file.path)) session.toggleFile(row.file.path, true)
      else focusRow(matchRowKey(row.file.path, 0))
    } else if (event.key === "ArrowLeft") {
      if (row.kind === "match") focusRow(fileRowKey(row.file.path))
      else session.toggleFile(row.file.path, false)
    } else if (event.key === "Enter" || event.key === " ") {
      // As in VS Code lists, Space previews and Enter moves focus to the document.
      openRow(row, event.key === "Enter")
    } else {
      return
    }
    event.preventDefault()
  }

  function onInputKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    const toggle = event.altKey && !event.ctrlKey && !event.metaKey ? toggles.find((item) => item.code === event.code) : undefined
    if (toggle) {
      session.changeQuery(toggleChange(toggle.option, !query[toggle.option]))
    } else if (event.key === "ArrowDown" && rows[0]) {
      focusRow(tabKey || rows[0].key)
    } else if (event.key === "Escape") {
      if (mobile) onDismiss()
      else onLeave()
    } else {
      return
    }
    event.preventDefault()
  }

  const scopeField = (field: "include" | "exclude", label: string, placeholder: string) => (
    <label className="search-scope">
      <span>{label}</span>
      <TextField inputClassName="search-scope-input" type="text" autoComplete="off" spellCheck={false}
        placeholder={placeholder} value={query[field]} invalid={invalidField === field}
        aria-describedby={invalidField === field ? "search-invalid" : undefined}
        onChange={(event) => session.changeQuery(field === "include" ? { include: event.target.value } : { exclude: event.target.value })}
        onKeyDown={(event) => {
          if (event.key !== "Escape") return
          event.preventDefault()
          if (mobile) onDismiss()
          else onLeave()
        }} />
    </label>
  )

  return (
    <section className="search-view" aria-label="Search">
      <ViewHeader className="explorer-header sidebar-header" title="Search" actions={<>
        <IconButton label="Refresh" icon={<RefreshCwIcon aria-hidden="true" />} onClick={session.refresh} />
        <IconButton label="Clear Search Results" icon={<ListXIcon aria-hidden="true" />}
          disabled={!query.pattern && !results} onClick={session.clear} />
        <IconButton label={anyExpanded ? "Collapse All" : "Expand All"}
          icon={anyExpanded ? <FoldVerticalIcon aria-hidden="true" /> : <UnfoldVerticalIcon aria-hidden="true" />}
          disabled={!results?.files.length} onClick={() => session.setAllCollapsed(anyExpanded)} />
        {mobile && <IconButton label="Close search" icon={<XIcon aria-hidden="true" />} onClick={onDismiss} />}
      </>} />
      <div className="search-form">
        <TextField className="search-box" inputRef={session.input} id="search-input" inputClassName="search-box-input" type="text"
          autoComplete="off" spellCheck={false} placeholder="Search" aria-label="Search" value={query.pattern}
          invalid={invalidField === "pattern"} aria-describedby={invalidField === "pattern" ? "search-invalid" : undefined}
          aria-keyshortcuts="Control+Shift+F Meta+Shift+F"
          onChange={(event) => session.changeQuery({ pattern: event.target.value })} onKeyDown={onInputKeyDown}
          trailing={toggles.map((toggle) => (
            <ToggleButton key={toggle.option} label={toggle.label} pressed={query[toggle.option]}
              title={`${toggle.label} (${altLabel}${toggle.code.slice(3)})`}
              onPressedChange={(pressed) => session.changeQuery(toggleChange(toggle.option, pressed))}>
              {toggle.icon}
            </ToggleButton>
          ))} />
        <div className="search-details-toggle">
          <IconButton label="Toggle Search Details" icon={<EllipsisIcon aria-hidden="true" />} aria-expanded={details}
            aria-controls="search-details" onClick={() => session.setDetails(!details)} />
        </div>
        <div id="search-details" className="search-details" hidden={!details}>
          {scopeField("include", "files to include", "e.g. *.ts, src/**/include")}
          {scopeField("exclude", "files to exclude", "e.g. *.ts, src/**/exclude")}
        </div>
        {invalidField && <StatusMessage id="search-invalid" tone="error">{status.message}</StatusMessage>}
      </div>
      <ProgressBar active={status.kind === "searching" || status.kind === "loading"} label="Searching" />
      <div className="search-message" role="status" aria-live="polite">
        {results && results.files.length > 0 && (
          <StatusMessage tone="info">{resultSummary(results.matchCount, results.files.length, results.truncated)}</StatusMessage>
        )}
        {status.kind !== "invalid" && status.kind !== "idle" && status.kind !== "searching" && status.message && (
          <StatusMessage tone={status.kind === "error" ? "error" : status.kind === "warning" ? "warning" : "muted"}
            title={status.detail}>{status.message}</StatusMessage>
        )}
        {status.kind === "error" && (
          <Button variant="outline" onClick={session.retry}>Retry search</Button>
        )}
      </div>
      {rows.length > 0 && (
        <div ref={tree} className="search-tree" role="tree" aria-label="Search results" aria-busy={status.kind === "searching"}
          onKeyDown={onTreeKeyDown}>
          {rows.map((row) => row.kind === "file" ? (
            <ListRow key={row.key} data-row-key={row.key} className="search-file" role="treeitem" aria-level={1}
              aria-expanded={!collapsed.has(row.file.path)} aria-selected={false} tabIndex={row.key === tabKey ? 0 : -1}
              aria-label={`${row.file.name} ${directoryOf(row.file.path)}, ${row.file.matches.length} ${row.file.matches.length === 1 ? "match" : "matches"}`}
              title={row.file.path}
              onFocus={() => setFocusedKey(row.key)} onClick={() => openRow(row, false)}>
              <Chevron />
              <FileIcon path={row.file.path} />
              <span className="search-file-name">{row.file.name}</span>
              <span className="search-file-directory">{directoryOf(row.file.path)}</span>
              <Badge aria-hidden="true">{row.file.matches.length.toLocaleString()}</Badge>
            </ListRow>
          ) : (
            <ListRow key={row.key} data-row-key={row.key} className="search-match"
              role="treeitem" aria-level={2} aria-selected={row.key === activeKey} tabIndex={row.key === tabKey ? 0 : -1}
              title={`${row.file.path}:${row.match.line}`}
              onFocus={() => setFocusedKey(row.key)} onClick={() => openRow(row, false)} onDoubleClick={() => openRow(row, true)}>
              <span className="search-match-text">
                <span className="visually-hidden">Line {row.match.line}: </span>
                {highlightedParts(row.match.preview, [row.match.highlight]).map((part, index) => (
                  part.highlighted ? <mark key={index}>{part.text}</mark> : <span key={index}>{part.text}</span>
                ))}
              </span>
            </ListRow>
          ))}
        </div>
      )}
    </section>
  )
}
