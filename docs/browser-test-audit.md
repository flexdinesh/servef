# Browser test audit

Browser tests protect browser integration: routing, native selection and
clipboard, focus, scrolling, workers, rendering, and live updates. Pure policy,
formatting, transport validation, and exhaustive input combinations belong in
Go or Node tests. Cosmetic dimensions and exhaustive theme sweeps do not gate
pushes.

This audit reduces 224 browser cases to 87. It removes tests rather than hiding
them behind a second browser suite. No product behavior changes.

## Coverage decisions

Counts include parameterized cases. Paths below are relative to `tests/browser/`.

| Spec | Before | After | Decision |
| --- | ---: | ---: | --- |
| `app.spec.ts` | 35 | 15 | Keep production bootstrap, real search, links/history, stale-response recovery, settings command, OS theme changes, highlighting, clipboard, modes, and bundled Mermaid rendering/interaction. Remove repeated entry points, cosmetic sizes, exhaustive themes, and tab/memory cases covered elsewhere. |
| `code-navigation.spec.ts` | 20 | 8 | Keep definition/reference navigation, F12 focus, browser caret coordinates, native selection/copy, authorization setting, cancellation, and server-failure refresh. Run native selection with the host modifier once. Pure navigation eligibility, location grouping, status labels, and installation data remain below the browser layer. |
| `color-theme.spec.ts` | 4 | 1 | Keep keyboard preview, cancel, commit, and reload. Theme aliases and matching are pure tests; omit repeated focus and pointer variants. |
| `design-consistency.spec.ts` | 5 | 0 | Remove exact row/button/field sizes, colors, radii, shadows, and padding comparisons. Token lint and theme contrast tests remain; these are not substitutes for visual review. |
| `diff-highlighting.spec.ts` | 1 | 1 | Keep production worker/shadow-DOM syntax rendering. |
| `document-views.spec.ts` | 13 | 5 | Keep binary rendering, one production image/zoom case, narrow SVG controls, image-decode failure, and one disabled-preview case. Remove filename/type matrices and repeated size thresholds already checked by Go and `document-view.test.ts`. |
| `editor-layout.spec.ts` | 12 | 3 | Keep tree keyboard activation and mobile drawer document/search navigation. Remove cosmetic geometry and repeated search/scroll cases. |
| `file-types.spec.ts` | 11 | 1 | Keep production on-demand JSON formatting and source preservation. Go tests already verify JSON/YAML/HTML/CSS/JS semantics. Filter/state coverage stays in its dedicated specs. |
| `filename-filter.spec.ts` | 9 | 3 | Keep filter/search scope interaction, invalid-filter recovery, and responsive expansion preservation. Go `TestFilenameScopeControlsTreePagesAndSearch` covers server scope, so no dedicated filtered browser server is needed. Remove cosmetic icons and repeated empty/scope-label cases. |
| `formatted-code.spec.ts` | 3 | 1 | Keep failed formatting followed by retry. Source-row escaping and blank rows remain in highlighting tests; pending-format policy remains in `document-view.test.ts`. |
| `line-links.spec.ts` | 5 | 3 | Keep click/shift/clipboard, pointer drag, and actual anchor scrolling. Remove duplicate mode selection and footer-copy entry points. |
| `live-updates.spec.ts` | 3 | 3 | Keep disk-to-browser document, tree/status, and branch-review updates. Unit/server tests cannot prove these subscriptions update mounted views. |
| `mobile-workspace.spec.ts` | 2 | 1 | Keep overflowed active-tab visibility. Drawer shortcut focus is omitted as a secondary interaction; mobile navigation remains covered. |
| `project-picker.spec.ts` | 14 | 4 | Keep worktree selection/memory, folder browsing, repository boundaries, and mobile switching. Remove label/icon matrices and repeated picker entry points. Pure picker tests retain filtering and selection policy. |
| `projects.spec.ts` | 5 | 1 | Keep project-specific tabs and last-project restoration. Server project tests retain scoping/unavailable-project behavior. |
| `react-architecture.spec.ts` | 13 | 8 | Keep production search retention and scroll/history once each, Worker recovery, module/WASM recovery, Strict Mode portals, callback replacement, and pointer resize. Remove dev/production duplicates and ancillary metrics-polling browser cases. Metrics transport remains tested; polling lifecycle is intentionally no longer a browser gate. |
| `settings.spec.ts` | 3 | 2 | Keep search/filter composition and controls reachable in small viewports. Remove exact footer/header/chevron geometry. |
| `source-control.spec.ts` | 48 | 19 | Keep working/commit/branch/range review flows, virtualization, anchors/hunk navigation, line copy, mobile/split behavior, request recovery, retained review state, base switching, pagination, viewed marks, and worker disposal. Remove presentation matrices, repeated tree/pane persistence, and pure route/status cases. Remove the two-second responsiveness threshold: bounded rendered rows and usable controls prove behavior without treating host CPU contention as failure. |
| `tab-menu.spec.ts` | 5 | 2 | Keep pointer focus restoration and keyboard close/neighbor activation. Omit repeated menu actions; workspace restoration remains separately covered. |
| `walkthroughs.spec.ts` | 3 | 2 | Keep published walkthrough navigation, snapshot/diff targets, drift, presentation mode, and phone controls. Agent-instruction schema/content is already tested in Node. |
| `welcome.spec.ts` | 6 | 2 | Keep first landing/project selection and restoration precedence. Pure welcome/empty-workspace models cover labels and entry decisions; omit repeated shortcut entry points. |
| `workspace-memory.spec.ts` | 4 | 2 | Keep project-isolated folder restoration and prevention of destructive empty writes during startup. Filter responsive remounts remain in `filename-filter.spec.ts`. |

## Cheaper coverage retained

- `internal/document/*_test.go`: family lookup, original source, token-preserving
  formatting, escaping, malformed input, binary/size boundaries.
- `internal/web/*_test.go`: scoped access, HTTP contracts, images, navigation,
  git comparisons, walkthroughs, and live-event transport.
- `web/src/features/documents/document-view.test.ts`, `image-zoom.test.ts`, and
  `diagram.test.ts`: capabilities, mode policy, loading/failure decisions, zoom,
  and diagram errors.
- `web/src/features/search/*.test.ts` and
  `web/src/features/explorer/filename-filter.test.ts`: matching, globs, scopes,
  result models, cancellation, and worker-controller recovery.
- `web/src/features/source-control/*.test.ts`: compare routes, branch/pane
  models, layout memory, hunk selection, viewed hashes, and worker lifecycle.
- `web/src/workspace/*.test.ts`, `web/src/settings/*.test.ts`, and
  `web/src/features/projects/project-picker.test.ts`: stored state validation,
  theme aliases/selection, commands, tabs, and project choices.
- `web/src/highlighting/*.test.ts` and `web/src/styles/themes.test.ts`: every
  registered language, source-row preservation/escaping, and all theme tokens
  and contrast checks.

Removing an interaction variant is not equivalent to proving it with a unit
test. The table distinguishes retained integration coverage, existing cheaper
policy coverage, and intentionally omitted cosmetic/secondary checks. Add a
browser regression only when its failure depends on browser behavior that
these lower layers cannot exercise.
