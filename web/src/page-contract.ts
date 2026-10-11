export type DocumentMode = "preview" | "source"

export interface FileType {
  id: string
  label: string
  extensions: string[]
  language: string
  previewKind: "markdown" | "code" | ""
  previewLabel: string
  defaultMode: DocumentMode
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
  fileTypes: FileType[]
  documentType: FileType | null
  sourceLines: string[]
  previewError: string
  content: string
  empty: boolean
  error: string
  fileCount: number
  fileSize: number
  hasFile: boolean
  rootName: string
  selected: string
  source: string
  tree: TreeNode[]
  warnings: string[]
}

export type PageLoadResult =
  | { kind: "page"; page: PageData }
  | { kind: "failure"; message: string }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

export function isFileType(value: unknown): value is FileType {
  return isRecord(value)
    && typeof value.id === "string"
    && typeof value.label === "string"
    && Array.isArray(value.extensions)
    && value.extensions.every((extension: unknown) => typeof extension === "string")
    && typeof value.language === "string"
    && (value.previewKind === "markdown" || value.previewKind === "code" || value.previewKind === "")
    && typeof value.previewLabel === "string"
    && (value.defaultMode === "preview" || value.defaultMode === "source")
    && (value.previewKind !== "" || value.defaultMode === "source")
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
    && Array.isArray(value.fileTypes)
    && value.fileTypes.every(isFileType)
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
