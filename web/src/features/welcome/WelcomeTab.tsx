import {
  ALargeSmallIcon, ArrowUpRightIcon, BracesIcon, FileSearchIcon, FolderGit2Icon, FunnelIcon, GitBranchIcon, PaletteIcon, RouteIcon,
  SearchIcon, SquareTerminalIcon, XIcon,
} from "lucide-react"
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react"

import { Button } from "../../components/ui/button.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { Kbd } from "../../components/ui/kbd.tsx"
import { ListButton } from "../../components/ui/list-row.tsx"
import { Chevron } from "../../components/ui/section-toggle.tsx"
import { TextField } from "../../components/ui/text-field.tsx"
import { isAbort } from "../../api/http.ts"
import type { ProjectsResponse } from "../../api/projects-contract.ts"
import { fetchProjects } from "../../api/projects-data.ts"
import { useDelayedFlag } from "../../lib/use-delayed-flag.ts"
import { readMemory, rememberedProjects } from "../../workspace/workspace-memory.ts"
import { OptionIcon, OptionText } from "../projects/ProjectPicker.tsx"
import type { ProjectRow } from "../projects/project-picker.ts"
import {
  documentationUrl, guideEntries, listsProjects, projectLines, startEntries, welcomeSections,
  type ProjectFacts, type WelcomeAction, type WelcomeEntry, type WelcomeProjectRow,
} from "./welcome-model.ts"

const mac = typeof navigator !== "undefined" && navigator.platform.includes("Mac")

const icons: Record<WelcomeAction, ReactNode> = {
  "open-project": <FolderGit2Icon aria-hidden="true" />,
  "find-file": <FileSearchIcon aria-hidden="true" />,
  "show-commands": <SquareTerminalIcon aria-hidden="true" />,
  "find-in-files": <SearchIcon aria-hidden="true" />,
  "source-control": <GitBranchIcon aria-hidden="true" />,
  "walkthroughs": <RouteIcon aria-hidden="true" />,
  "language-servers": <BracesIcon aria-hidden="true" />,
  "color-theme": <PaletteIcon aria-hidden="true" />,
  "font-size": <ALargeSmallIcon aria-hidden="true" />,
  "review-uncommitted": <GitBranchIcon aria-hidden="true" />,
}

type Load =
  | { kind: "loading"; previous: ProjectsResponse | null }
  | { kind: "ready"; value: ProjectsResponse }
  | { kind: "error"; message: string; previous: ProjectsResponse | null }

function loaded(load: Load): ProjectsResponse | null {
  return load.kind === "ready" ? load.value : load.previous
}

interface WelcomeTabProps {
  project: string
  // roots names the served folders.
  roots: readonly string[]
  facts: ProjectFacts
  onAction(action: WelcomeAction): void
  onOpenProject(project: string): void
  onStartupSetting(): void
}

// WelcomeTab (decision 0052) starts a session: open a project, a file, or a
// command; see the open project's branch, changes, and walkthroughs; switch
// among the served projects; and learn what servef does by doing it.
export function WelcomeTab({ facts, onAction, onOpenProject, onStartupSetting, project, roots }: WelcomeTabProps) {
  const lines = projectLines(facts)
  return (
    <div className="welcome">
      <header className="welcome-header">
        <p className="welcome-meta">Serving {roots.join(", ")}</p>
        <h1 className="welcome-title">servef</h1>
        <p className="welcome-tagline">Browse, search, and review local files.</p>
      </header>
      <div className="welcome-columns">
        <div className="welcome-column">
          <section className="welcome-section" aria-labelledby="welcome-start">
            <h2 id="welcome-start">Start</h2>
            <EntryList entries={startEntries(mac)} onAction={onAction} />
          </section>
          {lines.length > 0 && (
            <section className="welcome-section" aria-labelledby="welcome-project">
              <h2 id="welcome-project">This project</h2>
              <ul className="welcome-facts">
                {lines.map((line) => (
                  <li key={line.key}>
                    {line.action ? (
                      <Button variant="link" size="inline" onClick={() => { if (line.action) onAction(line.action) }}>{line.text}</Button>
                    ) : line.text}
                  </li>
                ))}
              </ul>
            </section>
          )}
          <WelcomeProjects project={project} onOpenProject={onOpenProject} />
        </div>
        <div className="welcome-column">
          <section className="welcome-section" aria-labelledby="welcome-guide">
            <h2 id="welcome-guide">What you can do</h2>
            <EntryList entries={guideEntries(mac)} onAction={onAction} />
            <a className="welcome-docs" href={documentationUrl} target="_blank" rel="noreferrer">
              Documentation<ArrowUpRightIcon aria-hidden="true" /><span className="visually-hidden"> (opens in a new tab)</span>
            </a>
          </section>
        </div>
      </div>
      <p className="welcome-footer">
        Shown when a project opens with no tabs.{" "}
        <Button variant="link" size="inline" onClick={onStartupSetting}>Change in Settings</Button>
      </p>
    </div>
  )
}

