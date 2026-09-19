export interface TreeNode {
  children: TreeNode[]
  isDir: boolean
  name: string
  open: boolean
  path: string
  selected: boolean
}

export interface PageData {
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

function isTreeNode(value: unknown): value is TreeNode {
  return isRecord(value)
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
