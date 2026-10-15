import {
  isPageData,
  type PageData,
  type PageLoadResult,
  type TreeNode,
} from "./page-contract.ts"

import { projectUrl } from "./project-url.ts"

export { isPageData, isPageLoadResult } from "./page-contract.ts"
export type { PageData, PageLoadResult, TreeNode } from "./page-contract.ts"

export const emptyPage: PageData = {
  roots: [],
  project: "",
  projectUnavailable: false,
  filenamePatterns: [],
  documentType: null,
  binary: false,
  tooLarge: false,
  previewDisabled: false,
  searchExcluded: false,
  sourceLines: [],
  previewError: "",
  content: "",
  empty: false,
  error: "",
  fileCount: 0,
  fileSize: 0,
  hasFile: false,
  rootName: "",
  repository: "",
  projectKind: "folder",
  branch: "",
  worktrees: 0,
  selected: "",
  source: "",
  tree: [],
  warnings: [],
}

function readInitialPageData(): PageData | null {
  const source = document.querySelector<HTMLTemplateElement>("#app-data")?.content.textContent.trim()
  if (!source?.startsWith("{")) return null
  try {
    const data: unknown = JSON.parse(source)
    return isPageData(data) ? data : null
  } catch {
    return null
  }
}

let initialPageData = readInitialPageData()

interface LoadPageOptions {
  pathname: string
  project: string
  selected: string
  signal: AbortSignal
}

export async function loadPage({ pathname, project, selected, signal }: LoadPageOptions): Promise<PageLoadResult> {
  const embedded = initialPageData
  initialPageData = null
  if (embedded && embedded.project === project && embedded.selected === selected && (pathname === "/" || pathname === "/view")) {
    return { kind: "page", page: embedded }
  }

  const endpoint = projectUrl("/api/page", project, pathname === "/view" ? selected : undefined)

  try {
    const response = await fetch(endpoint, { headers: { Accept: "application/json" }, signal })
    const payload: unknown = await response.json()
    return isPageData(payload)
      ? { kind: "page", page: payload }
      : { kind: "failure", message: "The server returned an invalid page response." }
  } catch (error: unknown) {
    if (error instanceof Error && error.name === "AbortError") throw error
    return { kind: "failure", message: "Could not load files." }
  }
}

export function filePaths(nodes: readonly TreeNode[]): string[] {
  return nodes.flatMap((node) => node.isDir ? filePaths(node.children) : [node.path])
}

export function openDirectoryPaths(nodes: readonly TreeNode[]): string[] {
  return nodes.flatMap((node) => [
    ...(node.isDir && node.open ? [node.path] : []),
    ...openDirectoryPaths(node.children),
  ])
}
