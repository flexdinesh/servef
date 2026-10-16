# 0018: LSP-only navigation and host-specific availability

- Status: Accepted
- Date: 2026-10-16
- Supersedes navigation architecture in [0015](0015-code-navigation.md),
  [0016](0016-navigation-memory.md), and [0017](0017-language-servers.md)

## Context

Name matching cannot reliably resolve imports, re-exports, aliases, or typed
members. An import binding in `packages/domain/src/errors.ts:2` in Stash
resolved to itself rather than the nearby declaration. The UI then requested
references and received none. A direct TypeScript LSP request resolves both
the import and its use to `newsletters/content.ts:7` and returns 22 references.

The optional LSP layer still depends on the syntax navigator for eligibility,
fallbacks, response types, and snippets. Keeping two resolvers produces
inconsistent answers and retains parser/index complexity and memory costs.
Availability also disappears for languages without the built-in navigator,
and install instructions currently mix platforms indiscriminately.

## Decision

- Installed language servers own all symbol resolution. Remove tree-sitter,
  local binding/import rules, symbol indexes, proximity ranking, grammar
  unloading, parser memory timers, and the parser-specific Go heap limit.
  Keep gitignore marking and content-search behavior from 0015.
- `document` owns presentation, formatting, and search text. Remove its
  `Navigator` capability and `Type.Navigable`. Web composes page-level
  `navigationSupported` from registered text families and the LSP catalog.
  No frontend extension detection or document-handler LSP implementation.
- `lsp` owns server descriptions, discovery, host-specific installation
  guidance, stdio JSON-RPC, synchronization, capabilities, and process
  lifecycle. Web owns safe indexed-file reads, project confinement, HTTP
  contracts, result filtering, and bounded snippets.
- Enable language servers by default in the browser. Preserve explicitly
  stored on/off settings; an absent setting adopts the new default. Retain
  `X-Servef-Language-Servers: on` on navigation requests so cross-origin
  requests cannot start processes without a preflight. Off means navigation
  is disabled. Other browser sessions can continue using shared servers.
- Start one server per project root and selected server on the first
  navigation request. Page loads, pointer hover, and status checks never
  start servers. Stop after 10 idle minutes and on application shutdown.
  Bound startup and query waits, preserve cancellation, and avoid idle
  expiry during active requests. Failures have bounded backoff; Check again
  refreshes discovery and permits a retry.
- Support both native TypeScript LSP and the tsserver adapter. Discover
  project-local native TypeScript 7 deliberately; do not run arbitrary
  `tsc` executables as language servers. Classic adapter install guidance
  pins a tsserver-bearing TypeScript release. Expand the explicit catalog
  to existing code families with suitable standard servers. Method support
  comes from capabilities; the existence of an LSP does not promise
  definition or reference support for every language.
- Honor UTF-16 positions, negotiated synchronization and method capabilities,
  and Location/LocationLink ranges. Keep the open-document set bounded and
  invalidate stale overlays when files change on disk. Include workspace
  configuration changes as well as source changes in synchronization.
- Keep the definition/references endpoints. Successful empty results mean
  the server answered with no locations. Missing, disabled, starting,
  failed, timed-out, and unsupported requests have explicit error codes;
  never replace them with guessed names or import locations.
- Navigation remains Source-only. Inputs and results must be indexed,
  in CLI scope, non-ignored, readable text, and at most 4 MiB. Servers may
  read dependencies and configuration to understand the project, but web
  never exposes out-of-scope locations. Report filtered results and response
  truncation. Keep caps of 50 definitions and 1000 references. Remove the
  minified-source heuristic, which existed to protect local parsing.
- Preserve shortcuts, file history, result selection, source selection,
  and click-on-definition reference lookup. LSP positions and ranges decide
  targets; identifier scanning is cosmetic feedback only. Cancel obsolete
  requests on document, project, mode, or setting changes.
- Show availability beside the bottom-right language label for text files,
  including unsupported languages. Distinguish unsupported, missing,
  disabled, available/stopped, starting, ready, failed, and unknown status.
  A file blocked by policy has an explicit reason. Status polling does not
  count as navigation activity or keep a server alive.
- Generate installation guidance on the servef host from OS, Linux distro,
  known package managers, and runtime prerequisites. Send vetted commands,
  labels, and documentation to the UI. Offer copy and Check again. Never
  infer the host from the browser or execute installers from the UI.

## Consequences

- Browsing remains one self-contained Go binary with embedded local assets.
  Optional navigation requires an installed server and its language runtime.
  No server installation or network dependency is added to ordinary browsing.
- Navigation uses the selected server's project configuration, build context,
  language version, and supported features. This improves semantic accuracy;
  it does not guarantee identical results across every editor/server setup.
- servef loses parser/index memory and binary weight. Language-server memory
  remains additional and potentially substantial. Measure servef and server
  process-tree RSS separately; the status panel continues exposing server
  memory.
- Without an installed compatible server, navigation is unavailable with
  actionable status. This is an intentional replacement for silent fallback.
- The embedded UI and Go transport contracts change together; runtime
  validators, API fixtures, deterministic browser tests, and real-server
  smoke tests cover the boundary.

## Validation measurements

On Stash with matching stripped Linux builds, the old binary measured
48,263,328 bytes. The LSP-only build measured about 39.3 MB. Opening the same
source file used about 30 MB RSS before and 28 MB after. After definition and
reference requests, syntax navigation peaked at about 166 MB and settled at
47 MB after four idle seconds; the LSP-only servef process stayed around
28 MB, with the native TypeScript server consuming about 280 MB separately.
These are local observations, not fixed limits or guarantees.

The real TypeScript check resolved both `errors.ts:2` and `errors.ts:7` to
`newsletters/content.ts:7`, returned 22 references, and filtered the dependency
definition for `ZodError`. A portable repository-local native TypeScript test
also covers re-exports, aliases, and exclusion of shadowed locals.
