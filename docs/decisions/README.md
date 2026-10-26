# Decision records

Short records of decisions that shape servef. Each states the context, the
decision, and its consequences. Supersede a record with a new one instead of
rewriting history; update `Status` on the old record.

| # | Decision | Status |
| --- | --- | --- |
| [0001](0001-filename-glob-scope.md) | Filename globs define CLI scope | Accepted |
| [0002](0002-explicit-presentation-registry.md) | Explicit registry decides presentation only | Chroma highlighting superseded by 0029 |
| [0003](0003-view-modes-and-on-demand-format.md) | Preview for Markdown/images; Format on demand | Partially superseded by 0010 |
| [0004](0004-binary-and-size-limits.md) | Binary detection and size limits | Accepted |
| [0005](0005-images-as-documents.md) | Images are browsable documents | Accepted |
| [0006](0006-explorer-filename-filter.md) | Explorer filename filter and tab icons | Accepted |
| [0007](0007-projects-and-project-picker.md) | Projects, repo discovery, and project picker | Partially superseded by 0009 |
| [0008](0008-line-links-and-copy-path.md) | Line links and copy path | Accepted |
| [0009](0009-multiple-roots-and-worktree-picker.md) | Multiple roots, whole repositories, and worktree picker | Partially superseded by 0012 |
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
| [0021](0021-built-frontend-on-release-tags-and-edge.md) | Built frontend on release tags and edge | Accepted |
| [0022](0022-excalidraw-only-mermaid.md) | Excalidraw-only Mermaid; no feature flags | Superseded by 0025 |
| [0023](0023-git-ignore-sources.md) | Git ignore sources: global excludes and worktrees | `.git` file clause superseded by 0031 |
| [0024](0024-source-control-view-and-working-tree-diffs.md) | Source Control view and working-tree diffs | Highlighting superseded by 0029; UI by 0035 |
| [0025](0025-ondine-mermaid.md) | Ondine renders Mermaid as themed SVG | Accepted |
| [0026](0026-vocs-user-docs-site.md) | Vocs user docs site | Accepted |
| [0027](0027-commit-history-in-source-control.md) | Commit history in Source Control | Extended by 0030; UI superseded by 0035 |
| [0028](0028-diff-line-links-and-copy.md) | Diff line links and copy | No-scroll consequence superseded by 0034; routes by 0035 |
| [0029](0029-highlights-syntax-highlighting.md) | Pierre Highlights for syntax highlighting | Accepted |
| [0030](0030-branch-and-range-diffs.md) | Branch and commit-range diffs | UI superseded by 0035; head branches by 0040 |
| [0031](0031-hide-git-entries.md) | Hide .git entries | Accepted |
| [0032](0032-browser-tests-before-push.md) | Browser tests run before push, not in CI | Accepted; check commands superseded by 0038 |
| [0033](0033-design-tokens-and-ui-primitives.md) | Design tokens and UI primitives | Accepted |
| [0034](0034-diff-anchor-reveal-and-hunk-navigation.md) | Diff anchor reveal and hunk navigation | Accepted; extended to review tabs by 0035 |
| [0035](0035-compare-list-and-review-tab.md) | Compare list and review tab | Accepted; list and Files pane superseded by 0040 |
| [0036](0036-omarchy-themes.md) | Omarchy themes and the Color Theme quick pick | Accepted; built-in labels superseded by 0039 |
| [0037](0037-dark-default-theme.md) | Dark is the default theme | Accepted; labels superseded by 0039 |
| [0038](0038-mise-dev-tooling-and-env-ports.md) | mise owns dev tooling, tasks, and env-configured dev ports | Accepted |
| [0039](0039-servef-theme-labels.md) | Servef Light, Servef Dark, and Auto theme labels | Accepted |
| [0040](0040-source-control-panes-and-review-files-rail.md) | Source Control panes, branch stacks, and the review Files rail | Accepted; commit-row branch chips superseded by 0042 |
| [0041](0041-white-canvas-servef-light.md) | White canvas for Servef Light | Accepted; white document surface amended by 0043 |
| [0042](0042-expandable-commit-rows.md) | Expandable commit rows | Accepted |
| [0043](0043-grey-document-surface-servef-light.md) | Grey document surface for Servef Light | Accepted |
