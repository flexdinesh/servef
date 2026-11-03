# Decision records

Short records of decisions that shape servef. Each states the context, the
decision, and its consequences. Supersede a record with a new one instead of
rewriting history; update `Status` on the old record.

| # | Decision | Status |
| --- | --- | --- |
| [0001](0001-filename-glob-scope.md) | Filename globs define CLI scope | Superseded by 0060 |
| [0002](0002-explicit-presentation-registry.md) | Explicit registry decides presentation only | Chroma highlighting superseded by 0029 |
| [0003](0003-view-modes-and-on-demand-format.md) | Preview for Markdown/images; Format on demand | Partially superseded by 0010 |
| [0004](0004-binary-and-size-limits.md) | Binary detection and size limits | Accepted |
| [0005](0005-images-as-documents.md) | Images are browsable documents | Accepted |
| [0006](0006-explorer-filename-filter.md) | Explorer filename filter and tab icons | Accepted |
| [0007](0007-projects-and-project-picker.md) | Projects, repo discovery, and project picker | Partially superseded by 0009 and 0052; exclusions by 0060 |
| [0008](0008-line-links-and-copy-path.md) | Line links and copy path | Accepted |
| [0009](0009-multiple-roots-and-worktree-picker.md) | Multiple roots, whole repositories, and worktree picker | Partially superseded by 0012; exclusions by 0060 |
| [0010](0010-markdown-opens-in-source.md) | Markdown opens in Source | Accepted |
| [0011](0011-token-preserving-web-formatters.md) | Token-preserving HTML, CSS, and JavaScript formatters | Accepted |
| [0012](0012-repository-names-and-worktree-header.md) | Repository names and worktree header | Accepted |
| [0013](0013-ui-settings-and-base-font-size.md) | UI settings and base font size | Accepted; theme default superseded by 0037 |
| [0014](0014-command-palette.md) | Command palette | Accepted |
| [0015](0015-code-navigation.md) | Code navigation with tree-sitter | Superseded for navigation by 0018; ignore sources by 0023 |
| [0016](0016-navigation-memory.md) | Navigation memory | Superseded by 0018 |
| [0017](0017-language-servers.md) | Language servers for code navigation | Superseded by 0018 |
| [0018](0018-lsp-only-navigation.md) | LSP-only navigation and host-specific availability | Accepted |
| [0019](0019-workspace-settings-and-memory.md) | Settings navigation and project explorer memory | Accepted |
| [0020](0020-activity-bar-and-search-view.md) | Activity bar and VS Code-style Search view | Accepted |
| [0021](0021-built-frontend-on-release-tags-and-edge.md) | Built frontend on release tags and edge | Superseded by 0079 |
| [0022](0022-excalidraw-only-mermaid.md) | Excalidraw-only Mermaid; no feature flags | Superseded by 0025 |
| [0023](0023-git-ignore-sources.md) | Git ignore sources: global excludes and worktrees | `.git` file clause superseded by 0031 |
| [0024](0024-source-control-view-and-working-tree-diffs.md) | Source Control view and working-tree diffs | Highlighting superseded by 0029; UI by 0035; no-polling refresh by 0050; browser diff parsing by 0083 |
| [0025](0025-ondine-mermaid.md) | Ondine renders Mermaid as themed SVG | Accepted |
| [0026](0026-vocs-user-docs-site.md) | Vocs user docs site | Partially superseded by 0074 |
| [0027](0027-commit-history-in-source-control.md) | Commit history in Source Control | Extended by 0030; UI superseded by 0035; no-polling refresh by 0050 |
| [0028](0028-diff-line-links-and-copy.md) | Diff line links and copy | No-scroll consequence superseded by 0034; routes by 0035 |
| [0029](0029-highlights-syntax-highlighting.md) | Pierre Highlights for syntax highlighting | Accepted |
| [0030](0030-branch-and-range-diffs.md) | Branch and commit-range diffs | UI superseded by 0035; head branches by 0040 |
| [0031](0031-hide-git-entries.md) | Hide .git entries | Accepted |
| [0032](0032-browser-tests-before-push.md) | Browser tests run before push, not in CI | Accepted; check commands superseded by 0038 |
| [0033](0033-design-tokens-and-ui-primitives.md) | Design tokens and UI primitives | Accepted |
| [0034](0034-diff-anchor-reveal-and-hunk-navigation.md) | Diff anchor reveal and hunk navigation | Accepted; extended to review tabs by 0035 |
| [0035](0035-compare-list-and-review-tab.md) | Compare list and review tab | Accepted; list and Files pane superseded by 0040; kept mounted by 0048 |
| [0036](0036-omarchy-themes.md) | Omarchy themes and the Color Theme quick pick | Accepted; built-in labels superseded by 0039 |
| [0037](0037-dark-default-theme.md) | Dark is the default theme | Accepted; labels superseded by 0039 |
| [0038](0038-mise-dev-tooling-and-env-ports.md) | mise owns dev tooling, tasks, and env-configured dev ports | Accepted |
| [0039](0039-servef-theme-labels.md) | Servef Light, Servef Dark, and Auto theme labels | Accepted |
| [0040](0040-source-control-panes-and-review-files-rail.md) | Source Control panes, branch stacks, and the review Files rail | Accepted; commit-row branch chips superseded by 0042 |
| [0041](0041-white-canvas-servef-light.md) | White canvas for Servef Light | Accepted; white document surface amended by 0043 |
| [0042](0042-expandable-commit-rows.md) | Expandable commit rows | Accepted |
| [0043](0043-grey-document-surface-servef-light.md) | Grey document surface for Servef Light | Accepted; Servef Light values amended by 0044; grey document surface superseded by 0046 |
| [0044](0044-servef-light-state-contrast.md) | Servef Light state and boundary contrast | Accepted; field border and tint-only states amended by 0045; Servef Light values amended by 0046 |
| [0045](0045-selection-marks-and-field-edges.md) | Selection marks and 3:1 field edges | Accepted; activity rail bar removed by 0046 |
| [0046](0046-slate-greys-and-flat-tabs.md) | Slate greys for Servef Light, flat tabs and toolbar | Accepted |
| [0047](0047-walkthroughs.md) | Walkthroughs | Accepted; publishing without a server added by 0065 |
| [0048](0048-kept-review-tab.md) | The last review tab stays mounted behind other tabs | Accepted |
| [0049](0049-copyable-agent-instructions.md) | Copyable agent instructions for walkthroughs | Accepted; CLI publish added by 0065 |
| [0050](0050-live-updates.md) | Live updates for the projects browsers show | Accepted; revised by 0066 |
| [0051](0051-branch-review-with-uncommitted.md) | Branch review with uncommitted changes | Accepted |
| [0052](0052-welcome-tab-and-contextual-empty-state.md) | Welcome tab and an empty pane that follows the sidebar view | Accepted |
| [0053](0053-files-rail-viewed-marks.md) | Viewed marks in the review Files rail | Accepted |
| [0054](0054-worktree-checks.md) | Queued worktree checks and critical browser coverage | Accepted; supersedes 0032/0038 check execution; check output superseded by 0057 |
| [0055](0055-scrollbars.md) | Thin, hover-revealed scrollbars with reserved gutters | Accepted |
| [0057](0057-quiet-check-runner.md) | Quiet check runner and targeted agent checks | Accepted |
| [0058](0058-local-dagger-validation.md) | Explicit local Dagger validation before PR work | Boundaries, certification, and PR wrappers superseded by 0059; engine ownership retained |
| [0059](0059-validation-execution-boundaries.md) | Test isolation and safe host checks before PR work | Accepted |
| [0060](0060-scope-flags-removed.md) | Scope flags removed; `servef browser` alias | Accepted; bare `servef` clause superseded by 0068 |
| [0061](0061-daemon-roots-and-leases.md) | Daemon, runtime roots, and project leases | Accepted (leases via 0066) |
| [0062](0062-desktop-app.md) | Desktop app | Accepted (0067, 0068, 0070, 0071, 0072, 0081) |
| [0063](0063-cli-commands-and-url-scheme.md) | CLI commands and the `servef://` URL scheme | Accepted (0065, 0068, 0071); revised by 0076 |
| [0064](0064-ssh-remotes.md) | SSH remotes | Accepted (0070); revised by 0076 |
| [0065](0065-walkthrough-publish-cli.md) | `servef walkthrough publish` without a server | Accepted; revised by 0068, 0069 |
| [0066](0066-lease-levels-and-desktop-events.md) | Lease levels, head events, and daemon control events | Accepted |
| [0067](0067-desktop-shell-and-protocol-proxy.md) | Desktop shell and protocol proxy | Accepted; revised by 0070, 0072, 0075, 0081 |
| [0068](0068-cli-opens-desktop-app-and-desktop-releases.md) | CLI opens the desktop app; desktop releases | Accepted; revised by 0072, 0073, 0076, 0079 (Releases, Homebrew) |
| [0069](0069-servef-remote-and-ssh-links.md) | `servef remote` and ssh links | Superseded by 0076 |
| [0070](0070-desktop-ssh-connections.md) | Desktop ssh connections | Accepted; revised by 0076, 0079 (install instructions) |
| [0071](0071-cli-server-connections-and-pasted-urls.md) | CLI-server connections and pasted URLs | Accepted; revised by 0076 |
| [0072](0072-os-integration.md) | OS integration: open folders from the OS, open at login | Accepted; revised by 0081 |
| [0073](0073-install-scripts.md) | Install scripts | Accepted; revised by 0078 (paths), 0079 (edge scripts, Homebrew and `go install`) |
| [0074](0074-landing-page-and-cloudflare-pages.md) | Landing page and Cloudflare Pages | Accepted; revised by 0079 (install tabs) |
| [0075](0075-desktop-title-bar.md) | Desktop title band | Accepted |
| [0076](0076-remote-servers-over-http.md) | Remote servers over http; ssh driven from the app | Accepted |
| [0077](0077-desktop-shell-stays-on-electron.md) | Desktop shell stays on Electron | Accepted |
| [0078](0078-apps-and-packages-layout.md) | `apps/` and `packages/` layout | Accepted; revised by 0080 |
| [0079](0079-releases-edge-prerelease-and-install-scripts.md) | Releases, the edge prerelease, and install scripts only | Accepted |
| [0080](0080-application-layer-contracts-and-transports.md) | Application layer, generated contracts, and transports | Accepted; source control stages by 0083 |
| [0081](0081-desktop-bridge-v2-native-features.md) | Desktop bridge v2: reveal, native context menus, recent folders, folder drop | Accepted |
| [0083](0083-server-structured-diffs.md) | Server-side structured diffs and the source control domain | Accepted |
