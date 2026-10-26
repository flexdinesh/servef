import {
  isPageData,
  type PageData,
  type PageLoadResult,
  type TreeNode,
} from "./page-contract.ts"

import { fetchJSON, RequestError, type RequestFailure } from "./http.ts"
import { projectUrl } from "./project-url.ts"

export { isPageData, isPageLoadResult } from "./page-contract.ts"
export type { PageData, PageLoadResult, TreeNode } from "./page-contract.ts"

export const emptyPage: PageData = {
  roots: [],
  project: "",
  projectUnavailable: false,
  filenamePatterns: [],
  documentType: null,
  navigationSupported: false,
  binary: false,
  tooLarge: false,
  previewDisabled: false,
  searchExcluded: false,
  highlightDisabled: false,
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
  at: "",
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
  // at reads selected from that commit instead of the working tree.
  at?: string
  signal: AbortSignal
}

export async function loadPage({ pathname, project, selected, at = "", signal }: LoadPageOptions): Promise<PageLoadResult> {
  const embedded = initialPageData
  initialPageData = null
  if (embedded && embedded.project === project && embedded.selected === selected && embedded.at === at && (pathname === "/" || pathname === "/view")) {
    return { kind: "page", page: embedded }
  }

  let endpoint = projectUrl("/api/page", project, pathname === "/view" ? selected : undefined)
  if (pathname === "/view" && at) endpoint += `&at=${at}`

  try {
    // Pages carry their own error, so any status with a page is a page.
    return { kind: "page", page: await fetchJSON(endpoint, { signal, anyStatus: true }, isPageData, pageError) }
  } catch (error: unknown) {
    if (error instanceof RequestError) return { kind: "failure", message: error.message }
    throw error
  }
}

function pageError(failure: RequestFailure): string {
  return failure.kind === "unreachable" || failure.kind === "malformed"
    ? "Could not load files."
    : "The server returned an invalid page response."
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
