---
name: mygo-parity
description: Bring a mygo native app to parity with the web app it mirrors, one feature at a time, with headless eyes and hands (a ui.Tester session over mygo's automation protocol, mygo-agent, YAML scenarios, web|native composites). Use when porting a web UI feature to a mygo (github.com/egoist/mygo) native app, when checking how the native app looks or behaves, or when driving a mygo app without a display.
---

# mygo parity loop

The web app is the source of truth. The native app mirrors its behavior,
not its DOM: each feature is ported, tested with `ui.Tester`, captured
headless, and compared with the web side at the same viewport.

## Tools

All of them speak mygo's automation protocol (`plugins/automation`:
NDJSON over a Unix socket; `windows`, `tree`, `act`, `input`, `menu`,
`capture`, `idle`), so they drive a headless session and a built app alike.

- **Headless session** (`agent/session`): hosts any `ui.View` in a
  `ui.Tester` (CPU rasterizer, no display). Its tree is built from
  `Tester.Texts` and `Tester.Find`: one `text` node per text, the box of the
  first element showing it; repeated texts are listed but only the first is
  pressable; open context-menu items show as `menuItem`. Extra methods:
  `resize`, `setDark`, `advance`, `shutdown`. Hosts give it a view factory
  and use the session as their runner (`Go`, `Post`) or add pumps; `idle`
  waits for both and for pending frames.
- **mygo-agent** (agent-browser for mygo):

  ```sh
  mygo-agent open -- <host binary> [args]   # host serves at $MYGO_AGENT_SOCKET (also sets MYGO_AUTOMATION)
  mygo-agent open --attach <socket>         # a running session or built app
  mygo-agent snapshot -i                    # @eN refs, role, label, bounds
  mygo-agent click @e3 | click "README.md" | click 120,48 [--right|--double]
  mygo-agent fill @e5 "text"; type "abc"; key CmdOrCtrl+F; scroll 300 --at @e7
  mygo-agent resize 1280 800; dark on; idle; screenshot out.png; close
  mygo-agent run scenario.yaml --out <dir>
  ```

  Every action waits for `idle`. Refs come from the last snapshot; take a
  new one after the UI changes. `--session <name>` keeps several open.
- **Scenarios** (`agent/scenario`): YAML steps (`click`, `fill`, `type`,
  `key`, `scroll`, `resize`, `dark`, `wait`, `settle`, `expect`, `shot`)
  plus an optional `web:` section (localStorage with `${theme}`, and a path
  and steps per shot; web clicks may find `by: label|role|testid|css`).
  Runs write `<out>/<scenario>/<shot>.png`, `tree.json`, `scenario.json`.
  Run them from `go test` (`scenario.Run` with a `client.Client`) or the CLI.
- **mygo-compare**: `web` captures the web shots with agent-browser at the
  scenario's size and appearance; `dir -diff <out>/<scenario>` writes
  `compare/<shot>.png` (web | native | heat map) and the changed-pixel
  ratio. The ratio is a hint, not a gate: layouts differ by design.
- **Real app**: build with `-tags e2e` importing
  `github.com/egoist/mygo/plugins/automation`, run with
  `MYGO_AUTOMATION=<socket>` on a display (Xvfb + Mesa + GTK 3 + dbus on
  Linux), and drive it with the same CLI. Use it for what the headless tree
  cannot show: real roles and keys, GTK menus, IME, window chrome.

## The loop

1. **Pick a row** of the parity matrix: the lowest open milestone, status
   `missing` or `partial`. One milestone per branch and PR.
2. **Read the web behavior**: the row's web files, their tests, and the
   decision records they cite. Note states (empty, loading, error, large,
   binary), keyboard bindings, persistence, and what the server decides
   (capabilities, not file names). Run the web app on the shared fixtures
   and drive it with agent-browser to see it.
3. **Write the scenario first**: native steps to reach each state, a
   `shot` per state, and the `web:` side reaching the same shots. Include
   light and dark when colors matter.
4. **Implement** in the native app's existing layering: pure view models
   (no mygo import) hold decisions; the mygo view layer only draws and
   turns input into model calls. Mirror the web's pure logic file by file
   and test it the same way. Never re-derive what the server provides.
5. **Semantic tests with `ui.Tester`**: behavior, not pixels: click, type,
   assert texts, model state, and requests the fake server saw. Cover the
   states from step 2. Keep the scenario itself in a test that runs every
   scenario headless so it never rots.
6. **Shots**: run the scenario headless; read every PNG. Fix what looks
   wrong before comparing.
7. **Compare** with the web side; read each composite in light and dark.
   List the differences: intended (platform conventions, documented in
   the decision) or gaps (fix, or record as a known gap).
8. **Review**: hand a reviewer the diff, the composites, and the web files,
   not your conclusions. Fix small findings yourself.
9. **PR**: update the matrix rows (status, scenario, PR), the decision's
   known gaps, and docs; validate as the repository requires; open the PR
   with the composite paths and what still differs.

## Rules

- Shots are artifacts, never committed goldens.
- Port behavior from the web, never DOM structure; keep the native app's
  boundaries (it reaches the server only through its client package).
- Work around framework gaps in the repository's own packages; never patch
  the Go module cache.
- Use the fixtures the web app's mock mode serves so both sides show the
  same data.
- When the headless tree is not enough (unlabeled icons, duplicate texts),
  click by position from the screenshot, give the element a text or label,
  or use the real-app path.

## Repo config (servef)

- Native app: `apps/native` (Go 1.27.3 module outside `go.work`; run Go
  with `GOWORK=off go -C apps/native ...`). Tooling module:
  `packages/mygo`. Web app: `apps/web/src`. Desktop bridge and ssh:
  `apps/desktop/src/main`.
- Matrix: `docs/native-parity.md`. Decisions: `docs/decisions/0082`
  (the native app), `0086` (this tooling and the port plan).
- Host: `apps/native/internal/agenthost` (`cmd/native-agent`, tag `agent`)
  runs the real app view on `testdata/api/default` via `fakeservef`; the
  web side is `pnpm dev:mock` on the same fixtures.
- Scenarios: `apps/native/scenarios/*.yaml` (run by
  `internal/agenthost` tests in `mise run test:native`).
- Commands: `mise run native:agent` (serve headless at
  `.dev/native-agent/agent.sock`), `mise run native:shots -- <scenario>`,
  `mise run native:compare -- <scenario>`, `mise run test:native`,
  `mise run test:native:e2e` (the built app under Xvfb in Dagger; also
  `native:e2e`). Outputs: `.dev/native-shots/<scenario>/`.
- Validation and PRs: follow `AGENTS.md` (`mise run check:pr`, then
  `mise run check:status` before pushing).
