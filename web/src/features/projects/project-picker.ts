import type { Directory, Project, ProjectKind, ProjectsResponse } from "../../api/projects-contract.ts"

// Pure list logic for the project picker; ProjectPicker.tsx only renders it.

export interface ProjectRow {
  kind: "project"
  key: string
  // id is the project opened by the row: for a repository with worktrees, its
  // last-used worktree.
  id: string
  name: string
  path: string
  // location is the path within its served root ("" for a root), or the
  // absolute path outside every root; it is shown and matched.
  location: string
  branch: string
  // opened marks a folder opened by hand that discovery does not list.
  projectKind: ProjectKind | "opened"
  // group is the repository the row stands for or belongs to.
  group: string
  // worktrees counts a repository's members when the row stands for all of
  // them; zero otherwise.
  worktrees: number
  // repository names a worktree's repository in worktree lists and flat
  // search results.
  repository: string
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

const unavailableNote = "Outside served folders"

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

function bestScore(query: string, names: readonly string[], path: string): number | null {
  const scores = names.flatMap((name) => {
    const score = matchScore(query, name, path)
    return score === null ? [] : [score]
  })
  return scores.length ? Math.min(...scores) : null
}

function baseName(path: string): string {
  return path.split(/[/\\]/).filter(Boolean).at(-1) ?? path
}

function separatorOf(root: string): string {
  return root.includes("\\") && !root.includes("/") ? "\\" : "/"
}

// within reports whether path is root or below it.
export function within(path: string, root: string): boolean {
  if (path === root) return true
  const separator = separatorOf(root)
  return path.startsWith(root.endsWith(separator) ? root : root + separator)
}

// locate is a path relative to the served root containing it.
function locate(path: string, roots: readonly string[]): string {
  const root = roots.find((candidate) => within(path, candidate))
  if (root === undefined) return path
  return path.slice(root.length).replace(/^[/\\]+/, "")
}

function byScore(entries: { score: number; row: ProjectRow }[]): ProjectRow[] {
  return entries
    .sort((left, right) => left.score - right.score || left.row.location.localeCompare(right.row.location))
    .map((entry) => entry.row)
}

interface Group {
  path: string
  // name is the repository's name, shared by its members.
  name: string
  // members lists the main worktree first, then the rest in discovery order.
  members: Project[]
  root: string
}

function groups(response: ProjectsResponse): Map<string, Group> {
  const result = new Map<string, Group>()
  for (const project of response.projects) {
    if (!project.group) continue
    const group = result.get(project.group) ?? { path: project.group, name: "", members: [], root: "" }
    if (project.path === project.group) group.members.unshift(project)
    else group.members.push(project)
    result.set(project.group, group)
  }
  for (const group of result.values()) {
    const leader = group.members.find((member) => member.path === group.path)
    group.root = leader?.root || group.members.find((member) => member.available)?.root || group.members[0]?.root || ""
    group.name = leader?.repository || group.members.find((member) => member.repository)?.repository || baseName(group.path)
  }
  return result
}

// groupTarget is the member a repository row opens: the most recently
// remembered available member, else the main worktree, else the first.
function groupTarget(group: Group, remembered: readonly string[]): Project | undefined {
  const available = group.members.filter((member) => member.available)
  for (const id of remembered) {
    const member = available.find((candidate) => candidate.id === id)
    if (member) return member
  }
  return available.find((member) => member.path === group.path) ?? available[0]
}

// projectRow lists one project; a worktree row names its repository and
// shows its branch.
function projectRow(project: Project, roots: readonly string[], current: string, repository = ""): ProjectRow {
  return {
    kind: "project",
    key: `project:${project.path}`,
    id: project.id,
    name: repository ? project.branch || project.name : project.name,
    path: project.path,
    location: locate(project.path, roots),
    branch: project.branch,
    projectKind: project.kind,
    group: project.group,
    worktrees: 0,
    repository,
    current: project.available && project.id === current,
    disabled: !project.available,
    note: project.available ? "" : unavailableNote,
  }
}

function groupRow(group: Group, roots: readonly string[], remembered: readonly string[], current: string): ProjectRow {
  const target = groupTarget(group, remembered)
  return {
    kind: "project",
    key: `group:${group.path}`,
    id: target?.id ?? "",
    name: group.name,
    path: group.path,
    location: locate(group.path, roots),
    branch: target?.branch ?? "",
    projectKind: "repo",
    group: group.path,
    worktrees: group.members.length,
    repository: "",
    current: group.members.some((member) => member.available && member.id === current),
    disabled: target === undefined,
    note: target ? "" : unavailableNote,
  }
}

// currentGroup is the repository group of the current project, if any.
export function currentGroup(response: ProjectsResponse, current: string): string {
  return response.projects.find((project) => project.available && project.id === current)?.group ?? ""
}

// groupName names a repository group by its repository name.
export function groupName(response: ProjectsResponse, group: string): string {
  return groups(response).get(group)?.name ?? baseName(group)
}

// projectSections lists each served root's folder and repositories, a
// repository with worktrees as one row, then folders opened by hand. A query
// flattens the list into ranked matches that include each worktree. Within a
// group, the list is the group's worktrees.
export function projectSections(response: ProjectsResponse, remembered: readonly string[], current: string, query: string, group = ""): PickerSection[] {
  const repositories = groups(response)
  const roots = response.roots.map((root) => root.path)
  if (group) {
    const members = repositories.get(group)?.members ?? []
    const name = repositories.get(group)?.name ?? baseName(group)
    const matches = members.flatMap((member) => {
      // The list title names the repository, so rows show only branches.
      const row = { ...projectRow(member, roots, current, name), repository: "" }
      const score = bestScore(query, [row.name, member.name], row.location)
      return score === null ? [] : [{ score, row }]
    })
    const rows = query.trim() ? byScore(matches) : matches.map((match) => match.row)
    return rows.length ? [{ title: `${name} worktrees`, rows }] : []
  }

  const listed = new Set(response.projects.filter((project) => project.available).map((project) => project.id))
  const opened = remembered
    .filter((id) => !listed.has(id) && roots.some((root) => within(id, root)))
    .map((id): ProjectRow => ({
      kind: "project",
      key: `opened:${id}`,
      id,
      name: baseName(id),
      path: id,
      location: locate(id, roots),
      branch: "",
      projectKind: "opened",
      group: "",
      worktrees: 0,
      repository: "",
      current: id === current,
      disabled: false,
      note: "",
    }))
  const browse: PickerSection = { title: "", rows: [{ kind: "browse", key: "browse" }] }

  if (query.trim()) {
    const matches = response.projects.flatMap((project) => {
      const repository = project.group ? repositories.get(project.group)?.name ?? "" : ""
      const row = projectRow(project, roots, current, repository)
      const score = bestScore(query, repository ? [row.name, project.name, repository] : [row.name], row.location)
      return score === null ? [] : [{ score, row }]
    })
    const openedMatches = opened.flatMap((row) => {
      const score = matchScore(query, row.name, row.location)
      return score === null ? [] : [{ score, row }]
    })
    return [
      { title: "Projects", rows: byScore(matches) },
      { title: "Opened folders", rows: byScore(openedMatches) },
      browse,
    ].filter((section) => section.rows.length > 0)
  }

  const sections = response.roots.map((root): PickerSection => ({ title: roots.length > 1 ? root.name : "Projects", rows: [] }))
  const emitted = new Set<string>()
  for (const project of response.projects) {
    const repository = project.group ? repositories.get(project.group) : undefined
    if (repository && emitted.has(repository.path)) continue
    const section = sections[Math.max(0, roots.indexOf(repository ? repository.root : project.root))]
    if (!section) continue
    if (repository) {
      emitted.add(repository.path)
      section.rows.push(groupRow(repository, roots, remembered, current))
    } else {
      section.rows.push(projectRow(project, roots, current))
    }
  }
  return [
    ...sections,
    { title: "Opened folders", rows: opened },
    browse,
  ].filter((section) => section.rows.length > 0)
}

function withSlash(path: string): string {
  return path.endsWith("/") ? path : `${path}/`
}

// browseStart is the browse input on entering Browse: a lone root's
// children, else the list of roots.
export function browseStart(roots: readonly string[]): string {
  const [root] = roots
  return roots.length === 1 && root ? withSlash(root) : ""
}

// splitBrowseInput reads an absolute folder path being typed: the folder whose
// children are listed, and the partial child name after the last slash. Input
// outside every root filters the roots themselves (folder "").
export function splitBrowseInput(input: string, roots: readonly string[]): { folder: string; partial: string } {
  const slash = input.lastIndexOf("/")
  const folder = slash === 0 ? "/" : input.slice(0, slash)
  if (slash < 0 || !roots.some((root) => within(folder, root))) return { folder: "", partial: input }
  return { folder, partial: input.slice(slash + 1) }
}

// parentInput is the browse input one folder up from a folder input ending
// in a slash; above a root it is the list of roots.
export function parentInput(input: string, roots: readonly string[]): string {
  const folder = input.replace(/\/+$/, "")
  if (!folder || roots.includes(folder)) return ""
  const parent = folder.slice(0, folder.lastIndexOf("/"))
  return parent && roots.some((root) => within(parent, root)) ? withSlash(parent) : ""
}

// browseRows offers the listed folder itself, then its children matching the
// partial name; folder "" lists the served roots. Repositories are listed to
// open whole, not to browse.
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
  const sections: PickerSection[] = [
    { title: "", rows: folder ? [{ kind: "open", key: `open:${folder}`, path: folder }] : [] },
    { title: folder ? "Folders" : "Served folders", rows: children },
  ]
  return sections.filter((section) => section.rows.length > 0)
}

export function selectableRows(sections: readonly PickerSection[]): PickerRow[] {
  return sections.flatMap((section) => section.rows)
    .filter((row) => row.kind !== "project" || !row.disabled)
}

// canDrill reports whether a row opens a repository's worktree list.
export function canDrill(row: PickerRow | undefined): row is ProjectRow {
  return row?.kind === "project" && row.worktrees > 1
}

// browseTarget is the folder Tab completes into: the active row when it is a
// browsable folder, else the first one. Repositories are not browsed.
export function browseTarget(rows: readonly PickerRow[], active: PickerRow | undefined): DirectoryRow | undefined {
  if (active?.kind === "directory" && !active.isProject) return active
  return rows.find((row): row is DirectoryRow => row.kind === "directory" && !row.isProject)
}
