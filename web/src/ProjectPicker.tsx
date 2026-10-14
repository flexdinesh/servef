import {
  CheckIcon,
  CornerDownLeftIcon,
  FolderGit2Icon,
  FolderIcon,
  FolderOpenIcon,
  FolderSearchIcon,
  GitBranchIcon,
  XIcon,
} from "lucide-react"
import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react"

import { Button } from "./components/ui/button.tsx"
import {
  browseRows,
  parentInput,
  projectSections,
  selectableRows,
  splitBrowseInput,
  type PickerRow,
  type PickerSection,
} from "./project-picker.ts"
import type { DirectoriesResponse, ProjectsResponse } from "./projects-contract.ts"
import { fetchDirectories, fetchProjects, takeLandingPick } from "./projects-data.ts"
import { useDelayedFlag } from "./use-document-format.ts"
import { forgetProject, readMemory, rememberedProjects, updateMemory } from "./workspace-memory.ts"

type Load<T> =
  | { kind: "loading"; previous: T | null }
  | { kind: "ready"; value: T }
  | { kind: "error"; message: string; previous: T | null }

interface ProjectPickerProps {
  project: string
  registerOpen(open: (() => void) | null): void
  onOpen(project: string): void
}

export const projectShortcutLabel = typeof navigator !== "undefined" && navigator.platform.includes("Mac") ? "⌘O" : "Ctrl+O"

