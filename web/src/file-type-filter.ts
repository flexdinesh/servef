import type { TreeNode } from "./page-contract.ts"

// Tree visibility is independent of the server's allowed file scope. Preserve
// original node metadata and prune directories without visible descendants.
export function filterFileTree(nodes: readonly TreeNode[], fileTypes: readonly string[]): TreeNode[] {
  if (fileTypes.length === 0) return [...nodes]
  return nodes.flatMap((node) => {
    if (!node.isDir) return fileTypes.includes(node.fileType) ? [node] : []
    const children = filterFileTree(node.children, fileTypes)
    return children.length ? [{ ...node, children }] : []
  })
}

export function fileTypeCounts(nodes: readonly TreeNode[]): ReadonlyMap<string, number> {
  const counts = new Map<string, number>()
  function visit(entries: readonly TreeNode[]) {
    for (const node of entries) {
      if (node.isDir) visit(node.children)
      else counts.set(node.fileType, (counts.get(node.fileType) ?? 0) + 1)
    }
  }
  visit(nodes)
  return counts
}
