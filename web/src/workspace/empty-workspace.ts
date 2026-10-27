import type { SidebarView } from "./use-sidebar.ts"

// The document pane with no tab open names what the visible sidebar view
// offers, so the hint never points at a view that is not there (decision
// 0052). Pure; EmptyWorkspace.tsx only renders it.

// show-view opens the view's drawer on phones, where the sidebar is closed.
export type EmptyWorkspaceAction = "find-file" | "review-uncommitted" | "show-sidebar" | "show-view"

export interface EmptyWorkspaceInput {
  // view is the sidebar's view, or hidden when it is closed on desktop.
  // Phones follow the last active view.
  view: SidebarView | "hidden"
  mobile: boolean
  searchPattern: string
  // searchFiles counts files with results for the current pattern.
  searchFiles: number
  // changes is the uncommitted file count; null until read; repository is
  // false for a folder project.
  changes: number | null
  repository: boolean
  // walkthroughs is the listed count; null until read.
  walkthroughs: number | null
}

export interface EmptyWorkspace {
  heading: string
  hint: string
  action: EmptyWorkspaceAction | null
}

function count(value: number, one: string, many: string): string {
  return `${value.toLocaleString()} ${value === 1 ? one : many}`
}

export function emptyWorkspace(input: EmptyWorkspaceInput): EmptyWorkspace {
  const state = viewState(input)
  return input.mobile && !state.action ? { ...state, action: "show-view" } : state
}

function viewState(input: EmptyWorkspaceInput): EmptyWorkspace {
  switch (input.view) {
    case "hidden":
      return { heading: "Nothing open", hint: "Open a file, a review, or a walkthrough.", action: "show-sidebar" }
    case "explorer":
      return { heading: "Choose a file", hint: "Select a file from the folder tree.", action: "find-file" }
    case "search":
      return input.searchPattern.trim() && input.searchFiles > 0
        ? { heading: "Open a match", hint: "Choose a result to open it at that line.", action: null }
        : { heading: "Search file contents", hint: "Type in the Search field, then open a match.", action: null }
    case "source-control":
      if (!input.repository) return { heading: "Not a git repository", hint: "Source Control needs a git repository.", action: null }
      return input.changes
        ? {
          heading: "Review your changes",
          hint: `${count(input.changes, "uncommitted change", "uncommitted changes")}. Open a file, a commit, or a branch.`,
          action: "review-uncommitted",
        }
        : { heading: "Review history", hint: "No uncommitted changes. Open a commit or a branch.", action: null }
    case "walkthroughs":
      return input.walkthroughs === 0
        ? { heading: "No walkthroughs yet", hint: "Agents publish guided tours of this project here.", action: null }
        : { heading: "Choose a walkthrough", hint: "Open one from the list to start.", action: null }
  }
}