function EntryList({ entries, onAction }: { entries: readonly WelcomeEntry[]; onAction(action: WelcomeAction): void }) {
  return (
    <ul className="welcome-entries">
      {entries.map((entry) => (
        <li key={entry.action} className="welcome-entry">
          {icons[entry.action]}
          <span className="welcome-entry-text">
            <Button variant="link" size="inline" onClick={() => onAction(entry.action)}>{entry.label}</Button>
            {entry.description && <span className="welcome-entry-description">{entry.description}</span>}
          </span>
          {entry.shortcut && <Kbd>{entry.shortcut}</Kbd>}
        </li>
      ))}
    </ul>
  )
}

// WelcomeProjects lists the served projects, as the picker does, once there
// is more than one to choose from.
function WelcomeProjects({ project, onOpenProject }: { project: string; onOpenProject(project: string): void }) {
  const [load, setLoad] = useState<Load>({ kind: "loading", previous: null })
  const [query, setQuery] = useState("")
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set())
  const controller = useRef<AbortController | null>(null)
  const remembered = useMemo(() => rememberedProjects(readMemory()), [])

  // The list starts loading; Retry marks it loading again.
  const read = useCallback(() => {
    controller.current?.abort()
    const next = new AbortController()
    controller.current = next
    fetchProjects(next.signal).then(
      (value) => setLoad({ kind: "ready", value }),
      (error: unknown) => {
        if (isAbort(error)) return
        const message = error instanceof Error && error.message ? error.message : "Couldn't list projects."
        setLoad((current) => ({ kind: "error", message, previous: loaded(current) }))
      },
    )
  }, [])
  useEffect(() => {
    read()
    return () => controller.current?.abort()
  }, [read])

  const response = loaded(load)
  const sections = useMemo(() => response ? welcomeSections(response, remembered, project, query) : [],
    [project, query, remembered, response])
  const scanning = useDelayedFlag(load.kind === "loading" && response === null, 150)
  if (response && !listsProjects(response)) return null
  if (!response && load.kind === "loading" && !scanning) return null

  const toggle = (group: string) => setExpanded((current) => {
    const next = new Set(current)
    if (next.has(group)) next.delete(group)
    else next.add(group)
    return next
  })
  const trimmed = query.trim()
  return (
    <section className="welcome-section welcome-projects" aria-labelledby="welcome-projects">
      <h2 id="welcome-projects">Projects</h2>
      {response && (
        <TextField className="welcome-projects-filter" icon={<FunnelIcon />} type="text" autoComplete="off" spellCheck={false}
          placeholder="Filter projects" aria-label="Filter projects" value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => { if (event.key === "Escape" && query) { event.stopPropagation(); setQuery("") } }}
          trailing={query && <IconButton size="xs" label="Clear filter" icon={<XIcon aria-hidden="true" />} onClick={() => setQuery("")} />} />
      )}
      <div className="welcome-projects-status" role="status" aria-live="polite">
        {load.kind === "error" ? <>
          <span>{load.message}</span>
          <Button variant="outline" onClick={() => {
            setLoad((current) => ({ kind: "loading", previous: loaded(current) }))
            read()
          }}>Retry</Button>
        </> : !response ? "Scanning for projects…"
          : trimmed && sections.length === 0 ? `No projects match “${trimmed}”.` : ""}
      </div>
      {sections.map((section) => (
        <div key={section.title} className="welcome-project-group">
          {sections.length > 1 && <h3 className="project-group-title">{section.title}</h3>}
          <ul aria-label={section.title}>
            {section.rows.map((entry) => (
              <ProjectEntry key={entry.row.key} entry={entry} expanded={expanded.has(entry.row.group)}
                onToggle={() => toggle(entry.row.group)} onOpen={onOpenProject} />
            ))}
          </ul>
        </div>
      ))}
    </section>
  )
}

interface ProjectEntryProps {
  entry: WelcomeProjectRow
  expanded: boolean
  onToggle(): void
  onOpen(project: string): void
}

function ProjectEntry({ entry, expanded, onToggle, onOpen }: ProjectEntryProps) {
  const { row, worktrees } = entry
  return (
    <li>
      <div className={worktrees.length ? "project-option-row drillable" : "project-option-row"}>
        <ProjectButton row={row} onOpen={onOpen} />
        {worktrees.length > 0 && (
          <IconButton size="xs" className="project-drill welcome-project-expand" aria-expanded={expanded}
            label={`${expanded ? "Hide" : "Show"} ${row.name} worktrees`} icon={<Chevron />} onClick={onToggle} />
        )}
      </div>
      {expanded && worktrees.length > 0 && (
        <ul className="welcome-worktrees" aria-label={`${row.name} worktrees`}>
          {worktrees.map((member) => (
            <li key={member.key} className="project-option-row"><ProjectButton row={member} onOpen={onOpen} /></li>
          ))}
        </ul>
      )}
    </li>
  )
}

function ProjectButton({ row, onOpen }: { row: ProjectRow; onOpen(project: string): void }) {
  const inert = row.disabled || row.current
  return (
    <ListButton density="pick" className={row.disabled ? "project-option disabled" : "project-option"}
      aria-current={row.current || undefined} aria-disabled={row.disabled || undefined} title={row.path}
      onClick={() => { if (!inert) onOpen(row.id) }}>
      <OptionIcon row={row} />
      <OptionText row={row} />
    </ListButton>
  )
}
