import { FilesIcon, SearchIcon } from "lucide-react"
import type { ReactNode } from "react"

import type { SidebarState, SidebarView } from "./use-sidebar.ts"

const mac = typeof navigator !== "undefined" && navigator.platform.includes("Mac")

const activities: ReadonlyArray<{ view: SidebarView; label: string; shortcut: string; keys: string; icon: ReactNode }> = [
  { view: "explorer", label: "Explorer", shortcut: mac ? "⇧⌘E" : "Ctrl+Shift+E", keys: "Control+Shift+E Meta+Shift+E", icon: <FilesIcon aria-hidden="true" /> },
  { view: "search", label: "Search", shortcut: mac ? "⇧⌘F" : "Ctrl+Shift+F", keys: "Control+Shift+F Meta+Shift+F", icon: <SearchIcon aria-hidden="true" /> },
]

interface SidebarProps {
  explorer: ReactNode
  search: ReactNode
  sidebar: SidebarState
}

// Sidebar is the VS Code activity bar plus its views. Desktop lays the dialog
// out inline (display: contents); mobile opens it as a modal drawer. Both views
// stay mounted so switching keeps tree and search state.
export function Sidebar({ explorer, search, sidebar }: SidebarProps) {
  const { drawer, hidden, mobile, view } = sidebar
  // Only the mobile drawer is a dialog; inline, the element is presentational.
  return (
    <dialog ref={drawer} className="sidebar-drawer explorer-drawer" role={mobile ? undefined : "none"}
      aria-label={mobile ? view === "search" ? "Search" : "Files" : undefined}
      onClick={(event) => { if (event.target === drawer.current) sidebar.dismiss() }}>
      <nav className="activity-bar" aria-label="Views">
        {activities.map((activity) => (
          <button key={activity.view} type="button" className="activity-item"
            aria-label={activity.label} aria-pressed={!hidden && view === activity.view}
            aria-keyshortcuts={activity.keys} title={`${activity.label} (${activity.shortcut})`}
            onClick={() => sidebar.select(activity.view)}>
            {activity.icon}
          </button>
        ))}
      </nav>
      <div className="sidebar" hidden={hidden}>
        <div className="sidebar-view" hidden={view !== "explorer"}>{explorer}</div>
        <div className="sidebar-view" hidden={view !== "search"}>{search}</div>
      </div>
    </dialog>
  )
}
