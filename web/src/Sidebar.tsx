import { FilesIcon, GitBranchIcon, SearchIcon } from "lucide-react"
import type { ReactNode } from "react"

import type { SidebarState, SidebarView } from "./use-sidebar.ts"

const mac = typeof navigator !== "undefined" && navigator.platform.includes("Mac")

const activities: ReadonlyArray<{ view: SidebarView; label: string; shortcut: string; keys: string; icon: ReactNode }> = [
  { view: "explorer", label: "Explorer", shortcut: mac ? "⇧⌘E" : "Ctrl+Shift+E", keys: "Control+Shift+E Meta+Shift+E", icon: <FilesIcon aria-hidden="true" /> },
  { view: "search", label: "Search", shortcut: mac ? "⇧⌘F" : "Ctrl+Shift+F", keys: "Control+Shift+F Meta+Shift+F", icon: <SearchIcon aria-hidden="true" /> },
  { view: "source-control", label: "Source Control", shortcut: mac ? "⌃⇧G" : "Ctrl+Shift+G", keys: "Control+Shift+G Meta+Shift+G", icon: <GitBranchIcon aria-hidden="true" /> },
]

const viewNames: Record<SidebarView, string> = { explorer: "Files", search: "Search", "source-control": "Source Control" }

// badgeLabel caps the activity badge as VS Code does.
function badgeLabel(count: number): string {
  return count > 99 ? "99+" : String(count)
}

interface SidebarProps {
  explorer: ReactNode
  search: ReactNode
  sourceControl: ReactNode
  // pendingChanges counts Source Control's changes; 0 shows no badge.
  pendingChanges: number
  sidebar: SidebarState
}

// Sidebar groups the VS Code activity bar and its views into one desktop
// island; mobile opens it as a modal drawer. All views
// stay mounted so switching keeps tree, search, and change-list state.
export function Sidebar({ explorer, pendingChanges, search, sidebar, sourceControl }: SidebarProps) {
  const { drawer, hidden, mobile, view } = sidebar
  // Only the mobile drawer is a dialog; inline, the element is presentational.
  return (
    <dialog ref={drawer} className="sidebar-drawer explorer-drawer" role={mobile ? undefined : "none"}
      aria-label={mobile ? viewNames[view] : undefined}
      onClick={(event) => { if (event.target === drawer.current) sidebar.dismiss() }}>
      <nav className="activity-bar" aria-label="Views">
        {activities.map((activity) => {
          const badge = activity.view === "source-control" && pendingChanges > 0 ? badgeLabel(pendingChanges) : ""
          const label = badge ? `${activity.label}, ${pendingChanges.toLocaleString()} pending ${pendingChanges === 1 ? "change" : "changes"}` : activity.label
          return (
            <button key={activity.view} type="button" className="activity-item" data-view={activity.view}
              aria-label={label} aria-pressed={!hidden && view === activity.view}
              aria-keyshortcuts={activity.keys} title={`${label} (${activity.shortcut})`}
              onClick={() => sidebar.select(activity.view)}>
              {activity.icon}
              {badge && <span className="activity-badge" aria-hidden="true">{badge}</span>}
            </button>
          )
        })}
      </nav>
      <div className="sidebar" hidden={hidden}>
        <div className="sidebar-view" hidden={view !== "explorer"}>{explorer}</div>
        <div className="sidebar-view" hidden={view !== "search"}>{search}</div>
        <div className="sidebar-view" hidden={view !== "source-control"}>{sourceControl}</div>
      </div>
    </dialog>
  )
}
