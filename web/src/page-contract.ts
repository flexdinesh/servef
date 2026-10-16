import { isProjectKind, type ProjectKind } from "./projects-contract.ts"

export type DocumentMode = "preview" | "source" | "format"
export type DefaultDocumentMode = "preview" | "source"

export interface FileType {
  id: string
  label: string
  extensions: string[]
  filenames: string[]
  language: string
  previewKind: "markdown" | "image" | ""
  previewLabel: string
  defaultMode: DefaultDocumentMode
  formattable: boolean
}

export interface TreeNode {
  fileType: string
  children: TreeNode[]
  isDir: boolean
  name: string
  open: boolean
  path: string
  selected: boolean
}

export interface PageData {
  // roots are the served roots; browser memory is keyed by them.
  roots: string[]
  project: string
  projectUnavailable: boolean
  filenamePatterns: string[]
  documentType: FileType | null
  navigationSupported: boolean
  binary: boolean
  tooLarge: boolean
  previewDisabled: boolean
  searchExcluded: boolean
  sourceLines: string[]
  previewError: string
  content: string
  empty: boolean
  error: string
  fileCount: number
  fileSize: number
  hasFile: boolean
  rootName: string
  // repository names the project's repository (origin remote, else main
  // worktree folder), empty for a folder.
  repository: string
  // projectKind tells a linked worktree from a repository or folder.
  projectKind: ProjectKind
  // branch is the project's checked-out branch, empty for a folder.
  branch: string
  // worktrees counts the repository's checkouts; above 1 it has worktrees.
  worktrees: number
  selected: string
  source: string
  tree: TreeNode[]
  warnings: string[]
}

export type PageLoadResult =
  | { kind: "page"; page: PageData }
  | { kind: "failure"; message: string }

export interface FormatResult {
  formattedLines: string[]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

export function isFileType(value: unknown): value is FileType {
  return isRecord(value)
    && typeof value.id === "string"
    && typeof value.label === "string"
    && isStringArray(value.extensions)
    && isStringArray(value.filenames)
    && typeof value.language === "string"
    && (value.previewKind === "markdown" || value.previewKind === "image" || value.previewKind === "")
    && typeof value.previewLabel === "string"
    && (value.defaultMode === "preview" || value.defaultMode === "source")
    && (value.previewKind !== "" || value.defaultMode === "source")
    && (value.previewKind === "" || value.previewLabel !== "")
    && typeof value.formattable === "boolean"
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((entry: unknown) => typeof entry === "string")
}

export function isFormatResult(value: unknown): value is FormatResult {
  return isRecord(value) && isStringArray(value.formattedLines)
}

function isTreeNode(value: unknown): value is TreeNode {
  return isRecord(value)
    && typeof value.fileType === "string"
    && Array.isArray(value.children)
    && value.children.every(isTreeNode)
    && typeof value.isDir === "boolean"
    && typeof value.name === "string"
    && typeof value.open === "boolean"
    && typeof value.path === "string"
    && typeof value.selected === "boolean"
}

export function isPageData(value: unknown): value is PageData {
  return isRecord(value)
    && isStringArray(value.roots)
    && typeof value.project === "string"
    && typeof value.projectUnavailable === "boolean"
    && isStringArray(value.filenamePatterns)
    && typeof value.navigationSupported === "boolean"
    && typeof value.binary === "boolean"
    && typeof value.tooLarge === "boolean"
    && typeof value.previewDisabled === "boolean"
    && typeof value.searchExcluded === "boolean"
    && (value.documentType === null || isFileType(value.documentType))
    && Array.isArray(value.sourceLines)
    && value.sourceLines.every((line: unknown) => typeof line === "string")
    && typeof value.previewError === "string"
    && (!value.hasFile || isFileType(value.documentType))
    && typeof value.content === "string"
    && typeof value.empty === "boolean"
    && typeof value.error === "string"
    && typeof value.fileCount === "number"
    && typeof value.fileSize === "number"
    && typeof value.hasFile === "boolean"
    && typeof value.rootName === "string"
    && typeof value.repository === "string"
    && isProjectKind(value.projectKind)
    && typeof value.branch === "string"
    && typeof value.worktrees === "number"
    && typeof value.selected === "string"
    && typeof value.source === "string"
    && Array.isArray(value.tree)
    && value.tree.every(isTreeNode)
    && Array.isArray(value.warnings)
    && value.warnings.every((warning) => typeof warning === "string")
}

export function isPageLoadResult(value: unknown): value is PageLoadResult {
  if (!isRecord(value) || typeof value.kind !== "string") return false
  if (value.kind === "page") return isPageData(value.page)
  return value.kind === "failure" && typeof value.message === "string"
}
