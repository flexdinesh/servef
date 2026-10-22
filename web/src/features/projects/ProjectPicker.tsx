import {
  CheckIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  CornerDownLeftIcon,
  FolderGit2Icon,
  FolderIcon,
  FolderOpenIcon,
  FolderSearchIcon,
  GitBranchIcon,
  GitForkIcon,
  XIcon,
} from "lucide-react"
import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react"

import { Button } from "../../components/ui/button.tsx"
import { isAbort } from "../../api/http.ts"
import {
  browseRows,
  browseStart,
  browseTarget,
  canDrill,
  currentGroup,
  groupName,
  parentInput,
  projectSections,
  selectableRows,
  splitBrowseInput,
  type PickerRow,
  type PickerSection,
} from "./project-picker.ts"
import type { DirectoriesResponse, ProjectsResponse } from "../../api/projects-contract.ts"
import { fetchDirectories, fetchProjects, takeLandingPick } from "../../api/projects-data.ts"
import { useDelayedFlag } from "../documents/use-document-format.ts"
import { forgetProject, readMemory, rememberedProjects, updateMemory } from "../../workspace/workspace-memory.ts"

type Load<T> =
  | { kind: "loading"; previous: T | null }
  | { kind: "ready"; value: T }
  | { kind: "error"; message: string; previous: T | null }

export interface PickerOpenOptions {
  // worktrees opens on the current project's repository worktrees.
  worktrees?: boolean
}

interface ProjectPickerProps {
  project: string
  registerOpen(open: ((options?: PickerOpenOptions) => void) | null): void
  onOpen(project: string): void
}

const noRoots: readonly string[] = []

export const projectShortcutLabel = typeof navigator !== "undefined" && navigator.platform.includes("Mac") ? "⌘O" : "Ctrl+O"