function loaded<T>(load: Load<T> | null): T | null {
  if (!load) return null
  return load.kind === "ready" ? load.value : load.previous
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

function isAbort(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError"
}

// ProjectPicker is a quick pick: Projects lists discovered repositories and
// opened folders; Browse walks folders under the served root.
export function ProjectPicker({ project, registerOpen, onOpen }: ProjectPickerProps) {
  const dialog = useRef<HTMLDialogElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLDivElement>(null)
  const returnFocus = useRef<HTMLElement | null>(null)
  const controller = useRef<AbortController | null>(null)
  const directoryCache = useRef(new Map<string, Load<DirectoriesResponse>>())
  const [open, setOpen] = useState(false)
  const [mode, setMode] = useState<"projects" | "browse">("projects")
  const [query, setQuery] = useState("")
  const [path, setPath] = useState("")
  const [projects, setProjects] = useState<Load<ProjectsResponse> | null>(null)
  const [directories, setDirectories] = useState<ReadonlyMap<string, Load<DirectoriesResponse>>>(() => new Map())
  const [memoryVersion, setMemoryVersion] = useState(0)
  const [activeKey, setActiveKey] = useState("")

  const loadProjects = useCallback(() => {
    controller.current?.abort()
    const next = new AbortController()
    controller.current = next
    setProjects((current) => ({ kind: "loading", previous: loaded(current) }))
    fetchProjects(next.signal).then(
      (value) => setProjects({ kind: "ready", value }),
      (error: unknown) => {
        if (isAbort(error)) return
        setProjects((current) => ({ kind: "error", message: errorMessage(error, "Could not list projects."), previous: loaded(current) }))
      },
    )
  }, [])

  const show = useCallback(() => {
    const element = dialog.current
    if (!element) return
    if (!element.open) {
      returnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
      element.showModal()
    }
    directoryCache.current = new Map()
    setDirectories(new Map())
    setMode("projects")
    setQuery("")
    setPath("")
    setActiveKey("")
    setOpen(true)
    loadProjects()
    input.current?.focus()
  }, [loadProjects])

  const close = useCallback(() => {
    controller.current?.abort()
    if (dialog.current?.open) dialog.current.close()
    setOpen(false)
    returnFocus.current?.focus({ preventScroll: true })
  }, [])

  useEffect(() => {
    registerOpen(show)
    const shortcut = (event: globalThis.KeyboardEvent) => {
      if (event.defaultPrevented || event.altKey || event.shiftKey || !(event.metaKey || event.ctrlKey)) return
      if (event.key.toLowerCase() !== "o") return
      event.preventDefault()
      show()
    }
    document.addEventListener("keydown", shortcut)
    if (takeLandingPick()) show()
    return () => {
      registerOpen(null)
      document.removeEventListener("keydown", shortcut)
      controller.current?.abort()
    }
  }, [registerOpen, show])

  const { folder, partial } = splitBrowseInput(path)
  useEffect(() => {
    if (!open || mode !== "browse" || directoryCache.current.has(folder)) return
    const store = (load: Load<DirectoriesResponse>) => {
      directoryCache.current.set(folder, load)
      setDirectories(new Map(directoryCache.current))
    }
    const abort = new AbortController()
    store({ kind: "loading", previous: null })
    fetchDirectories(folder, abort.signal).then(
      (value) => store({ kind: "ready", value }),
      (error: unknown) => {
        if (isAbort(error)) {
          directoryCache.current.delete(folder)
          return
        }
        store({ kind: "error", message: errorMessage(error, "Could not read that folder."), previous: null })
      },
    )
    return () => abort.abort()
  }, [folder, mode, open])

  const projectList = loaded(projects)
  const servedRoot = projectList?.servedRoot ?? ""
  const rootName = projectList?.rootName ?? ""
  // Read on render so a forgotten folder (memoryVersion) disappears at once.
  const remembered = useMemo(
    () => servedRoot && memoryVersion >= 0 ? rememberedProjects(readMemory(), servedRoot) : [],
    [servedRoot, memoryVersion],
  )
  const folderLoad = directories.get(folder) ?? null
  const sections: PickerSection[] = useMemo(() => {
    if (mode === "browse") return browseRows(folder, partial, loaded(folderLoad)?.directories ?? [])
    return projectList ? projectSections(projectList, remembered, project, query) : []
  }, [folder, folderLoad, mode, partial, project, projectList, query, remembered])
  const rows = useMemo(() => selectableRows(sections), [sections])
  const active = rows.find((row) => row.key === activeKey) ?? rows[0]
  const scanning = useDelayedFlag(open && mode === "projects" && projects?.kind === "loading", 150)
  const listing = useDelayedFlag(open && mode === "browse" && folderLoad?.kind === "loading", 150)

  useEffect(() => {
    if (!active) return
    list.current?.querySelector(`[data-key="${CSS.escape(active.key)}"]`)?.scrollIntoView({ block: "nearest" })
  }, [active])

  function activate(row: PickerRow | undefined) {
    if (!row) return
    switch (row.kind) {
      case "project":
        if (row.disabled) return
        close()
        if (row.id !== project) onOpen(row.id)
        return
      case "browse":
        setMode("browse")
        setPath("")
        setActiveKey("")
        input.current?.focus()
        return
      case "open":
        close()
        if (row.path !== project) onOpen(row.path)
        return
      case "directory":
        setPath(`${row.path}/`)
        setActiveKey("")
        input.current?.focus()
    }
  }

  function forget(id: string) {
    if (!servedRoot) return
    updateMemory((memory) => forgetProject(memory, servedRoot, id))
    setMemoryVersion((version) => version + 1)
    input.current?.focus()
  }

  function changeInput(value: string) {
    setActiveKey("")
    if (mode === "projects") {
      if (!query && (value === "/" || value === "./")) {
        setMode("browse")
        setPath("")
        return
      }
      setQuery(value)
      return
    }
    setPath(value.replace(/^(\.?\/)+/, ""))
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      if (!rows.length) return
      const index = active ? rows.indexOf(active) : -1
      const step = event.key === "ArrowDown" ? 1 : -1
      setActiveKey(rows[(index + step + rows.length) % rows.length]?.key ?? "")
    } else if (event.key === "Enter") {
      event.preventDefault()
      activate(active)
    } else if (event.key === "Escape") {
      event.preventDefault()
      close()
    } else if (event.key === "Tab" && mode === "browse") {
      const target = active?.kind === "directory" ? active : rows.find((row) => row.kind === "directory")
      if (target?.kind !== "directory") return
      event.preventDefault()
      setPath(`${target.path}/`)
      setActiveKey("")
    } else if (event.key === "Backspace" && mode === "browse" && event.currentTarget.selectionStart === event.currentTarget.selectionEnd) {
      if (path === "") {
        event.preventDefault()
        setMode("projects")
        setActiveKey("")
      } else if (path.endsWith("/") && event.currentTarget.selectionStart === path.length) {
        event.preventDefault()
        setPath(parentInput(path))
        setActiveKey("")
      }
    }
  }

  const status = mode === "projects"
    ? projects?.kind === "error"
      ? { tone: "error", text: `Couldn't list projects: ${projects.message}` }
      : scanning
        ? { tone: "muted", text: "Scanning for projects…" }
        : projectList && query.trim() && !sections.some((section) => section.title)
          ? { tone: "muted", text: `No projects match “${query.trim()}”.` }
          : null
    : folderLoad?.kind === "error"
      ? { tone: "error", text: folderLoad.message }
      : listing
        ? { tone: "muted", text: "Reading folder…" }
        : folderLoad?.kind === "ready" && !sections.some((section) => section.title)
          ? { tone: "muted", text: partial ? `No folders match “${partial}”.` : "No folders here." }
          : null
  const warnings = mode === "projects" ? projectList?.warnings ?? [] : []
  const activeID = active ? `project-option-${active.key}` : undefined

  return (
    <dialog
      ref={dialog}
      className="search-dialog project-dialog"
      aria-labelledby="project-picker-title"
      onCancel={(event) => { event.preventDefault(); close() }}
      onClick={(event) => { if (event.target === dialog.current) close() }}
    >
      <div className="search-shell">
        <h2 id="project-picker-title" className="visually-hidden">
          {mode === "projects" ? "Switch project" : "Open folder"}
        </h2>
        <div className="search-field project-field">
          {mode === "projects" ? <FolderGit2Icon aria-hidden="true" /> : <FolderSearchIcon aria-hidden="true" />}
          {mode === "browse" && <span className="project-prefix" title={servedRoot}>{rootName || "root"}/</span>}
          <input
            ref={input}
            className="search-input"
            type="text"
            autoComplete="off"
            spellCheck={false}
            placeholder={mode === "projects" ? "Switch project, / to browse" : "Type a folder path"}
            aria-label={mode === "projects" ? "Switch project" : `Folder path under ${rootName || "the served folder"}`}
            role="combobox"
            aria-controls="project-results"
            aria-expanded={open}
            aria-autocomplete="list"
            aria-activedescendant={activeID}
            value={mode === "projects" ? query : path}
            onChange={(event) => changeInput(event.target.value)}
            onKeyDown={onKeyDown}
          />
          <kbd>Esc</kbd>
        </div>
        {(status || warnings.length > 0) && (
          <div className={`search-status project-status${status?.tone === "error" ? " search-status-error" : ""}`} role="status" aria-live="polite">
            {status && <span>{status.text}</span>}
            {status?.tone === "error" && mode === "projects" && (
              <Button type="button" size="sm" variant="outline" onClick={loadProjects}>Retry</Button>
            )}
            {!status && warnings.length > 0 && (
              <span title={warnings.join("\n")}>
                {warnings.length === 1 ? "1 folder couldn't be read." : `${warnings.length} folders couldn't be read.`}
              </span>
            )}
          </div>
        )}
        <div ref={list} id="project-results" className="search-results project-results" role="listbox"
          aria-label={mode === "projects" ? "Projects" : "Folders"}>
          {sections.map((section, sectionIndex) => (
            <div key={section.title || `actions-${sectionIndex}`} role="group" aria-label={section.title || undefined}
              className={section.title ? "project-group" : "project-group project-actions"}>
              {section.title && <h3 className="project-group-title" aria-hidden="true">{section.title}</h3>}
              {section.rows.map((row) => (
                <PickerOption key={row.key} row={row} active={row === active} rootName={rootName}
                  onActivate={() => activate(row)}
                  onHover={() => setActiveKey(row.key)}
                  onForget={row.kind === "project" && row.projectKind === "opened" && !row.current ? () => forget(row.id) : undefined} />
              ))}
            </div>
          ))}
        </div>
      </div>
    </dialog>
  )
}

