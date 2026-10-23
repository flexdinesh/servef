# 0024: Source Control view and working-tree diffs

- Status: Accepted; diff line selection added by [0028](0028-diff-line-links-and-copy.md); Shiki highlighting superseded by [0029](0029-highlights-syntax-highlighting.md); view layout and diff tabs superseded by [0035](0035-compare-list-and-review-tab.md)
- Date: 2026-10-21
- Extends: [0020](0020-activity-bar-and-search-view.md)

## Context

servef shows files as they are on disk but not what changed since the last
commit. Reviewing local or agent-made edits meant switching to another tool.
VS Code lists uncommitted changes in a Source Control view and opens each as a
diff tab. servef never runs `git` (0023) and limits reads to indexed,
CLI-scoped files (0001).

## Decision

- The activity bar gains a third item, Source Control (Cmd/Ctrl+Shift+G),
  after Search. It shows a count badge (`99+` above 99). The view persists
  under `servef-sidebar` like the others.
- One change list compares HEAD with the working tree; staged and unstaged
  changes are not separated. Statuses: `modified`, `added`, `untracked`,
  `deleted`, `renamed`. Renames pair a deleted and an added path with
  identical content only. Type changes (file and symlink) report `modified`.
- Status is pure Go in `internal/git/`: go-git reads the HEAD tree, the index,
  and objects. Tracked entries reuse the index hash when size, mtime, mode,
  and the racy check match, and are hashed as git blobs otherwise. Untracked
  files come from a walk that applies servef's own 0023 ignore rules, so
  Source Control and Search agree on what git ignores. No `git` binary.
  go-git's `Worktree.Status` is not used: its ignore sources differ from 0023
  and it does not prune ignored trees such as `node_modules` before work.
- Source Control is repository-scoped, not CLI-scoped: `--filename`,
  `--depth`, and `--exclude` do not apply, so every change in the checkout
  is listed. This refines the indexed-read rule for this view only: worktree
  reads for changed paths go through `files.ResolveFile` (root-confined,
  symlinks evaluated, regular files only), and only after the path is
  validated against the change set (index, HEAD, lstat, single-path ignore
  check). Folder projects report no repository.
- Status is bounded: 5,000 entries, then `truncated`; a 20s timeout bound to
  the request context. Submodules and nested repositories are skipped.
- `GET /api/changes?project=` returns `repository`, `branch` (`""` when
  detached), path-sorted `files` (`path`, `status`, optional `oldPath`),
  `truncated`, and `warnings`. `GET /api/diff?project=&path=[&oldPath=]`
  returns the registry `documentType` for `path` plus `original` (HEAD,
  `null` when added or untracked) and `modified` (worktree, `null` when
  deleted). A path outside the change set is 404; an unchanged tracked path
  is 404 with `code: "unchanged"`.
- 0004 limits apply per side: `binary` sends no content; over 4 MiB sets
  `tooLarge` with no content and the diff is off; over 32 MiB is never read
  (size from stat or blob header). Symlink sides carry the link target text.
- Diffs open as tabs in the existing tab strip, labelled `name (Working
  Tree)` or `name (Deleted)`. URLs add `&diff=1`. Tab memory (0019) accepts
  legacy path strings and `{diff: path}` entries; diff tabs restore without
  checking the tree.
- `@pierre/diffs` renders diffs from client-side jsdiff. The parse runs in
  one shared module worker (`web/src/diff-worker.ts`), and Pierre's
  virtualizer renders only visible rows, with the document pane as its
  scroll root so keyboard scrolling and scroll memory keep working.
  Closing the last diff tab ends the parse and highlighting workers and
  clears Pierre's resolved grammars, themes, and the last diff response. Shiki highlights
  through a CSS-variable theme bound to `--color-syntax-*`, so every servef
  theme applies. The grammar comes from the server's `documentType.id`
  through an explicit family-to-Shiki map in `web/src/diff-language.ts`
  (`text` turns highlighting off); the UI detects no extensions. Families,
  not Chroma languages, key the map because TSX shares Chroma's TypeScript
  lexer but needs Shiki's TSX grammar. A worker pool of 2 highlights off the
  main thread. Grammars and workers are bundled locally.
- Only mapped grammars ship. `@pierre/diffs` imports Shiki's full bundle and
  every Pierre and Shiki theme; a build-only Vite plugin
  (`servef-diffs-subset`) resolves its `shiki`, `shiki/wasm`, and
  `@pierre/theming/themes` imports to local subsets: the grammars in
  `web/src/diff-grammars.ts`, no themes, and no Oniguruma WebAssembly.
- Every registered family needs an explicit grammar choice. Tests chain the
  registry to the bundle: `TestDocumentFamiliesFixtureMatchesBuiltins` keeps
  `testdata/document-families.json` equal to the `Builtins()` IDs;
  `diff-language.test.ts` requires each listed family in the map and each
  bundled grammar to be used; types require each mapped grammar to have a
  loader. Adding or removing a family updates all three; adding a grammar
  that imports others brings those along.
- The diff toolbar holds Split/Unified, Wrap, Ignore whitespace, Inline
  (Word/Character/Off), Expand unchanged, and Open File (disabled for deleted
  files). Options persist in localStorage `servef-diff-options`, not
  Settings. Below 48rem Split is disabled and Unified forced.
- Changes refresh on project load, window focus or visibility (throttled to
  2s), view activation, and Refresh. No polling.
- The Explorer decorates changed paths it shows with git status. Diff-tab
  footers show `+N −M` line stats and no line selection.

## Consequences

- No clean/smudge filters: LFS or `core.autocrlf` files may show as modified
  after their stat changes.
- Changes inside submodules and nested repositories are not shown. Renames
  with edits show as a deletion plus an addition.
- Source Control can list and diff files the CLI scope hides; reads stay
  confined to the project root and the validated change set.
- Diffs add about 3 MB to the binary (46.5 MB to 49.7 MB): 35 grammars and
  Pierre's renderer. Shiki's full bundle would add about 12 MB.
- Grammars that a mapped grammar imports, such as C++'s embedded languages,
  ship with it; the largest is C++ at about 0.8 MB.
- Large diffs no longer block the main thread, but a 3 MB side still takes
  seconds to parse; the footer shows Loading meanwhile. Pierre skips syntax
  highlighting past its tokenize limits.
- `@pierre/diffs` is pinned to 1.5.2 because pnpm `minimumReleaseAge` blocks
  newer releases for now.
