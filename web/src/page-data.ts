import {
  isPageData,
  type PageData,
  type PageLoadResult,
  type TreeNode,
} from "./page-contract.ts"

export { isPageData, isPageLoadResult } from "./page-contract.ts"
export type { PageData, PageLoadResult, TreeNode } from "./page-contract.ts"

export const emptyPage: PageData = {
  content: "",
  empty: false,
  error: "",
  fileCount: 0,
  fileSize: 0,
  hasFile: false,
  rootName: "",
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
  selected: string
  signal: AbortSignal
}

export async function loadPage({ pathname, selected, signal }: LoadPageOptions): Promise<PageLoadResult> {
  const embedded = initialPageData
  initialPageData = null
  if (embedded && embedded.selected === selected && (pathname === "/" || pathname === "/view")) {
    return { kind: "page", page: embedded }
  }

  const endpoint = pathname === "/view"
    ? `/api/page?path=${encodeURIComponent(selected)}`
    : "/api/page"

  try {
    const response = await fetch(endpoint, { headers: { Accept: "application/json" }, signal })
    const payload: unknown = await response.json()
    return isPageData(payload)
      ? { kind: "page", page: payload }
      : { kind: "failure", message: "The server returned an invalid page response." }
  } catch (error: unknown) {
    if (error instanceof Error && error.name === "AbortError") throw error
    return { kind: "failure", message: "Could not load Markdown files." }
  }
}

export function openDirectoryPaths(nodes: readonly TreeNode[]): string[] {
  return nodes.flatMap((node) => [
    ...(node.isDir && node.open ? [node.path] : []),
    ...openDirectoryPaths(node.children),
  ])
}