interface PickerOptionProps {
  row: PickerRow
  active: boolean
  rootName: string
  onActivate(): void
  onHover(): void
  onForget?(): void
}

function PickerOption({ row, active, rootName, onActivate, onHover, onForget }: PickerOptionProps) {
  const disabled = row.kind === "project" && row.disabled
  const className = [
    "project-option",
    active ? "active" : "",
    disabled ? "disabled" : "",
    row.kind === "project" && row.nested ? "nested" : "",
  ].filter(Boolean).join(" ")
  return (
    <div className="project-option-row" data-key={row.key}>
      <div
        id={`project-option-${row.key}`}
        className={className}
        role="option"
        aria-selected={active}
        aria-disabled={disabled || undefined}
        onClick={onActivate}
        onMouseMove={disabled || active ? undefined : onHover}
      >
        <OptionIcon row={row} />
        <OptionText row={row} rootName={rootName} />
      </div>
      {onForget && (
        <button type="button" className="project-forget" tabIndex={-1}
          aria-label={`Forget ${row.kind === "project" ? row.path : ""}`} title="Remove from opened folders"
          onClick={onForget}>
          <XIcon aria-hidden="true" />
        </button>
      )}
    </div>
  )
}

function OptionIcon({ row }: { row: PickerRow }) {
  switch (row.kind) {
    case "browse": return <FolderSearchIcon aria-hidden="true" />
    case "open": return <CornerDownLeftIcon aria-hidden="true" />
    case "directory": return row.isProject ? <FolderGit2Icon aria-hidden="true" /> : <FolderIcon aria-hidden="true" />
    case "project":
      switch (row.projectKind) {
        case "repo": return <FolderGit2Icon aria-hidden="true" />
        case "worktree": return <GitBranchIcon aria-hidden="true" />
        case "opened": return <FolderOpenIcon aria-hidden="true" />
        case "folder": return <FolderIcon aria-hidden="true" />
      }
  }
}

