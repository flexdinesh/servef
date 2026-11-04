# 0091: Native workspace shell and memory

- Status: Accepted (experimental)
- Date: 2026-11-05
- Relates to: [0082](0082-native-mygo-experiment.md) (the native app),
  [0086](0086-native-agent-tooling-and-ports.md) (parity loop),
  [0019](0019-workspace-settings-and-memory.md) (workspace memory),
  [0020](0020-activity-bar-and-search-view.md) (activity bar),
  [0052](0052-welcome-tab-and-contextual-empty-state.md) (empty pane),
  [0056](0056-borderless-editor-tabs.md) (tabs),
  [0061](0061-daemon-roots-and-leases.md) (`sessionKey`)

## Context

The native app showed one document at a time beside a Files/Changes
switch and forgot everything on quit. M2 of the parity plan brings the
web's shell: activity bar, sidebar, editor tabs, tab menu, empty pane,
and workspace memory. The web keeps memory in `localStorage`, which is
per origin; the native app has no origin and can connect to several
servers.

## Decision

- Pure Go models mirror the web's pure modules and their tests:
  `internal/tabs` (`workspace-tabs.ts`, `tab-menu.ts`, the close and
  merge rules of `use-document-workspace.ts`), `internal/memory`
  (`workspace-memory.ts`: last project per `sessionKey`, tabs, active
  tab, open folders, LRU caps of 100 projects and 20 sessions), and
  `internal/shell` (`use-sidebar.ts`, `use-sidebar-resize.ts`,
  `empty-workspace.ts`). `internal/view/shell.go` only draws them.
- Memory is one file, `workspaces.json` in the app's user data dir: the
  file is 0600, written atomically and synced, and the directory is
  mygo's (0755). Saves wait 500 ms off the UI thread for later ones, so a
  sidebar drag writes once, and quit flushes them. Workspaces are kept
  per connection (the stand-in for the web's origin, at most 20), then
  per `sessionKey` as the web's; sidebar view, hidden state, and width
  are global as the web's.
- A malformed file is nothing remembered and the next save replaces it.
  A mistyped field drops alone. A file of another version is neither read
  nor written, so a newer app's memory survives an older app.
- Restore follows the web: the remembered project opens when it is still
  listed; remembered tabs and folders are checked against the fresh
  tree; a linked document joins after the remembered tabs and stays
  active. It waits for a project's first good page, as the web's
  `ready`: a failed page changes no memory, and nothing is saved until
  the restore ran.
- The review is one Uncommitted tab showing one change at a time; it
  opens on the first change. The tab menu is the web's, drawn as a
  native context menu; Reveal waits for M10.
- Tabs own `docview.Memory` (0008): closing a tab forgets its view and
  scroll, as `closeTabs` in `App.tsx`, and its Format. Switching tabs
  pushes to the history, as a route change does; Back from a review or
  the empty pane shows the document last shown, and back to a closed
  tab's document opens it again.
- Closed things are released: a closed review's diff and highlighted
  lines, and a closed tab's editor, image, and preview blocks. An open
  tab's document keeps its editor under a review and its blocks until
  the next preview, so it returns where it stood; its image is read
  again.
- Activity bar and tab icons are Lucide's, generated from the web's
  pinned `lucide-react` by `apps/native/scripts/sync-lucide-icons.mjs`;
  a test checks the pin against `pnpm-lock.yaml`, and Lucide's ISC
  license ships in `THIRD_PARTY_NOTICES.txt`.
- M2 is split: M2a (this) is tabs, tab menu, memory, activity bar,
  sidebar, and the empty pane; M2b is the title band, status bar parity,
  the Welcome tab, and the project picker.

## Consequences

- Not yet: Welcome, snapshot, commit, branch, and walkthrough tabs; the
  Find a file action (M3); phone layouts; a double-click width reset.
  The sidebar's keyboard step is mygo's `Split` step (10 DIPs), not 16.
- Search and Walkthroughs show placeholders until M6 and M9.
- `apps/native/scripts/measure-retention.sh` measures what stays after a
  3.5 MiB document and a 1 MiB-per-side review close. Against a87ce717,
  which has no tabs to close and replaces them with a small document:
  live heap settles at 14 MiB instead of 34 and RSS at 82 MiB instead of
  107, from 7 and 61 at project open. Peaks do not change: 141 MiB with
  the document, 137 with the review, as before within noise.
