# 0086: Native agent tooling, parity loop, and ports

- Status: Accepted; M1a (Highlights, Ondine) shipped as [0088](0088-native-highlights-and-ondine-via-wazero.md)
- Date: 2026-11-04
- Relates to: [0082](0082-native-mygo-experiment.md) (the native app; stays
  in force), [0078](0078-apps-and-packages-layout.md) (`packages/`),
  [0059](0059-validation-execution-boundaries.md) (validation),
  [0081](0081-desktop-bridge-v2-native-features.md) (bridge v2),
  [0070](0070-desktop-ssh-connections.md) (ssh)

## Context

0082 shipped a native app that covers a slice of the web UI. Closing the
rest is many milestones of porting, mostly by agents on machines without a
display. An agent needs to see the app, act on it, and compare it with the
web UI it mirrors, and later sessions need the queue and the method.

## Decision

### Tooling: `packages/mygo`

- A Go module of its own (`github.com/flexdinesh/servef/packages/mygo`,
  Go 1.27.3, mygo v0.7.4) outside `go.work`, generic for any mygo app: it
  imports nothing from servef (a boundary test enforces it). `apps/native`
  requires it through `replace => ../../packages/mygo` and may import it.
- `agent/session` hosts any `ui.View` in a `ui.Tester` (CPU rasterizer, no
  display) and speaks mygo's automation protocol (`plugins/automation`:
  NDJSON over a Unix socket made under umask 0177, then 0600; `windows`,
  `tree`, `act`, `input`, `menu`, `capture`, `idle`, same params, results,
  and error codes). Where a Tester cannot match a built app:
  - Its tree comes from `Tester.Texts` and `Tester.Find` (no public access
    tree): role `text`, stable IDs per text and occurrence, repeated texts
    listed but not pressable, open context-menu items as `menuItem`. Nodes
    carry no `key`; `act` by key answers `unsupported`, so clients act by
    id.
  - `menu` chooses from the open context menu (a Tester has no menu bar);
    `eval` is `unsupported`.
  - `idle` also waits for work in flight the host started with `Go`, its
    posted applies, and pumps, not only for frames: the fetch a click
    started has landed when it answers. A built app's `idle` waits for
    frames only, so clients wait for a text there (`mygo-agent wait
    --text`, scenario `wait:` steps).
  - It adds `resize`, `setDark`, `advance`, and `shutdown`, which built
    apps answer with `unknown_method`; scenarios skip those steps there and
    log it.
  It is the host's runner (`Go`, `Post`) and takes pump hooks.
- `agent/client` (Go client), `cmd/mygo-agent` (agent-browser-like CLI;
  refs from the last snapshot kept beside the socket; state in a 0700
  directory the user owns, files written without following symlinks;
  `open` refuses a socket that already answers unless `--replace`, and
  records a host only while it runs and serves), `agent/scenario`
  (YAML steps and named shots, with an optional web side), `agent/webshot`
  (the web side through agent-browser), and `cmd/mygo-compare` (labeled
  web | native composites at one scale, optional heat map).
- The headless tree is text based; real roles, keys, GTK menus, and IME
  need the built app, which links `plugins/automation` only in `-tags e2e`
  builds (`apps/native/agent_e2e.go`). Release builds never carry it.

### Servef wiring

- `apps/native/internal/agenthost` runs the real view (`view.App` over
  `workspace.Model`) in a session, against `fakeservef` serving
  `testdata/api/default` (the files `pnpm dev:mock` serves) by default, or
  a real daemon socket or server. `cmd/native-agent` (tag `agent`) is its
  command; `--fixtures-server` serves only the fixtures over TCP.
- Scenarios live in `apps/native/scenarios/`; a test runs every one
  headless in `mise run test:native`.
- mise: `native:agent`, `native:shots`, `native:compare`, and
  `test:native:e2e` (alias `native:e2e`): the e2e build under Xvfb, Mesa,
  GTK 3, and dbus in the Dagger `native` group, driven by `mygo-agent`,
  artifacts in the run's `native-e2e`. Shots go to `.dev/native-shots`.
- Validation: `test:native` also tidies, vets, and race-tests
  `packages/mygo` and vets the `agent` and `e2e` builds; `fmt:check` covers
  it; it is an input of the `repo` and `native` groups only (groups that
  read `packages/` now read `packages/contracts/`). Neither native task is
  in `check:pr`.

### Parity loop

- `docs/native-parity.md` is the queue: one row per web behavior with its
  files, decision, native status, milestone, scenario, and PR.
- `skills/mygo-parity/SKILL.md` (linked at `.claude/skills/mygo-parity`)
  is the generic loop: pick a row, read the web behavior, write the
  scenario, implement, `ui.Tester` semantic tests, shots, web | native
  compare, review, PR. Its last section holds servef's paths.
- One PR per milestone (M1 ports, M2 to M11 features, as the matrix lists).

### User decisions

- Highlighting and diagrams are ported, not reimplemented: Highlights
  (`@pierre/highlights`, WASM) and Ondine run under wazero in
  `packages/mygo`, pinned to the web app's lockfile versions; native drops
  Chroma. Trees and diffs get Pierre-like native widgets.
- No committed goldens: shots and composites are gitignored artifacts;
  tests assert behavior.
- Scope: the web UI, desktop bridge v2 (0081), and ssh connections (0070,
  under its argv-only, BatchMode, constant-command rules). Open at Login is
  excluded.

### 0082 known gaps this series closes

- Mermaid shown as code (Ondine, M1 and M4). Diagrams: M1a, 0088; expand
  and zoom stay M4.
- Review limited to uncommitted changes and one file; no in-place gap
  expansion, no-newline marker, line selection, links, or hunk steps (M1
  diffs, M8).
- Chroma's tokens differ from Highlights' (M1). Closed by M1a, 0088.
- No Format, search, code navigation, walkthroughs, or tabs (M2 to M9).

0082's graduation criteria (macOS by hand, accessibility and IME, memory
against Electron on real GPUs) stay open; the e2e path is where they will
be checked on Linux.

## Consequences

- Agents can run, see, and compare the native app on a headless machine.
- The tooling is mygo's, not servef's, so it can move out unchanged.
- Pixels are compared by eye per milestone, not gated: layouts differ by
  design, and the heat map only points at differences.