// OptionPath truncates from the start so the deepest folders stay visible.
function OptionPath({ path }: { path: string }) {
  return <span className="project-option-path project-option-path-tail" title={path}><span dir="ltr">{path}</span></span>
}

function OptionText({ row, rootName }: { row: PickerRow; rootName: string }) {
  switch (row.kind) {
    case "browse":
      return <>
        <span className="project-option-name">Open folder…</span>
        <span className="project-option-grow" />
        <kbd className="project-option-meta">/</kbd>
      </>
    case "open":
      return <>
        <span className="project-option-name">Open</span>
        <OptionPath path={row.path === "." ? `${rootName || "root"}/` : `${row.path}/`} />
      </>
    case "directory":
      return <>
        <span className="project-option-name">{row.name}</span>
        <span className="project-option-grow" />
        {row.isProject && <span className="project-option-meta">git</span>}
      </>
    case "project":
      return <>
        <span className="project-option-name">{row.name}</span>
        {row.path !== row.name && row.path !== "." && <OptionPath path={row.path} />}
        {row.path === "." && <span className="project-option-path">served folder</span>}
        <span className="project-option-grow" />
        {row.disabled && <span className="project-option-meta">{row.note}</span>}
        {row.current && <span className="project-option-current"><CheckIcon aria-hidden="true" />Current</span>}
      </>
  }
}
