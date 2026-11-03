import type { NativeMenuItem } from "../../lib/desktop-bridge.ts"

// The Explorer row menu, shown only in the desktop app as a native menu
// (decision 0081); browsers keep no row menu. Pure: FileTree runs the action
// whose id comes back.

export type TreeMenuAction = "reveal" | "copy-path" | "copy-relative-path"

const ACTIONS: readonly TreeMenuAction[] = ["reveal", "copy-path", "copy-relative-path"]

export function isTreeMenuAction(id: string | null): id is TreeMenuAction {
  return ACTIONS.some((action) => action === id)
}

export function treeMenuItems(): NativeMenuItem[] {
  return [
    { id: "reveal", label: "Reveal in File Manager" },
    { separator: true },
    { id: "copy-path", label: "Copy Path" },
    { id: "copy-relative-path", label: "Copy Relative Path" },
  ]
}

// treeRowPath is a tree row's project-relative path: directories drop the
// tree's trailing "/".
export function treeRowPath(path: string): string {
  return path.endsWith("/") ? path.slice(0, -1) : path
}

// absolutePath joins a project (an absolute path) and a relative path.
export function absolutePath(project: string, path: string): string {
  return `${project.endsWith("/") ? project.slice(0, -1) : project}/${path}`
}