function loaded<T>(load: Load<T> | null): T | null {
  if (!load) return null
  return load.kind === "ready" ? load.value : load.previous
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

// ProjectPicker is a quick pick: Projects lists served folders, discovered
// repositories (each opening into its worktrees), and opened folders; Browse
// walks folders under the served roots that are not inside a repository.
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
  // group is the repository whose worktrees are listed; "" lists projects.
  const [group, setGroup] = useState("")
  // followCurrent lists the current project's worktrees once projects load.
  const [followCurrent, setFollowCurrent] = useState(false)
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
        setProjects((current) => ({ kind: "error", message: errorMessage(error, "Couldn't list projects."), previous: loaded(current) }))
      },
    )
  }, [])

  const show = useCallback((options?: PickerOpenOptions) => {
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
    setGroup("")
    setFollowCurrent(options?.worktrees === true)
    setPath("")
    setActiveKey("")
    // Re-read memory: projects opened since mount changed it.
    setMemoryVersion((version) => version + 1)
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

  const projectList = loaded(projects)
  const roots = useMemo(() => projectList?.roots.map((root) => root.path) ?? noRoots, [projectList])
  const { folder, partial } = splitBrowseInput(path, roots)
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

  // Read on render so a forgotten folder (memoryVersion) disappears at once.
  const remembered = useMemo(() => memoryVersion >= 0 ? rememberedProjects(readMemory()) : [], [memoryVersion])
  const listedGroup = followCurrent ? projectList ? currentGroup(projectList, project) : "" : group
  const folderLoad = directories.get(folder) ?? null
  const sections: PickerSection[] = useMemo(() => {
    if (mode === "browse") return browseRows(folder, partial, loaded(folderLoad)?.directories ?? [])
    return projectList ? projectSections(projectList, remembered, project, query, listedGroup) : []
  }, [folder, folderLoad, listedGroup, mode, partial, project, projectList, query, remembered])
  const rows = useMemo(() => selectableRows(sections), [sections])
  const active = rows.find((row) => row.key === activeKey) ?? rows[0]
  const scanning = useDelayedFlag(open && mode === "projects" && projects?.kind === "loading", 150)
  const listing = useDelayedFlag(open && mode === "browse" && folderLoad?.kind === "loading", 150)

  useEffect(() => {
    if (!active) return
    list.current?.querySelector(`[data-key="${CSS.escape(active.key)}"]`)?.scrollIntoView({ block: "nearest" })
  }, [active])

  function openProject(id: string) {
    close()
    if (id !== project) onOpen(id)
  }

  function startBrowse() {
    setMode("browse")
    setPath(browseStart(roots))
    setActiveKey("")
    input.current?.focus()
  }

  // listGroup lists a repository's worktrees, or projects again for "".
  function listGroup(next: string) {
    setFollowCurrent(false)
    setGroup(next)
    setQuery("")
    setActiveKey("")
    input.current?.focus()
  }

  function activate(row: PickerRow | undefined) {
    if (!row) return
    switch (row.kind) {
      case "project":
        if (!row.disabled) openProject(row.id)
        return
      case "browse":
        startBrowse()
        return
      case "open":
        openProject(row.path)
        return
      case "directory":
        if (row.isProject) {
          openProject(row.path)
          return
        }
        setPath(`${row.path}/`)
        setActiveKey("")
        input.current?.focus()
    }
  }

  function forget(id: string) {
    updateMemory((memory) => forgetProject(memory, id))
    setMemoryVersion((version) => version + 1)
    input.current?.focus()
  }

  function changeInput(value: string) {
    setActiveKey("")
    if (mode === "projects") {
      if (!query && !listedGroup && (value === "/" || value === "./")) {
        startBrowse()
        return
      }
      setQuery(value)
      return
    }
    setPath(value)
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
    } else if (mode === "projects" && !listedGroup && canDrill(active)
      && (event.key === "Tab" || (event.key === "ArrowRight" && event.currentTarget.selectionStart === query.length))) {
      event.preventDefault()
      listGroup(active.group)
    } else if (mode === "projects" && listedGroup && query === "" && (event.key === "Backspace" || event.key === "ArrowLeft")) {
      event.preventDefault()
      listGroup("")
    } else if (event.key === "Tab" && mode === "browse") {
      const target = browseTarget(rows, active)
      if (!target) return
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
        setPath(parentInput(path, roots))
        setActiveKey("")
      }
    }
  }

  const status = mode === "projects"
    ? projects?.kind === "error"
      ? { tone: "error", text: projects.message }
      : scanning
        ? { tone: "muted", text: "Scanning for projects…" }
        : projectList && query.trim() && !sections.some((section) => section.title)
          ? { tone: "muted", text: listedGroup ? `No worktrees match “${query.trim()}”.` : `No projects match “${query.trim()}”.` }
          : null
    : folderLoad?.kind === "error"
      ? { tone: "error", text: folderLoad.message }
      : listing
        ? { tone: "muted", text: "Reading folder…" }
        : folderLoad?.kind === "ready" && !sections.some((section) => section.title)
          ? { tone: "muted", text: partial ? `No folders match “${partial}”.` : "No folders here." }
          : null
  const listedName = listedGroup && projectList ? groupName(projectList, listedGroup) : ""
  const label = mode === "browse" ? "Folder path" : listedGroup ? `Switch ${listedName} worktree` : "Switch project"
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
          {mode === "browse" ? <FolderSearchIcon aria-hidden="true" /> : listedGroup ? <GitBranchIcon aria-hidden="true" /> : <FolderGit2Icon aria-hidden="true" />}
          {mode === "projects" && listedGroup && (
            <button type="button" className="project-prefix project-back" title="Back to projects (Backspace)"
              aria-label="Back to projects" onClick={() => listGroup("")}>
              <ChevronLeftIcon aria-hidden="true" />{listedName}
            </button>
          )}
          <input
            ref={input}
            className="search-input"
            type="text"
            autoComplete="off"
            spellCheck={false}
            placeholder={mode === "browse" ? "Type a folder path" : listedGroup ? "Filter worktrees" : "Switch project, → for worktrees, / to browse"}
            aria-label={label}
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
          aria-label={mode === "browse" ? "Folders" : listedGroup ? "Worktrees" : "Projects"}>
          {sections.map((section, sectionIndex) => (
            <div key={section.title || `actions-${sectionIndex}`} role="group" aria-label={section.title || undefined}
              className={section.title ? "project-group" : "project-group project-actions"}>
              {section.title && <h3 className="project-group-title" aria-hidden="true">{section.title}</h3>}
              {section.rows.map((row) => (
                <PickerOption key={row.key} row={row} active={row === active}
                  onActivate={() => activate(row)}
                  onHover={() => setActiveKey(row.key)}
                  onDrill={canDrill(row) ? () => listGroup(row.group) : undefined}
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
  onActivate(): void
  onHover(): void
  onDrill?(): void
  onForget?(): void
}

function PickerOption({ row, active, onActivate, onHover, onDrill, onForget }: PickerOptionProps) {
  const disabled = row.kind === "project" && row.disabled
  const className = [
    "project-option",
    active ? "active" : "",
    disabled ? "disabled" : "",
  ].filter(Boolean).join(" ")
  return (
    <div className={onDrill ? "project-option-row drillable" : "project-option-row"} data-key={row.key}>
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
        <OptionText row={row} />
      </div>
      {onDrill && row.kind === "project" && (
        <button type="button" className="project-drill" tabIndex={-1}
          aria-label={`Show ${row.name} worktrees`} title="Show worktrees (→)" onClick={onDrill}>
          <ChevronRightIcon aria-hidden="true" />
        </button>
      )}
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
        case "worktree": return <GitForkIcon className="worktree-icon" aria-hidden="true" />
        case "opened": return <FolderOpenIcon aria-hidden="true" />
        case "folder": return <FolderIcon aria-hidden="true" />
      }
  }
}

// OptionPath truncates from the start so the deepest folders stay visible.
function OptionPath({ path, title = path }: { path: string; title?: string }) {
  return <span className="project-option-path project-option-path-tail" title={title}><span dir="ltr">{path}</span></span>
}

function OptionText({ row }: { row: PickerRow }) {
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
        <OptionPath path={`${row.path}/`} />
      </>
    case "directory":
      return <>
        <span className="project-option-name">{row.name}</span>
        <span className="project-option-grow" />
        {row.isProject && <span className="project-option-meta">git · opens whole</span>}
      </>
    case "project":
      return <>
        {row.repository && row.projectKind !== "opened" && <span className="project-option-meta">{row.repository} ›</span>}
        <span className="project-option-name">{row.name}</span>
        {row.branch && row.branch !== row.name && <span className="project-option-branch">{row.branch}</span>}
        {row.location
          ? row.location !== row.name && <OptionPath path={row.location} title={row.path} />
          : <span className="project-option-path" title={row.path}>served folder</span>}
        <span className="project-option-grow" />
        {row.disabled && <span className="project-option-meta">{row.note}</span>}
        {row.worktrees > 1 && <span className="project-option-meta">{row.worktrees} worktrees</span>}
        {row.current && <span className="project-option-current"><CheckIcon aria-hidden="true" />Current</span>}
      </>
  }
}
