# Native parity matrix

The orchestrator's queue for web-to-native parity (decision 0086): one row per user-visible behavior of the web UI (and parts of the Electron app) against the experimental mygo app (decision 0082).
Use: pick a `missing` row of the lowest open milestone, follow `skills/mygo-parity/SKILL.md`, then update the row's status, scenario, and PR when merged.
Status: `done` (native behaves equivalently), `partial` (gap noted), `missing`. Milestone `M0/0082` means shipped in M1 (#145).
Web files are under `apps/web/src/` unless a path starts with `apps/`. Scenarios live in `apps/native/scenarios/`; italic names are proposed.

## Summary

| Milestone | Scope | Rows | Done / partial / missing |
| --- | --- | --- | --- |
| M0/0082 | Shipped in M1 (#145): connections, projects, explorer, Source, Markdown preview, live updates, uncommitted review | 24 | 24 / 0 / 0 |
| M1 | Ports: Highlights (wazero), ondine diagrams, trees widget, diffs viewer | 6 | 5 / 1 / 0 |
| M2 | Shell: activity bar, sidebar, tabs and tab menu, title bar, status bar, Welcome, empty state, workspace memory, project picker | 13 | 0 / 4 / 9 |
| M3 | Settings dialog, font size, themes (Omarchy), color theme picker, command palette, quick pick, commands | 8 | 0 / 1 / 7 |
| M4 | Documents: Format, Mermaid, fences, images, line links, copy path, navigation memory | 8 | 8 / 0 / 0 |
| M5 | Explorer: filename filter, gitignore marks, icons, context menus | 5 | 1 / 1 / 3 |
| M6 | Search: filename and content, include/exclude globs | 7 | 0 / 0 / 7 |
| M7 | Code navigation and language-server status | 4 | 0 / 0 / 4 |
| M8 | Source Control: panes, commits, branches, compares, review tab, files rail, viewed, hunks, line links, stacks, stats | 20 | 0 / 4 / 16 |
| M9 | Walkthroughs: list, tab, narration, step bar, announcements | 8 | 0 / 0 / 8 |
| M10 | Bridge v2: reveal, context menus, recent folders, folder drop, notifications | 6 | 0 / 0 / 6 |
| M11 | ssh: Connect page hosts, ControlMaster, socket forwards, remote folder picker | 6 | 0 / 0 / 6 |
| Total | | 115 | 35 / 10 / 70 |

## Shell and workspace

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Activity bar (Explorer, Search, Source Control, Walkthroughs) | `workspace/Sidebar.tsx`, `workspace/use-sidebar.ts` | 0020 | partial (Files/Changes switch only) | M2 | _activity-bar_ | |
| Resizable sidebar; hide and show | `workspace/SidebarResizer.tsx`, `workspace/use-sidebar-resize.ts`, `workspace/use-sidebar.ts` | 0020 | partial (split only; no hide, width not remembered) | M2 | _sidebar-resize_ | |
| Tabs: open, switch, close (document, review, walkthrough, Welcome) | `workspace/TabBar.tsx`, `workspace/workspace-tabs.ts` | 0035, 0056 | missing | M2 | _tabs_ | |
| Tab menu (Close, Close Others, Close to the Right, Close All) | `workspace/tab-menu.ts`, `workspace/TabBar.tsx` | 0081 | missing | M2 | _tab-menu_ | |
| Window title (document, project, connection) | `workspace/TitleBar.tsx`, `apps/desktop/src/main/titles.ts` | 0075 | done | M0/0082 | projects | #145 |
| Title band with app menu button | `workspace/TitleBar.tsx`, `apps/desktop/src/main/titlebar.ts` | 0075 | missing | M2 | _title-bar_ | |
| Status bar: connection, branch, language, live state | `workspace/StatusBar.tsx` | 0050 | done | M0/0082 | source-view | #145 |
| Status bar: path, copy path, Find in files, Settings, process info | `workspace/StatusBar.tsx`, `workspace/process-metrics.ts` | 0008, 0013, 0080 | missing | M2 | _status-bar_ | |
| Welcome tab (start entries, guides, projects) | `features/welcome/WelcomeTab.tsx`, `features/welcome/welcome-model.ts` | 0052 | missing | M2 | _welcome_ | |
| Contextual empty pane per sidebar view | `workspace/EmptyWorkspace.tsx`, `workspace/empty-workspace.ts` | 0052 | partial (file count line only) | M2 | _empty-workspace_ | |
| Workspace memory per sessionKey (last project, tabs, active tab, expansion) | `workspace/workspace-memory.ts`, `workspace/use-project-memory.ts` | 0019, 0061 | missing | M2 | _workspace-memory_ | |

## Settings and commands

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Settings dialog (search, categories, modified filter, reset, Show Welcome) | `settings/SettingsDialog.tsx`, `settings/settings.ts` | 0013, 0019, 0052 | missing | M3 | _settings_ | |
| Base font size | `settings/settings.ts`, `settings/SettingsProvider.tsx` | 0013 | missing | M3 | _font-size_ | |
| Servef Light and Servef Dark palettes | `styles/tokens.css`, `styles/themes.css` | 0037, 0041, 0043, 0044, 0046 | done | M0/0082 | dark-theme | #145 |
| Theme choice (Auto, Light, Dark), persisted | `settings/theme.ts`, `settings/ThemeProvider.tsx` | 0037, 0039 | partial (follows system only) | M3 | _theme-choice_ | |
| Omarchy themes | `settings/theme.ts`, `styles/themes.css` | 0036, 0039 | missing | M3 | _omarchy-themes_ | |
| Color theme picker with live preview | `workspace/ColorThemePicker.tsx`, `workspace/color-theme-pick.ts` | 0036 | missing | M3 | _color-theme-picker_ | |
| Command palette (Cmd/Ctrl+Shift+P, F1, fuzzy match) | `workspace/CommandPalette.tsx`, `workspace/commands.ts` | 0014 | missing | M3 | _command-palette_ | |
| Commands and shortcuts (settings, navigation, walkthrough, help; Cmd+O, Cmd+,) | `workspace/commands.ts`, `App.tsx` | 0014, 0020 | missing | M3 | _command-palette_ | |
| Quick pick (top-anchored palette shell: projects, Color Theme) | `components/ui/dialog-shell.tsx`, `features/projects/ProjectPicker.tsx` | 0014, 0033 | missing | M3 | _quick-pick_ | |

## Projects and connections

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Connect page: this computer (daemon ensure), CLI servers, disconnect | `apps/desktop/src/main/app-pages.ts`, `apps/desktop/src/main/cli-servers.ts`, `apps/desktop/src/main/daemon.ts` | 0061, 0071, 0082 | done | M0/0082 | connect-page | #145 |
| Open a server by address or pasted link | `apps/desktop/src/main/location.ts`, `apps/desktop/src/main/app-pages.ts` | 0071, 0076 | done | M0/0082 | connect-page | #145 |
| Links (`servef-native://` mirrors `servef://`); single instance | `apps/desktop/src/main/scheme.ts`, `apps/desktop/src/main/main.ts` | 0063, 0072, 0082 | done | M0/0082 | _links_ | #145 |
| Project list (name, branch, path, availability, warnings) | `features/projects/ProjectPicker.tsx`, `features/projects/project-picker.ts` | 0007 | done | M0/0082 | projects | #145 |
| Repository names and worktree header | `features/explorer/FileTree.tsx`, `features/projects/project-picker.ts` | 0012 | partial (root name and branch only) | M2 | projects | |
| Project picker filter, remembered projects (Cmd/Ctrl+O) | `features/projects/ProjectPicker.tsx`, `features/projects/project-picker.ts` | 0007, 0019 | missing | M2 | _project-picker_ | |
| Worktree grouping and drill-in | `features/projects/project-picker.ts` | 0009, 0012 | missing | M2 | _project-picker_ | |

## Documents

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Read-only Source with line numbers and find | `features/documents/SourceDocument.tsx` | 0003, 0082 | done | M0/0082 | source-view | #145 |
| Highlighting in the declared language, off the UI thread | `highlighting/source-highlighter.ts`, `highlighting/highlight-thread.ts` | 0029 | done | M0/0082 | source-view | #145 |
| Highlights token parity (wazero port) | `highlighting/highlights-loader.ts`, `highlighting/syntax-highlight.ts` | 0029, 0082, 0088 | done | M1 | source-languages | |
| Source/Preview toggle; Markdown opens in Source | `features/documents/document-view.ts`, `features/documents/DocumentModeToggle.tsx` | 0003, 0010 | done | M0/0082 | document-preview | #145 |
| Markdown preview: document, heading, and external links; root-confined images | `features/documents/DocumentPreview.tsx` | 0002, 0005, 0010 | done | M0/0082 | document-preview | #145 |
| Highlighted fences with language chip and copy | `features/documents/DocumentPreview.tsx`, `features/documents/CodeBlockControls.tsx` | 0029, 0088 | done (language label in mygo's head row, not the copy chip) | M4 | markdown-fences | |
| Format (on demand, read-only) | `features/documents/FormattedDocument.tsx`, `features/documents/use-document-format.ts` | 0003, 0011 | done | M4 | format, format-dark | |
| Ondine diagram renderer (port) | `features/documents/ondine-renderer.ts`, `features/documents/diagram.ts` | 0025, 0088 | done | M1 | mermaid | |
| Mermaid rendered in preview, source on error | `features/documents/MermaidBlock.tsx`, `features/documents/MermaidSource.tsx` | 0022, 0025, 0088 | done | M4 | mermaid | |
| Mermaid expand, zoom, reset | `features/documents/OndineMermaidDiagram.tsx` | 0025 | done | M4 | mermaid-expanded | |
| Image documents | `features/documents/ImageDocument.tsx` | 0005 | done | M0/0082 | image-document | #145 |
| Image zoom controls | `features/documents/ImageZoomControls.tsx`, `features/documents/image-zoom.ts` | 0005 | done | M4 | image-document | |
| Binary, tooLarge, and preview-disabled notes | `features/documents/BinaryDocument.tsx`, `features/documents/DocumentNotice.tsx` | 0004 | done | M0/0082 | file-notes | #145 |
| Line selection, line links (`#L`), copy path and line reference | `features/documents/SourceDocument.tsx`, `lib/line-selection.ts` | 0008 | done (copy path in the document toolbar until the M2 status bar; Copy Link in the context menu, no address bar) | M4 | line-links | |
| Navigation memory (back/forward, scroll per document) | `features/documents/use-document-navigation.ts` | 0008, 0050 | done (scroll per document, not per history entry) | M4 | view tests | |

## Explorer

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Virtualized file tree; reveals the open document | `features/explorer/FileTree.tsx` | 0007, 0031 | done | M0/0082 | source-view | #145 |
| Tree header: switch project, refresh | `features/explorer/FileTree.tsx` | 0007 | partial (Projects button only) | M5 | _explorer-header_ | |
| Filename filter (base-name globs) | `features/explorer/FilenameFilter.tsx`, `features/explorer/filename-filter.ts` | 0001, 0006 | missing | M5 | _filename-filter_ | |
| Gitignore marks | `features/explorer/FileTree.tsx`, `lib/tree-css.ts` | 0023 | missing (`trees` dims `Ignored`; `TreeNode` has no ignored field) | M5 | _gitignore-marks_ | |
| Pierre tree widget and icon set (port) | `lib/tree-css.ts`, `components/FileIcon.tsx` | 0082, 0090 | partial (`packages/mygo/trees`, chains flattened as the web; no sticky folders or middle truncation) | M1 | explorer, explorer-dark | |
| File icons and git statuses in tree rows | `components/FileIcon.tsx`, `features/explorer/FileTree.tsx` | 0033, 0090 | done | M5 | explorer, explorer-dark | |
| Row context menu (Copy Path, Copy Relative Path) | `features/explorer/tree-menu.ts` | 0081 | missing | M5 | _explorer-menu_ | |

## Search

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Filename search in the Explorer (Cmd/Ctrl+K, P) | `features/explorer/FileTree.tsx` | 0020 | missing | M6 | _search-filename_ | |
| Search view: content search, results per file, refresh, clear | `features/search/SearchView.tsx`, `features/search/use-search-session.ts` | 0020 | missing | M6 | _search-content_ | |
| Match case, whole word, regex | `features/search/search-engine.ts` | 0020 | missing | M6 | _search-content_ | |
| Include and exclude globs | `features/search/search-scope.ts` | 0001, 0020 | missing | M6 | _search-globs_ | |
| Line previews with highlights; open at match | `features/search/search-results.ts`, `features/search/search-highlights.ts` | 0020 | missing | M6 | _search-content_ | |
| Ignored, binary, and large files excluded | `features/search/search-controller.ts` | 0004, 0023 | missing | M6 | _search-content_ | |
| Stale results notice on live change | `features/search/search-controller.ts` | 0050 | missing | M6 | _search-stale_ | |

## Code navigation

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Go to Definition (F12, click) | `features/navigation/use-code-navigation.ts`, `features/navigation/code-navigation.ts` | 0015, 0018 | missing | M7 | _go-to-definition_ | |
| Find All References (Shift+F12), locations dialog | `features/navigation/use-code-navigation.ts`, `features/navigation/NavigationDialog.tsx` | 0018 | missing | M7 | _find-references_ | |
| Language-server status panel and install guidance | `features/navigation/NavigationStatusPanel.tsx`, `features/navigation/navigation-status.ts` | 0017, 0018 | missing | M7 | _language-servers_ | |
| Language servers on/off setting | `settings/settings.ts`, `features/navigation/use-navigation-status.ts` | 0017 | missing | M7 | _language-servers_ | |

## Source Control

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Uncommitted changes list (status, renames, warnings, non-repo note) | `features/source-control/UncommittedPane.tsx`, `features/source-control/use-changes.ts` | 0024 | done | M0/0082 | changes-diff | #145 |
| Diff split and unified | `features/source-control/DiffToolbar.tsx`, `features/source-control/diff-metadata.ts` | 0024, 0083 | done | M0/0082 | changes-diff | #145 |
| Ignore whitespace; full file (expand unchanged) | `features/source-control/diff-options.ts`, `features/source-control/diff-hunks.ts` | 0083 | done | M0/0082 | _diff-options_ | #145 |
| Diff sides highlighted in the server's language | `features/source-control/diff-resources.ts`, `highlighting/highlight-thread.ts` | 0029, 0083 | done | M0/0082 | changes-diff | #145 |
| Line counts; binary and too-large diff notes | `features/source-control/diff-stats.ts`, `features/source-control/DiffStatsText.tsx` | 0004, 0024 | done | M0/0082 | changes-diff | #145 |
| Wrap lines | `features/source-control/DiffToolbar.tsx`, `features/source-control/use-diff-options.ts` | 0024 | missing | M8 | _diff-options_ | |
| Inline changes menu (word, character, none) | `features/source-control/DiffToolbar.tsx` | 0083 | missing | M8 | _diff-options_ | |
| Word emphasis in changed lines (diffs viewer) | `features/source-control/diff-metadata.ts` | 0082, 0083, 0089 | done | M1 | changes-diff | |
| In-place gap expansion | `features/source-control/diff-hunks.ts`, `features/source-control/diff-metadata.ts` | 0034, 0083, 0089 | done | M1 | diff-gaps | |
| No newline at end of file marker | `features/source-control/diff-metadata.ts` | 0083, 0089 | done | M1 | diff-gaps | |
| Panes in fixed order, collapsible, sash | `features/source-control/SourceControlView.tsx`, `features/source-control/Pane.tsx`, `features/source-control/source-control-layout.ts` | 0040 | missing | M8 | _source-control-panes_ | |
| Commit history with paging | `features/source-control/CommitsPane.tsx`, `features/source-control/commit-pages.ts`, `features/source-control/use-commits.ts` | 0027 | missing | M8 | _commits_ | |
| Expandable commit rows, copy hash | `features/source-control/CommitsPane.tsx` | 0042 | missing | M8 | _commits_ | |
| Commit range selection | `features/source-control/CommitsPane.tsx`, `features/source-control/compare.ts` | 0035 | missing | M8 | _commit-range_ | |
| Branches pane and ref chips | `features/source-control/BranchesPane.tsx`, `features/source-control/use-refs.ts` | 0030, 0040 | missing | M8 | _branches_ | |
| Base picker | `features/source-control/BasePicker.tsx` | 0030 | missing | M8 | _branches_ | |
| Branch and range compares | `features/source-control/compare.ts`, `features/source-control/use-compare-data.ts`, `features/source-control/compare-files.ts` | 0030, 0035 | missing | M8 | _branch-compare_ | |
| Branch plus uncommitted scope | `features/source-control/ReviewDocument.tsx` | 0051 | missing | M8 | _branch-compare_ | |
| Branch stacks | `features/source-control/use-branch-stack.ts`, `features/source-control/source-control-panes.ts` | 0040 | missing | M8 | _branches_ | |
| Compare stats | `features/source-control/use-compare-stats.ts`, `features/source-control/DiffStatsText.tsx` | 0035 | missing | M8 | _branches_ | |
| Review tab: every file of a compare in one document | `features/source-control/ReviewDocument.tsx`, `features/source-control/review-diffs.ts` | 0035 | partial (one file at a time) | M8 | _review-tab_ | |
| Kept review tab | `features/source-control/ReviewDocument.tsx` | 0048 | missing | M8 | _review-tab_ | |
| Files rail (tree, line stats, uncommitted dot) | `features/source-control/ReviewFilesRail.tsx`, `features/source-control/FilesTree.tsx`, `features/source-control/files-tree-decoration.ts` | 0040, 0051 | missing | M8 | _files-rail_ | |
| Viewed marks | `features/source-control/review-viewed.ts`, `features/source-control/use-review-viewed.ts` | 0053 | missing | M8 | _files-rail_ | |
| Hunk navigation (F7, Shift+F7) | `features/source-control/diff-hunks.ts`, `features/source-control/DiffToolbar.tsx` | 0034, 0089 | partial (within the open file; no step to the next file) | M8 | diff-hunk-step | |
| Diff line selection, links, copy, anchor reveal | `features/source-control/diff-line-selection.ts`, `features/source-control/compare.ts` | 0028, 0034, 0089 | partial (select and copy reference; no links or anchor reveal from a link) | M8 | diff-line-selection | |
| Open file from review; commit snapshots (`at=`) | `features/source-control/ReviewDocument.tsx`, `workspace/workspace-tabs.ts` | 0027, 0035 | missing | M8 | _review-tab_ | |
| Refresh on return to the window | `features/source-control/use-refresh-on-return.ts` | 0050 | partial (Refresh button only) | M8 | changes-diff | |

## Walkthroughs

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Walkthroughs list (polling, refresh, delete) | `features/walkthrough/WalkthroughsView.tsx`, `features/walkthrough/use-walkthroughs.ts` | 0047 | missing | M9 | _walkthroughs-list_ | |
| Copy agent instructions | `features/walkthrough/agent-instructions.ts` | 0049 | missing | M9 | _walkthroughs-list_ | |
| Walkthrough tab: cover and steps | `features/walkthrough/WalkthroughTab.tsx` | 0047 | missing | M9 | _walkthrough-tab_ | |
| Narration (Markdown) | `features/walkthrough/Narration.tsx` | 0047 | missing | M9 | _walkthrough-tab_ | |
| Step targets (file lines, diff selections) and line drift | `features/walkthrough/walkthrough-steps.ts`, `features/walkthrough/use-walkthrough-session.ts` | 0047 | missing | M9 | _walkthrough-steps_ | |
| Step bar: previous, next (Cmd/Ctrl+Left, Right), remembered step | `features/walkthrough/WalkthroughStepBar.tsx`, `features/walkthrough/walkthrough-memory.ts` | 0047 | missing | M9 | _walkthrough-steps_ | |
| Present mode | `features/walkthrough/WalkthroughStepBar.tsx` | 0047 | missing | M9 | _walkthrough-present_ | |
| Announcements (notification opens the walkthrough) | `apps/desktop/src/main/notifications.ts`, `apps/desktop/src/main/subscriptions.ts` | 0065, 0066 | missing | M9 | _walkthrough-announce_ | |

## Live updates

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Event stream per open project (`lease=open`), reconnect, live indicator | `features/live-updates/use-live-updates.ts`, `features/live-updates/live-updates.ts` | 0050, 0066 | done | M0/0082 | _live-updates_ | #145 |
| Reread open document, tree, changes list, and open diff on change | `features/live-updates/live-updates.ts`, `features/source-control/use-changes.ts` | 0050 | done | M0/0082 | _live-updates_ | #145 |
| Reread keeps scroll | `features/documents/use-document-navigation.ts` | 0050 | done | M4 | view tests | |
| `head` event (branch label) | `features/live-updates/use-live-updates.ts` | 0050, 0066 | done | M0/0082 | _live-updates_ | #145 |
| One stream per visible tab | `features/live-updates/use-live-updates.ts` | 0050 | missing | M2 | _tabs_ | |

## Desktop bridge v2

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Open Folder dialog; folders from argv and the OS | `lib/desktop-bridge.ts`, `apps/desktop/src/main/folder-opener.ts`, `apps/desktop/src/main/open-paths.ts` | 0072, 0081 | done | M0/0082 | _open-folder_ | #145 |
| Reveal in File Manager (explorer rows, tabs) | `apps/desktop/src/main/reveal.ts`, `features/explorer/tree-menu.ts` | 0081 | missing | M10 | _reveal_ | |
| Native context menus (tabs, explorer rows) | `apps/desktop/src/main/context-menu.ts`, `apps/desktop/src/main/electron-context-menu.ts` | 0081 | missing | M10 | _context-menus_ | |
| Recent folders (Open Recent, clear, system recents, Dock) | `apps/desktop/src/main/recent-folders.ts` | 0081 | missing | M10 | _recent-folders_ | |
| Folder drop on a window | `apps/desktop/src/preload/preload.ts`, `apps/desktop/src/main/open-paths.ts` | 0081 | missing | M10 | _folder-drop_ | |
| Notices: branch switch, commit, settled | `apps/desktop/src/main/notifications.ts`, `apps/desktop/src/main/subscriptions.ts` | 0066 | missing | M10 | _notifications_ | |
| Watched lease for background projects | `apps/desktop/src/main/subscriptions.ts`, `apps/desktop/src/main/sse.ts` | 0066 | missing | M10 | _notifications_ | |

## ssh and remote

| Feature | Web files | Decision | Native status | Milestone | Scenario | PR |
| --- | --- | --- | --- | --- | --- | --- |
| Remote http servers over TCP, confirmed once per host and port | `apps/desktop/src/main/remote-hosts.ts`, `apps/desktop/src/main/electron-dialogs.ts` | 0076 | done | M0/0082 | _links_ | #145 |
| Connect page ssh hosts, config aliases, host pages | `apps/desktop/src/main/app-pages.ts`, `apps/desktop/src/main/connection-store.ts`, `apps/desktop/src/main/ssh-config.ts` | 0070, 0076 | missing | M11 | _ssh-hosts_ | |
| App-owned ControlMaster (argv only, BatchMode) | `apps/desktop/src/main/ssh-args.ts`, `apps/desktop/src/main/ssh-connection.ts`, `apps/desktop/src/main/runtime-dir.ts` | 0070 | missing | M11 | _ssh-connect_ | |
| Remote daemon and servers over Unix-socket forwards (direct http fallback) | `apps/desktop/src/main/forward.ts`, `apps/desktop/src/main/proxy.ts`, `apps/desktop/src/main/remote-hosts.ts` | 0070, 0076 | missing | M11 | _ssh-connect_ | |
| Remote folder picker (`/_daemon/directories`) | `apps/desktop/src/main/app-pages.ts`, `features/projects/project-picker.ts` | 0076 | missing | M11 | _remote-folder-picker_ | |
| Failure pages, reconnecting state, titles | `apps/desktop/src/main/ssh-errors.ts`, `apps/desktop/src/main/pages.ts`, `apps/desktop/src/main/titles.ts` | 0070 | missing | M11 | _ssh-failures_ | |
| ssh links and first-contact confirm | `apps/desktop/src/main/scheme.ts`, `apps/desktop/src/main/electron-dialogs.ts` | 0070, 0076 | missing | M11 | _ssh-links_ | |
