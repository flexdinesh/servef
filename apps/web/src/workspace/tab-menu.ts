import type { NativeMenuItem } from "../lib/desktop-bridge.ts"
import type { WorkspaceTab } from "./workspace-tabs.ts"

// The tab context menu as native menu items for the desktop app (decision
// 0081). Pure: TabBar runs the action whose id comes back.

export type TabMenuAction = "close" | "close-others" | "close-right" | "close-all" | "reveal"

const ACTIONS: readonly TabMenuAction[] = ["close", "close-others", "close-right", "close-all", "reveal"]

export const REVEAL_LABEL = "Reveal in File Manager"

export function isTabMenuAction(id: string | null): id is TabMenuAction {
  return ACTIONS.some((action) => action === id)
}

// revealablePath is a working-tree document tab's path; other tabs (reviews,
// walkthroughs, Welcome, commit snapshots) have nothing on disk to reveal.
export function revealablePath(tab: WorkspaceTab): string | null {
  return tab.kind === "document" && !tab.at ? tab.path : null
}

// tabMenuItems mirrors the in-page tab menu, plus Reveal when the app offers
// it and the tab has a file on disk.
export function tabMenuItems(tab: WorkspaceTab, index: number, count: number, canReveal: boolean): NativeMenuItem[] {
  const items: NativeMenuItem[] = [
    { id: "close", label: "Close" },
    { id: "close-others", label: "Close Others", enabled: count > 1 },
    { id: "close-right", label: "Close to the Right", enabled: index < count - 1 },
    { separator: true },
    { id: "close-all", label: "Close All" },
  ]
  if (canReveal && revealablePath(tab) !== null) items.push({ separator: true }, { id: "reveal", label: REVEAL_LABEL })
  return items
}
