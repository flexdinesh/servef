import { Button } from "../components/ui/button.tsx"
import { Kbd } from "../components/ui/kbd.tsx"
import type { EmptyWorkspace as EmptyWorkspaceState, EmptyWorkspaceAction } from "./empty-workspace.ts"

const mac = typeof navigator !== "undefined" && navigator.platform.includes("Mac")

function actionLabel(action: EmptyWorkspaceAction, viewName: string): { label: string; shortcut?: string } {
  switch (action) {
    case "find-file": return { label: "Find a file", shortcut: mac ? "⌘K" : "Ctrl+K" }
    case "review-uncommitted": return { label: "Review uncommitted changes" }
    case "show-sidebar": return { label: "Show sidebar", shortcut: mac ? "⌘B" : "Ctrl+B" }
    case "show-view": return { label: `Open ${viewName}` }
  }
}

interface EmptyWorkspaceProps {
  state: EmptyWorkspaceState
  // viewName names the sidebar view the state follows.
  viewName: string
  onAction(action: EmptyWorkspaceAction): void
  onWelcome(): void
}

// EmptyWorkspace fills the document pane while no tab is open, naming what
// the visible sidebar view offers (decision 0052).
export function EmptyWorkspace({ state, viewName, onAction, onWelcome }: EmptyWorkspaceProps) {
  const { action } = state
  const label = action ? actionLabel(action, viewName) : null
  return (
    <div className="empty empty-workspace">
      <h1>{state.heading}</h1>
      <p>{state.hint}</p>
      <div className="empty-workspace-actions">
        {action && label && (
          <Button variant="outline" onClick={() => onAction(action)}>
            {label.label}
            {label.shortcut && <Kbd>{label.shortcut}</Kbd>}
          </Button>
        )}
        <Button variant="link" size="inline" onClick={onWelcome}>Show Welcome</Button>
      </div>
    </div>
  )
}
