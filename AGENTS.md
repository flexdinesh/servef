# AGENTS.md

## Purpose

`servef` is a Go CLI and local server for browsing local files through an embedded Vite/React UI.

Read `docs/decisions/` before changing scope, file types, views, or limits. Record new decisions there; supersede rather than rewrite.

## Boundaries

- Go CLI/server: `main.go`, `cmd/`, and Go files under `internal/`.
- Local validation: `tools/validate/` owns committed/current-edit snapshots, host static checks, input certificates, admission, and artifacts; `.dagger/` owns all test execution and its immutable preparation. Public mise tasks orchestrate; hidden `_host:*` and `_container:*` leaves never invoke public wrappers. PR publishing is separate. This tooling is not part of the shipped application.
- `cmd/` owns CLI validation, session configuration, application startup, and the `servef daemon` commands, including spawning a detached daemon. It also owns `servef walkthrough` (publish, schema) in `cmd/walkthrough.go` (decision 0065). Bare `servef [path]` opens the desktop app through the daemon and falls back to browser mode with a notice (`cmd/app.go`, decision 0068); `servef browser` always serves.
- `internal/runtime/` owns the web app, HTTP server, and shutdown order (event streams end first, then in-flight requests get 5 s) shared by the CLI and the daemon. It knows no transport or listener.
- `internal/daemon/` owns the daemon (decisions 0061, 0066): per-user socket path, private directory and `flock` single instance, the `/_daemon/*` control API (intercepted before the web app: status, roots, covering open, the control event stream, walkthrough announcements, stop), `roots.json`, idle exit, the Go `Client`, and `Ensure`. It knows no CLI flags and spawns no processes.
- `internal/files/` owns traversal, the always-skipped `.git`, `node_modules`, and `vendor` directories (`Excluded`), depth, base-name globs (`NameFilter`), gitignore marking, and safe indexed-path resolution. It also exports the 0023 ignore rules (`IgnoreRules`), an unscoped walk of non-ignored files (`WalkUnignored`), root-confined `ResolveFile` for changed paths, and the directories a scan reads (`Directories`) for live updates. Accept an explicit matcher; know no file formats.
- `internal/projects/` owns the served-root set (`RootSet`: ordered, concurrency-safe; `Add` enforces 0009 rules), repository/worktree discovery, and project-directory resolution within the roots (repositories open whole) or standalone (`ResolveStandalone`: the directory is its own root). It knows the `.git` marker, git's worktree and `HEAD` files, where git state lives (`ReadGitState`: HEAD, index, refs), and the `origin` remote URL only.
- `internal/git/` owns go-git repository opening, HEAD tree and index reads, working-tree change detection against HEAD or HEAD's merge base with a base branch, exact rename pairing, HEAD blob reads, base-branch detection and branch listing, bounded commit walks (`base..HEAD` and full history), commit-to-first-parent file changes, bounded merge-base and branch/commit-range tree changes (a branch compare's head may be any local branch), bounded branch-stack detection, commit blob reads, and per-file line counts. It knows git formats only; no HTTP or document policy.
- `internal/document/` owns file-type metadata (including each family's highlight language and image media types), lookup, previews, formatting, binary/size policy (`Available`), and document-link policy. Handlers receive bytes/context; never perform filesystem or HTTP work.
- `internal/lsp/` owns language-server data (`Builtins()`), host-specific install guidance, binary discovery, process lifecycle (start on demand, idle stop, failure backoff), and the JSON-RPC client. It knows absolute paths and LSP positions only; `internal/web/` validates indexed project access and maps locations to transport responses. Language servers own all symbol resolution.
- `internal/web/` owns indexed-file reads, HTTP routing, the local-Host check, and transport contracts. Every JSON error is `errorResponse` (`{error, code?}`). Delegate format behavior to document handlers. It exports `NewWalkthroughTargets` (the 0047 checks) for the CLI.
- `internal/walkthrough/` owns the walkthrough model, strict decoding, structural validation, the JSON Schema, the user-cache store, and `Service` (publish, list, get, delete), which every transport shares. It knows no HTTP and no git; `internal/web/` implements its `Targets` check against the index and git (decision 0047).
- `internal/watch/` owns live updates (decisions 0050, 0066): the subscription hub, open and watched lease levels, one shared reference-counted fsnotify watcher, per-project debounce, the stat-poll fallback, and watch limits. It knows absolute paths only; `internal/web/` composes each project's target from `files.Directories` and `projects.ReadGitState` and streams `/api/events`.
- `internal/control/` owns authenticated discovery (`List`, `Lookup`) and shutdown of local servef processes; `internal/browser/` opens the browser; `internal/desktopapp/` detects the desktop app's `servef://` handler, opens URLs in it, and gives per-OS install hints (never installs anything); `internal/version/` reports the build version.
- `desktop/` is the Electron app (decision 0067): its main process is a client of the daemon socket (`servef daemon ensure`, `/_daemon/open`, `/_daemon/events`, `lease` streams) and proxies `servef-app://local` to it; it never reads project files or installs anything. Only `src/main/main.ts`, `src/main/electron-*.ts`, and `src/preload/` import `electron`; every other module is pure or Node-only and unit-tested (oxlint enforces it). It reuses `web/src/api/events-contract.ts` by relative import.
- Frontend source: `web/`. `web/src/` keeps `main.tsx`, `router.tsx`, `App.tsx` (composition only), and `style.css` (import manifest and cascade layers only) at its root; `styles/` holds tokens, themes, base, layout, and print CSS; `components/ui/` holds the UI primitives; `gallery/` is the dev-only primitive gallery (`/__ui` in `pnpm dev:mock`); `api/` holds the HTTP adapter (`http.ts`), transport contracts, and data adapters; `features/<feature>/` holds each feature's views, hooks, and pure logic; `highlighting/` is shared by documents and diffs; `workspace/` holds tabs, memory, sidebar, status bar, and palette; `settings/` holds settings and their providers; `lib/` holds shared helpers.
- Every API read goes through `fetchJSON` in `web/src/api/http.ts`, which turns network, non-JSON, server-rejected, and invalid responses into user-facing errors. The one stream, `/api/events`, is an `EventSource` in `web/src/features/live-updates/use-live-updates.ts`; validate its events with `web/src/api/events-contract.ts`.
- React owns interaction and presentation. Use server metadata for file families and view capabilities; never duplicate extension detection in the UI.
- User docs site: `site/` (Vocs, static, build-time only; `docs/decisions/0026-vocs-user-docs-site.md`). Contributor docs stay in `docs/`.
- `internal/web/dist/` is generated by `pnpm build:web`; never edit or commit it. Run `mise run build:web` before host development builds. Dagger builds private embedded assets before Go tests and vet; validation never consumes host build outputs. Only release tags and the CI-published `edge` branch carry it (`docs/decisions/0021-built-frontend-on-release-tags-and-edge.md`).

Key files:

| Concern | Files |
| --- | --- |
| CLI | `cmd/root.go` |
| Families and lookup | `internal/document/builtins.go`, `registry.go` |
| Handlers | `internal/document/code.go` (source/format), `json.go`, `yaml.go`, `html.go`, `css.go`, `javascript.go`, `markdown.go`, `mermaid.go` |
| Binary/size limits | `internal/document/content.go` |
| Projects | `internal/projects/projects.go`, `web/src/features/projects/project-picker.ts`, `docs/decisions/0007-projects-and-project-picker.md`, `0009-multiple-roots-and-worktree-picker.md`, `0012-repository-names-and-worktree-header.md` |
| Sidebar and search | `web/src/workspace/Sidebar.tsx`, `use-sidebar.ts`, `web/src/features/search/SearchView.tsx`, `use-search-session.ts`, `search-controller.ts`, `search-engine.ts` (pure matching), `search-scope.ts` (include/exclude globs), `search-results.ts`, `docs/decisions/0020-activity-bar-and-search-view.md` |
| Source Control | `internal/git/` (`numstat.go` line counts, `stack.go` branch stacks), `internal/web/routes.go` (`/api/changes`, `/api/diff`, `/api/commits`, `/api/commit-files`, `/api/branch-files`, `/api/range-files`, `/api/refs`, `/api/branch-stack`, `/api/compare-stats`), `internal/web/commits.go`, `compare.go`, `web/src/features/source-control/` (`SourceControlView.tsx` (fixed pane order), `Pane.tsx` (collapsible pane, sash), `UncommittedPane.tsx`, `CommitsPane.tsx`, `BranchesPane.tsx`, `BasePicker.tsx`, `FilesTree.tsx`, `files-tree-decoration.ts` (pure row stats, uncommitted dot, and viewed CSS), `ReviewFilesRail.tsx` (rail, phone sheet), `ReviewDocument.tsx`, `compare.ts` (pure compare model, labels, routes, `#f=` fragments, range selection), `source-control-panes.ts` (pure Diff placement, stack rows, ref chips), `compare-files.ts`, `commit-pages.ts` (pure paging), `use-commits.ts`, `use-branch-stack.ts`, `use-compare-data.ts`, `use-compare-stats.ts`, `use-refs.ts`, `review-diffs.ts` (lazy diffs, bounded parse cache), `review-viewed.ts`, `use-review-viewed.ts`, `source-control-layout.ts` (pure panes, weights, rail), `use-changes.ts`, `use-refresh-on-return.ts`, `diff-line-selection.ts` (pure diff anchors and line references), `diff-hunks.ts` (pure hunk selection, steps, and collapsed gaps), `diff-options.ts`, `diff-stats.ts`), `web/src/lib/bounded-cache.ts`, `web/src/api/commits-contract.ts`, `changes-contract.ts`, `refs-contract.ts`, `stack-contract.ts`, `compare-stats-contract.ts`, `web/src/highlighting/syntax-theme.ts`, `highlight-thread.ts`, `web/vite.config.ts` (`servef-diffs-subset`), `docs/decisions/0024-source-control-view-and-working-tree-diffs.md`, `0029-highlights-syntax-highlighting.md`, `0027-commit-history-in-source-control.md`, `0028-diff-line-links-and-copy.md`, `0030-branch-and-range-diffs.md`, `0034-diff-anchor-reveal-and-hunk-navigation.md`, `0035-compare-list-and-review-tab.md`, `0040-source-control-panes-and-review-files-rail.md`, `0051-branch-review-with-uncommitted.md`, `0053-files-rail-viewed-marks.md`, `patches/@pierre__diffs@1.6.1.patch`, `patches/@pierre__trees@1.0.0-beta.6.patch` (`setUnsafeCSS`) |
| HTTP | `internal/web/routes.go` (route table, Host check), `documents.go`, `projects.go`, `assets.go` (`/api/page` and `/api/format` (`at=<full hash>` reads a commit snapshot: Source and Format only), `/api/raw`, `/api/image`, `/api/search-documents`, `/api/definition`, `/api/references`, `/api/changes`, `/api/diff`, `/api/commits`, `/api/commit-files`, `/api/branch-files` (`uncommitted=1` adds the working tree to HEAD's branch), `/api/range-files`, `/api/refs`, `/api/branch-stack`, `/api/compare-stats`, `/api/walkthroughs` (GET, POST, DELETE), `/api/walkthrough`, `/api/events` (server-sent events), all scoped by `project`; `/api/projects`, `/api/directories`, `/api/metrics`, `/api/walkthrough-schema`) |
| Walkthroughs | `internal/walkthrough/` (`walkthrough.go` model and validation, `store.go`, `service.go`, `schema.json`), `internal/web/walkthroughs.go` (handlers, Origin/JSON guards, `Targets` checks, `NewWalkthroughTargets`), `cmd/walkthrough.go` (`servef walkthrough publish`, `schema`), `web/src/features/walkthrough/` (`walkthrough-steps.ts` pure routes, labels, drift, and stepping; `agent-instructions.ts` the copyable agent prompt, checked against the schema; `walkthrough-memory.ts`; `use-walkthroughs.ts` polling; `use-walkthrough-session.ts`; `WalkthroughsView.tsx`; `WalkthroughTab.tsx`; `Narration.tsx`; `WalkthroughStepBar.tsx` present mode and phones), `web/src/api/walkthrough-contract.ts`, `site/src/pages/walkthroughs.mdx`, `skills/servef-walkthrough/SKILL.md` (agent guidance), `docs/decisions/0047-walkthroughs.md`, `0049-copyable-agent-instructions.md`, `0065-walkthrough-publish-cli.md` |
| Transport contract | `web/src/api/` (`http.ts`, `page-contract.ts`, `projects-contract.ts`, `navigation-contract.ts`, `changes-contract.ts`, `commits-contract.ts`, `api-fixtures.test.ts`), `testdata/api/default/`, `web/dev-server.ts` |
| View selection | `web/src/features/documents/document-view.ts` (pure), `DocumentPane.tsx`, `DocumentModeToggle.tsx` |
| Syntax highlighting | `web/src/highlighting/` (`syntax-highlight.ts` pure rows, `syntax-theme.ts`, `highlights-loader.ts`, `highlight-thread.ts`, `source-highlighter.ts`, `highlight-worker.ts`, `use-highlighted-lines.ts`), `web/src/features/documents/DocumentPreview.tsx` (fences), `docs/decisions/0029-highlights-syntax-highlighting.md` |
| Code navigation | `internal/lsp/`, `internal/web/navigation.go`, `web/src/features/navigation/code-navigation.ts` (pure), `use-code-navigation.ts`, `source-position.ts`, `NavigationDialog.tsx`, `docs/decisions/0018-lsp-only-navigation.md` |
| Language servers | `internal/lsp/` (`servers.go` data, `manager.go`, `client.go`, `lsptest/` fake), `internal/web/navigation.go` (`/api/navigation-status`), `web/src/features/navigation/navigation-status.ts` (pure), `use-navigation-status.ts`, `NavigationStatusPanel.tsx`, `docs/decisions/0018-lsp-only-navigation.md` |
| Desktop | `desktop/src/main/` (`main.ts` composition and menu, `electron-protocol.ts`, `electron-windows.ts` window security and routing, `electron-notify.ts` notice log seam, `proxy.ts` + `forward.ts` + `headers.ts` servef-app proxy, `connections.ts`, `daemon.ts` + `daemon-contract.ts` ensure/open/stop, `binary.ts`, `scheme.ts` servef:// routing, `windows.ts`, `navigation.ts`, `subscriptions.ts` + `sse.ts`, `notifications.ts` notices, settle, throttle, `pages.ts` + `instructions.ts` failure pages, `bridge.ts`), `desktop/src/preload/preload.ts` (bridge v1), `desktop/test/` (unit; `smoke/` Playwright `_electron`), `desktop/scripts/measure-memory.ts`, `desktop/electron-builder.yml` + `desktop/scripts/package.ts` (packages, decision 0068), `cmd/app.go` + `internal/desktopapp/` (`servef [path]` opens the app), `web/src/lib/desktop-bridge.ts`, `shareable-origin.ts`, `cmd/daemon_unix.go` (`closeInheritedOnExec`), `docs/decisions/0062-desktop-app.md`, `0067-desktop-shell-and-protocol-proxy.md`, `0068-cli-opens-desktop-app-and-desktop-releases.md` |
| Live updates | `internal/watch/` (`watch.go` hub and subscriptions, `project.go` per-project watch and debounce, `poll.go` fallback, `backend.go` fsnotify), `internal/files/directories.go`, `internal/projects/git_state.go`, `internal/web/events.go` (`/api/events`, `lease`, `head`), `internal/git/head.go`, `internal/web/notices.go` (walkthrough announcements), `web/src/api/events-contract.ts`, `web/src/features/live-updates/` (`live-updates.ts` pure targets and labels, `use-live-updates.ts` stream), `use-document-navigation.ts` (`reread` keeps scroll), `search-controller.ts` (`onStale`), `tests/browser/live-fixture.ts`, `docs/decisions/0050-live-updates.md`, `0066-lease-levels-and-desktop-events.md` |
| Design system | `web/src/styles/tokens.css`, `themes.css`, `web/src/style.css` (manifest), `web/src/components/ui/`, `web/src/gallery/`, `web/.oxlintrc.json`, `web/stylelint.config.mjs`, `DESIGN.md`, `docs/decisions/0033-design-tokens-and-ui-primitives.md` |
| Explorer filter | `web/src/features/explorer/filename-filter.ts`, `FilenameFilter.tsx`, `FileTree.tsx` |
| Daemon | `internal/runtime/runtime.go`, `internal/daemon/` (`daemon.go` `Run`, `control.go` routes and `Status`, `listen_unix.go` socket and lock, `paths.go`, `roots.go`, `idle.go`, `client.go`, `ensure.go`), `cmd/daemon.go`, `docs/decisions/0061-daemon-roots-and-leases.md`, `0066-lease-levels-and-desktop-events.md` |
| Welcome and empty pane | `web/src/features/welcome/` (`welcome-model.ts` pure entries, project lines, and sections; `WelcomeTab.tsx`), `web/src/workspace/empty-workspace.ts` (pure, per sidebar view), `EmptyWorkspace.tsx`, `workspace-memory.ts` (`startupTab`), `web/src/api/projects-data.ts` (`landingProject`), `docs/decisions/0052-welcome-tab-and-contextual-empty-state.md` |

## Patterns

- Capabilities over type switches: the server declares `previewKind`, `formattable`, page-level `navigationSupported`, `binary`, `previewDisabled`, `searchExcluded`, `tooLarge`; the UI derives modes and views from them in `document-view.ts`.
- Optional interfaces: add behavior as a small interface (e.g. `Formatter`) that the registry detects and reports, not as flags handlers must remember.
- One composition point: `Builtins()` lists every family explicitly; a family's metadata is data, its behavior is its handler.
- Fail closed at startup: invalid registrations panic in `Builtins()`; invalid CLI input errors before serving.
- On-demand work: expensive or optional transforms (Format) get their own endpoint; `/api/page` stays cheap.
- Pure decision functions: keep view/mode logic and glob matching pure and unit-tested; components only render.

## Contracts

- Define families through `document.Type` and behavior through `document.Handler`: `Type` and `Preview`. Content search reads original source; handlers know nothing about it. Formatting is the optional `document.Formatter` capability; the registry derives `formattable` from it. Language-server support is composed by web from `lsp.Builtins()` as page-level `navigationSupported`; document handlers know no code-navigation behavior.
- Declare extensions/filenames, language, preview kind/label, and default mode explicitly. Preview kinds are `markdown` and `image`; types without one default to Source.
- Validate composition with `NewRegistry`; reject duplicate/ambiguous IDs, extensions, and filenames, missing or malformed languages, and invalid view policies. Web tests reject languages Highlights does not know.
- Treat handler output as a contract: escape file text in preview/highlight HTML; leave input bytes unchanged; preserve source and search access on preview errors.
- Keep Go response shapes, TypeScript runtime validators in `web/src/api/page-contract.ts`, and `testdata/api` fixtures aligned. Validate unknown transport data before use; preserve strict TypeScript safety.

## System Design

- Compose families explicitly in `internal/document.Builtins()`. Registration decides presentation only; every in-scope file is browsable. Lookup: exact base name, then extension, then the plain-text fallback. The browser highlights the declared language with Highlights (decision 0029); never guess types.
- Reuse `CodeHandler` for highlighted source and `FormattedCodeHandler` for formatted text. Implement a specialized handler only when behavior requires it. Source-only additions should need no discovery, HTTP, or frontend format branches.
- Prefer cohesive packages, small semantic interfaces, and explicit dependencies. Avoid global registries, scattered file-type switches, and speculative plugin frameworks.
- Follow the file-type extension guide in `docs/development.md`; preserve the single-binary distribution.

## Encapsulation

- Keep parsing and formatting inside the owning handler. Keep code intelligence in installed language servers. Share highlighting and code-view behavior through existing document primitives.
- Keep filesystem safety in `internal/files/`; renderers must not bypass indexed-path resolution.
- Keep UI filter state separate from server scope and document/tab state. Consume declared capabilities rather than inspecting filenames or inferring behavior from rendered HTML.
- Expose semantic operations through package/component contracts. Keep parser/renderer internals private; do not widen APIs solely for tests.

## Behavior

- Source shows original text. Formatting is read-only; preserve meaningful values and presentation, including JSON number precision/key order and YAML comments, scalar styles, anchors/aliases, and multiple documents.
- Malformed files remain readable and searchable. Highlighting falls back to escaped text when a language is unknown or tokenization fails or panics.
- Binary files (NUL in the first 8000 bytes) send no source and skip search. Over 4 MiB: no highlighting, format, or non-image preview. Over 8 MiB: also skip search. Over 32 MiB (non-image): not read; `tooLarge` note only.
- Every file within `--depth` is available; `.git`, `node_modules`, and `vendor` directories are always skipped (decision 0060). Tree filters only change visibility; preserve open tabs, the active document, expansion, and responsive state. Filename search intersects the tree filter; global content search covers every available file.
- Gitignored files stay browsable in the tree but are excluded from content search and returned navigation locations.
- Code navigation is Source-only and LSP-only. Follow VS Code bindings. Binary files and files over 4 MiB do not navigate. No local parsing, symbol index, import guesses, or syntax fallback. Keep only indexed, in-scope, non-ignored project locations; preserve UTF-16 ranges.
- Language servers are enabled by default per browser, authorized per request with `X-Servef-Language-Servers: on`, started only by navigation, stopped after 10 idle minutes, and closed with servef. Status requests never start servers. Show unsupported, missing, disabled, starting, ready, and failed states beside the language label; missing servers show host-specific install instructions.
- Memory footprint is a priority: measure servef RSS and language-server process-tree RSS separately before and after navigation changes. Bound open documents, response sizes, startup/query time, and cancellation.
- Source Control is repository-scoped: `--depth` and the always-skipped directories do not apply. It reads worktree files only for paths validated against the change set, through `files.ResolveFile`; HEAD sides come from `internal/git/`. Commit history (0027) is repository-scoped too; commit diffs read both sides from git objects, validated against the commit's change set. 0004 limits apply per diff side. Compare stats (0035) count both sides under the same limits within a 64 MiB per-request budget. Never run the `git` binary.
- The daemon (0061) listens on a Unix socket only (0600 in a 0700 per-user directory; macOS and Linux), one per user under a `flock`, and exits after 30 idle minutes (no request in flight). Clients block only on a protocol mismatch, never a release mismatch. Saved roots that are missing at start are skipped and kept. `/_daemon/open` serves a path, replacing served roots inside it; `/_daemon/events` announces walkthroughs and keeps the daemon alive while open (0066). Browser memory keys by `sessionKey` (`daemon`, or the CLI's roots).
- Live updates (0050) follow only subscribed projects: a visible tab's `/api/events` stream is its interest; the server keeps no current project. Watch what a scan reads plus git state; past 4096 directories (8192 entries on kqueue) or on a watch failure, poll. Bound followed projects (8), paths per change (256), and stop a project 30 s after its last subscriber. Streams hold `lease=open` (default) or `lease=watched` (git state only, 16 projects, never lists the tree; 0066); an open project drops to watched 30 s after its last open stream. Every checkout stream sends `head` (baseline, then on moves).
- Keep Markdown image dependencies separate from browsable-document scope while enforcing root confinement. Serve raw bytes only for image-preview types, sandboxed.
- Test semantic behavior across boundaries: lookup/scope, direct access, source preservation, preview/format failures, binary/size limits, search, and UI navigation. Include source-only composition coverage when changing extension contracts; avoid tests tied to private implementation details.

## Adding Languages and File Types

Pick the smallest step that delivers the behavior. Each step is deliberate: no language guessing.

1. Source-only language (highlighting only): add one line to `Builtins()`, e.g. `source("zig", "Zig", "zig", []string{".zig"})`.
   - `Language` must be a Highlights language (`text` for none); fish, INI, Properties, and reStructuredText have no lexer and use `text`.
   - IDs, extensions, and filenames share one case-insensitive namespace; pick unused ones.
   - Add the family and its language to `testdata/document-families.json`. Tests fail until the fixture matches `Builtins()` and every language is a Highlights language (decision 0029).
   - No HTTP or contract change is needed.
2. Files identified by name (`Dockerfile`, `Makefile`): pass filenames as trailing args to `source(...)`. Without a Highlights lexer, add them to the `text` family instead.
3. Read-only Format: use `FormattedCodeHandler` with a `FormatSource` that copies input and preserves meaningful values. `formattable` and the Format button follow automatically.
4. New preview of an existing kind (`markdown`, `image`): implement `document.Handler` and register it. Escape HTML; keep Source available on preview errors.
5. New preview kind (e.g. table): this is a contract change. Update together:
   - `internal/document/registry.go` preview-kind constants and validation.
   - `web/src/api/page-contract.ts` `previewKind` union and validator.
   - `web/src/features/documents/document-view.ts` view kinds and `DocumentPane.tsx` rendering.
   - `testdata/api/default/` fixtures, `web/dev-server.ts` mocks, and a decision record.
6. Code navigation: add an explicit `lsp.Builtins()` server entry for registered text families (launch command/args, LSP language IDs, prerequisites, host-specific install recipes, docs). `web.New` validates composition. Use real negotiated definition/reference capabilities; a language server does not imply every method is supported. No handler, HTTP type switch, or frontend extension detection is needed.
7. Test language-server behavior through `internal/lsp/lsptest` and a real-server smoke test when available. Include missing/failed/starting states, changes on disk, cancellation, and project confinement.
8. Binary formats with a viewer: declare an empty `Language` only for image-like families and extend `Type.HasTextSource`/`/api/raw` deliberately; never serve raw bytes for text families.

For every addition:

- Add a sample under `testdata/markdown/` and tests for lookup (extension, filename, fallback), highlighting, search, and preview/format failure where relevant.
- Keep `TestAPIFixturesMatchServerDocumentContract` green: fixtures must carry the registry's real `Type` JSON.
- Keep `TestDocumentFamiliesFixtureMatchesBuiltins` and `web/src/highlighting/syntax-languages.test.ts` green: every family declares a Highlights language (`text` for none); raster images declare none.
- Update `site/src/pages` (and the README feature summary) when user-visible behavior changes. Tree and tab icons come from Pierre by file name; unknown names get the generic icon.

## Distribution

- The CLI must run from one self-contained Go binary. The desktop app is a separate artifact that bundles that binary (`resources/bin/servef`) plus Electron's Chromium and Node (decision 0068).
- Node, pnpm, and other JavaScript tooling are build-time only for the CLI. Optional, user-installed language servers may require their own runtimes; browsing never requires them.
- Build frontend assets before Go compilation and embed them through `internal/web/dist/`.
- Bundle runtime assets locally. Do not require a CDN, frontend sidecar, or external asset server.

## Conventions

- Make minimal, surgical changes.
- Preserve strict TypeScript safety: no `any`, non-null assertions, or type assertions.
- Build UI from the primitives in `web/src/components/ui/` (decision 0033). They are hand-owned React components with co-located CSS in `layer(ui)`; add a missing capability to the primitive instead of forking it in a feature. Never use raw `button`, `input`, `select`, `textarea`, or `dialog` outside `components/ui/`.
- Co-locate each feature's CSS beside it and import it from `style.css` in `layer(features)`. Use tokens from `web/src/styles/tokens.css`: no color literals, raw font sizes, radii, z-indexes, durations, or px above 2. Tailwind stays for its theme and utilities; primitives do not use Tailwind classes.
- `pnpm lint` runs oxlint (Hooks rules, primitive-only elements) and stylelint (tokens only); both must report nothing.
- Format Go with `gofmt`; test behavior, not implementation details.

## Iteration

Use `testdata/markdown` and `mise run dev` while iterating; it also serves the browser tests' git fixture, rebuilt in the gitignored `.dev/git-fixture`, for Source Control, and publishes sample walkthroughs (`scripts/dev-walkthroughs.ts`) into `.dev/cache`. `mise run dev:desktop` runs the Electron app against an isolated dev daemon under `.dev/desktop`. mise is required: `mise.toml` declares tools (locked in `mise.lock`), loads the gitignored `.env` (dev ports: `SERVEF_DEV_SERVER_PORT`, `SERVEF_DEV_WEB_PORT`), and owns every composite task; `package.json` keeps single JS steps (`docs/decisions/0038-mise-dev-tooling-and-env-ports.md`).

While editing, use targeted mise checks below. They capture current edits and
relevant new files in immutable snapshots; targeted results do not establish PR
validation evidence. All tests run in Dagger, including unit and runner tests.
Safe static checks run on the host with private worktree-local writable state.
Development and formatting remain explicit host operations (decision 0059;
see `docs/validation.md`).

| Change | Command |
| --- | --- |
| Go behavior | `mise run test:go`; `mise run test:race` for race coverage |
| Frontend logic | `mise run test:web` |
| TypeScript types | `mise run typecheck` |
| CSS, components, Hooks | `mise run lint` |
| Release tooling, `scripts/` | `mise run test:release` |
| `mise.toml` | `mise run mise:check` |
| Formatting and Go modules | `mise run fmt:check`, `mise run tidy:check` |
| Docs site | `mise run typecheck`; `mise run validate -- --group docs` after committing |
| Browser behavior | `mise run test:browser -- tests/browser/<spec>.ts` |
| Desktop app (`desktop/`) | `mise run test:desktop` (unit, then Electron smoke); `mise run typecheck`, `mise run lint` |
| Desktop packaging | `mise run package:desktop:dir` (unpacked) or `mise run package:desktop` (installers), host OS and arch; `actionlint` for `release.yml` |

- Before creating or updating a PR, commit changes, run `mise run check:pr`, then run `mise run check:status` immediately before the separate push/PR operation. Confirm HEAD still matches the reported tested commit. Do not publish changed tracked content using an earlier result.
- No mandatory checks run on push. `check:pr` covers all eleven host/container groups, including race and docs builds; `validate`, `check`, and `check:full` are aliases. `check:status` checks relevant input coverage without running checks. `--force` reruns checks and builds; a failed or cancelled active rerun revokes prior success for that input. Partial or incomplete runs never qualify as full validation.
- PR creation and updates are separate from validation. Local instructions and status checks are bypassable; Git has no standard pre-PR hook. There is no remote enforcement. Existing CI and releases remain unchanged.
- Run only against the explicitly managed `servef-validation-engine-v0.21.10`. Never automatically bootstrap, remove, restart, or prune another engine. Preserve other repositories' engines and volumes; cancellation stops only the current invocation.
- Validation prints per-group execution/reuse and a unique local artifact directory. Read failure logs there; browser traces/screenshots and diagnostics survive failure. Rerun only the failed isolated group while iterating, then establish aggregate coverage before PR work.
- Use `mise run check:prune` for completed artifacts: defaults are 30 days and 10 GiB per worktree. Active runs are protected by locks. Never prune engines or their volumes as part of validation.
- Run long checks in the background and wait for the completion notification. Never poll logs or `sleep` in loops.
