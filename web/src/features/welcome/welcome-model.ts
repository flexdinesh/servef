import type { ProjectsResponse } from "../../api/projects-contract.ts"
import { projectSections, type ProjectRow } from "../projects/project-picker.ts"

// Pure content for the Welcome tab (decision 0052); WelcomeTab.tsx renders it.

export type WelcomeAction =
  | "open-project"
  | "find-file"
  | "show-commands"
  | "find-in-files"
  | "source-control"
  | "walkthroughs"
  | "language-servers"
  | "color-theme"
  | "font-size"
  | "review-uncommitted"

export interface WelcomeEntry {
  action: WelcomeAction
  label: string
  description?: string
  shortcut?: string
}

export const documentationUrl = "https://github.com/flexdinesh/servef#readme"

// startEntries are the actions that begin a session.
export function startEntries(mac: boolean): WelcomeEntry[] {
  return [
    { action: "open-project", label: "Open project…", shortcut: mac ? "⌘O" : "Ctrl+O" },
    { action: "find-file", label: "Find a file", shortcut: mac ? "⌘K" : "Ctrl+K" },
    { action: "show-commands", label: "Show all commands", shortcut: mac ? "⇧⌘P" : "Ctrl+Shift+P" },
  ]
}

// guideEntries are what servef does, each running the action it describes.
export function guideEntries(mac: boolean): WelcomeEntry[] {
  return [
    {
      action: "find-in-files", label: "Find in files", shortcut: mac ? "⇧⌘F" : "Ctrl+Shift+F",
      description: "Search every file's contents, skipping what git ignores.",
    },
    {
      action: "source-control", label: "Review changes", shortcut: mac ? "⌃⇧G" : "Ctrl+Shift+G",
      description: "Read uncommitted work, commits, branches, and ranges as one stacked diff.",
    },
    {
      action: "walkthroughs", label: "Follow walkthroughs",
      description: "Step through guided tours of this code that agents publish.",
    },
    {
      action: "language-servers", label: "Go to definition", shortcut: "F12",
      description: "Jump through code and find references with installed language servers.",
    },
    {
      action: "color-theme", label: "Change the color theme",
      description: "Preview each theme live as you move through the list.",
    },
    {
      action: "font-size", label: "Adjust the font size",
      description: "Scale the whole interface from one setting.",
    },
  ]
}

export interface ProjectFacts {
  repository: boolean
  branch: string
  // changes is the uncommitted file count; null until read.
  changes: number | null
  // walkthroughs is the listed count and unseen those new since the view
  // last showed; walkthroughs is null until read.
  walkthroughs: number | null
  unseen: number
}

export interface ProjectLine {
  key: string
  text: string
  action?: WelcomeAction
}

function count(value: number, one: string, many: string): string {
  return `${value.toLocaleString()} ${value === 1 ? one : many}`
}

// projectLines describe the open project: its branch, uncommitted changes,
// and walkthroughs, each opening where it is reviewed.
export function projectLines(facts: ProjectFacts): ProjectLine[] {
  const lines: ProjectLine[] = []
  if (!facts.repository) lines.push({ key: "folder", text: "Folder, not a git repository" })
  else {
    if (facts.branch) lines.push({ key: "branch", text: `On ${facts.branch}` })
    if (facts.changes !== null) {
      lines.push(facts.changes > 0
        ? { key: "changes", text: count(facts.changes, "uncommitted change", "uncommitted changes"), action: "review-uncommitted" }
        : { key: "changes", text: "No uncommitted changes" })
    }
  }
  if (facts.unseen > 0) lines.push({ key: "walkthroughs", text: count(facts.unseen, "new walkthrough", "new walkthroughs"), action: "walkthroughs" })
  else if (facts.walkthroughs) lines.push({ key: "walkthroughs", text: count(facts.walkthroughs, "walkthrough", "walkthroughs"), action: "walkthroughs" })
  return lines
}

// listsProjects: the project list shows only when there is a choice.
export function listsProjects(response: ProjectsResponse): boolean {
  return response.projects.filter((project) => project.available).length > 1
}

export interface WelcomeSection {
  title: string
  rows: WelcomeProjectRow[]
}

export interface WelcomeProjectRow {
  row: ProjectRow
  // worktrees lists a repository's members when it has several, else none.
  worktrees: ProjectRow[]
}

// welcomeSections are the picker's sections without its Browse actions: a
// query flattens them into ranked matches; otherwise each repository with
// several worktrees carries them for inline expansion.
export function welcomeSections(response: ProjectsResponse, remembered: readonly string[], current: string, query: string): WelcomeSection[] {
  const filtering = query.trim() !== ""
  return projectSections(response, remembered, current, query).flatMap((section): WelcomeSection[] => {
    const rows = section.rows.flatMap((row): WelcomeProjectRow[] => {
      if (row.kind !== "project") return []
      const members = !filtering && row.worktrees > 1
        ? projectSections(response, remembered, current, "", row.group).flatMap((group) => group.rows)
          .flatMap((member) => member.kind === "project" ? [member] : [])
        : []
      return [{ row, worktrees: members }]
    })
    return rows.length ? [{ title: section.title, rows }] : []
  })
}
