import { useCallback, useEffect, useState } from "react"

import { openDirectoryPaths, type TreeNode } from "./page-data.ts"

export function useFileTreeExpansion(tree: readonly TreeNode[]) {
  const [expandedPaths, setExpandedPaths] = useState<ReadonlySet<string>>(
    () => new Set(openDirectoryPaths(tree)),
  )

  useEffect(() => {
    const paths = openDirectoryPaths(tree)
    if (paths.length === 0) return
    setExpandedPaths((current) => {
      const next = new Set(current)
      for (const path of paths) next.add(path)
      return next.size === current.size ? current : next
    })
  }, [tree])

  const updateExpanded = useCallback((path: string, expanded: boolean) => {
    setExpandedPaths((current) => {
      if (current.has(path) === expanded) return current
      const next = new Set(current)
      if (expanded) next.add(path)
      else next.delete(path)
      return next
    })
  }, [])

  return { expandedPaths, updateExpanded }
}
