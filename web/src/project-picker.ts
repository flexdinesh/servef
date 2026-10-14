import type { Directory, ProjectKind, ProjectsResponse } from "./projects-contract.ts"

// Pure list logic for the project picker; ProjectPicker.tsx only renders it.

export interface ProjectRow {
  kind: "project"
  key: string
  id: string
  name: string
  path: string
  // opened marks a folder opened by hand that discovery does not list.
  projectKind: ProjectKind | "opened"
  nested: boolean
  current: boolean
  disabled: boolean
  note: string
}

export interface BrowseActionRow {
  kind: "browse"
  key: string
}

export interface OpenFolderRow {
  kind: "open"
  key: string
  path: string
}

export interface DirectoryRow {
  kind: "directory"
  key: string
  name: string
  path: string
  isProject: boolean
}

export type PickerRow = ProjectRow | BrowseActionRow | OpenFolderRow | DirectoryRow

export interface PickerSection {
  title: string
  rows: PickerRow[]
}

const rootID = "."

// matchScore ranks a query against a name and path; lower is better and null
// is no match. Name prefixes beat name substrings, then path substrings, then
// in-order characters of the path.
export function matchScore(query: string, name: string, path: string): number | null {
  const needle = query.trim().toLowerCase()
  if (!needle) return 0
  const lowerName = name.toLowerCase()
  const lowerPath = path.toLowerCase()
  if (lowerName.startsWith(needle)) return 0
  if (lowerName.includes(needle)) return 1
  if (lowerPath.includes(needle)) return 2
  let index = 0
  for (const character of lowerPath) {
    if (character === needle[index]) index++
    if (index === needle.length) return 3
  }
  return null
}

function baseName(path: string): string {
  return path.split("/").filter(Boolean).at(-1) ?? path
}

function byScore<T extends { score: number; row: ProjectRow }>(entries: T[]): ProjectRow[] {
  return entries
    .sort((left, right) => left.score - right.score || left.row.path.localeCompare(right.row.path))
    .map((entry) => entry.row)
}

// projectSections lists discovered projects, worktrees nested under their
// group's main worktree, then folders opened by hand. A query flattens the
// list into ranked matches.
export function projectSections(response: ProjectsResponse, remembered: readonly string[], current: string, query: string): PickerSection[] {
  const discovered = response.projects.map((project): ProjectRow => ({
    kind: "project",
    key: `project:${project.id || project.path}`,
    id: project.id,
    name: project.name,
    path: project.path,
    projectKind: project.kind,
    nested: false,
    current: project.available && project.id === current,
    disabled: !project.available,
    note: project.available ? "" : `Outside ${response.rootName}`,
  }))
  const listed = new Set(response.projects.filter((project) => project.available).map((project) => project.id))
  const opened = remembered
    .filter((id) => id !== rootID && !listed.has(id))
    .map((id): ProjectRow => ({
      kind: "project",
      key: `opened:${id}`,
      id,
      name: baseName(id),
      path: id,
      projectKind: "opened",
      nested: false,
      current: id === current,
      disabled: false,
      note: "",
    }))
  const browse: PickerSection = { title: "", rows: [{ kind: "browse", key: "browse" }] }

  if (query.trim()) {
    const ranked = (rows: ProjectRow[]) => byScore(rows.flatMap((row) => {
      const score = matchScore(query, row.name, row.path)
      return score === null ? [] : [{ score, row }]
    }))
    return [
      { title: "Projects", rows: ranked(discovered) },
      { title: "Opened folders", rows: ranked(opened) },
      browse,
    ].filter((section) => section.rows.length > 0)
  }

  const groups = new Map<string, ProjectRow[]>()
  response.projects.forEach((project, index) => {
    const row = discovered[index]
    if (!project.group || !row) return
    groups.set(project.group, [...groups.get(project.group) ?? [], row])
  })
  const rows: ProjectRow[] = []
  const emitted = new Set<string>()
  response.projects.forEach((project, index) => {
    const row = discovered[index]
    if (!row) return
    if (!project.group) {
      rows.push(row)
      return
    }
    if (emitted.has(project.group)) return
    emitted.add(project.group)
    const members = groups.get(project.group) ?? []
    const leader = members.find((member) => member.path === project.group)
    if (leader) rows.push(leader)
    for (const member of members) {
      if (member !== leader) rows.push({ ...member, nested: leader !== undefined })
    }
  })
  return [
    { title: "Projects", rows },
    { title: "Opened folders", rows: opened },
    browse,
  ].filter((section) => section.rows.length > 0)
}

// splitBrowseInput reads a root-relative folder path being typed: the folder
// whose children are listed, and the partial child name after the last slash.
export function splitBrowseInput(input: string): { folder: string; partial: string } {
  const text = input.replace(/^(\.?\/)+/, "")
  const slash = text.lastIndexOf("/")
  if (slash < 0) return { folder: rootID, partial: text }
  return { folder: text.slice(0, slash) || rootID, partial: text.slice(slash + 1) }
}

// parentInput is the browse input one folder up from a folder input ending
// in a slash.
export function parentInput(input: string): string {
  const segments = input.replace(/^(\.?\/)+/, "").split("/").filter(Boolean)
  segments.pop()
  return segments.length ? `${segments.join("/")}/` : ""
}

// browseRows offers the listed folder itself, then its children matching the
// partial name.
export function browseRows(folder: string, partial: string, directories: readonly Directory[]): PickerSection[] {
  const children = directories
    .flatMap((directory) => {
      const score = matchScore(partial, directory.name, directory.name)
      return score === null ? [] : [{ score, directory }]
    })
    .sort((left, right) => left.score - right.score)
    .map(({ directory }): DirectoryRow => ({
      kind: "directory",
      key: `directory:${directory.path}`,
      name: directory.name,
      path: directory.path,
      isProject: directory.isProject,
    }))
  const open: OpenFolderRow = { kind: "open", key: `open:${folder}`, path: folder }
  const sections: PickerSection[] = [
    { title: "", rows: [open] },
    { title: "Folders", rows: children },
  ]
  return sections.filter((section) => section.rows.length > 0)
}

export function selectableRows(sections: readonly PickerSection[]): PickerRow[] {
  return sections.flatMap((section) => section.rows)
    .filter((row) => row.kind !== "project" || !row.disabled)
}
