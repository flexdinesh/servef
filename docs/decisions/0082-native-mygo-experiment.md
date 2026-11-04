# 0082: Experimental native app on mygo

- Status: Accepted (experimental); revised by [0085](0085-image-metrics-and-navigation-in-the-application-layer.md) (image reads through `pkg/client`); known gaps scheduled by [0086](0086-native-agent-tooling-and-ports.md)
- Date: 2026-11-03
- Relates to: [0077](0077-desktop-shell-stays-on-electron.md) (Electron stays
  the desktop shell), [0078](0078-apps-and-packages-layout.md) (outside
  `go.work`), [0079](0079-releases-edge-prerelease-and-install-scripts.md)
  (the native extension point), [0080](0080-application-layer-contracts-and-transports.md)
  (contracts and the Go client), [0061](0061-daemon-roots-and-leases.md),
  [0066](0066-lease-levels-and-desktop-events.md),
  [0071](0071-cli-server-connections-and-pasted-urls.md),
  [0076](0076-remote-servers-over-http.md),
  [0083](0083-server-structured-diffs.md) (structured diffs),
  [0084](0084-in-process-client-package.md) (`pkg/client` links no server)

## Context

0077 kept Electron after a mygo shell that hosted the web UI in WebKitGTK
lost on memory, CPU, and request handling. A native UI is a different bet:
draw servef with mygo's own widgets, no web view, as one more client of the
contracts 0080 introduced. It can only be judged by building it, so it ships
as an experiment beside the Electron app and does not replace it.

## Decision

### Scope (milestone M1)

- Connections: the local daemon over its Unix socket, and CLI or remote http
  servers over TCP.
- Projects and a virtualized explorer (`ui.Outline`).
- A read-only source view: mygo's code editor with line numbers, find, and
  copy, highlighted by Chroma.
- Markdown preview (`markdown.View`) with document links, headings, external
  links, and images; Mermaid shows as code with a caption.
- Live updates from the event stream.
- Source Control review of uncommitted changes: a Changes sidebar
  (`Client.Changes`) and a native diff viewer for one file (`Client.Diff`),
  split or unified, hunks or the full file, ignoring whitespace on request.
  Commit history and branch or range compares are not in M1.

### Boundaries

- `apps/native` is its own Go module (`go 1.27.3`, mygo v0.7.4) outside
  `go.work` (0078). From servef it imports only `pkg/contract` and
  `pkg/client` (`replace github.com/flexdinesh/servef => ../servef`). Its
  module path sits under servef's, so Go's internal rule would allow more; a
  boundary test fails on any other servef import. Since 0084 those two link
  no other servef package (`go list -deps`), so the binary carries neither
  the server nor the web app and builds without `internal/web/dist`.
- View models are pure Go with no mygo import: `link` (links and arguments),
  `tree`, `docview` (modes and views), `mdlink` (Markdown link and image
  resolution), `highlight` (Chroma to servef's eight syntax categories),
  `theme` (servef's tokens), `live`, `connect`, `review` (diff layout), and
  `workspace` (the app state). `view` is the thin mygo layer.
- The web UI stays the source of truth. `docview` mirrors
  `document-view.ts` (Source and Preview; no Format), `theme` mirrors
  `tokens.css` and its test parses that file, and `highlight` maps every
  language in `testdata/document-families.json`, checked by a test. Server
  capabilities decide views; the app never detects file types itself.

### Review on the structured diff contract

- Diffs render only the server's `lineDiff` (0083). `review` lays out its
  hunks, blocks, and gaps as rows over the two sides' lines, as
  `diff-metadata.ts` does for Pierre; it splits sides after each `\n`, as the
  server does, and never compares texts.
- mygo's `editor.DiffView` diffs its two buffers itself. Native instead hands
  it the rows through `DiffState.SetPatch` (an `editor.FilePatch` value,
  rows marked ` `, `-`, `+`), whose change runs are taken as given; no patch
  text exists or is parsed. The full-file view is one hunk with the gaps as
  unchanged rows; the hunk view captions each hunk with the unmodified lines
  before it. File names stay empty so mygo detects no language; both sides
  are highlighted from their full files in the server's language.
- Word emphasis inside changed lines is mygo's (word-alt), as Pierre's is in
  the web UI: presentation, not the diff.
- Live updates reread the change list on any path or git change, and the
  open diff when its path, its rename source, or git changes (`live.Diff`).
- Tests use `testdata/api/default` (`changes.json`, `diff-*.json`), whose
  line diffs the server's tests check against `internal/diff`.

### Connections and links

- The daemon is ensured with `servef daemon ensure --json` from the bundled
  binary (`resources/bin/servef`; `SERVEF_NATIVE_BINARY` then `PATH` in
  development) and must speak protocol 1. `servef control list --json`
  lists loopback CLI servers. Any server opens by address.
- Scheme `servef-native://`, mirroring `servef://`:
  `servef-native://local/?project=&path=` (the daemon) and
  `servef-native://<host>:<port>/?project=&path=` (a server). Non-loopback
  servers are confirmed once per host and port per session (0076).
- One instance per user (mygo's lock): links and `servef-native [path]` reach
  the first instance; Linux passes links as arguments, macOS as Apple Events.
  Paths open through `/_daemon/open` (`Client.DaemonOpen`).
- The servef CLI knows nothing about the native app; `servef [path]` still
  opens the Electron app or a browser (0068).
- Markdown images read `/api/image` over the connection's transport:
  root-confined and image types only. `pkg/client` has no image read yet, so
  this one request is made beside it.

### Distribution

- `build.yml` jobs `native-linux` (cross-compiles linux amd64 and arm64 with
  `mygo build`; mygo needs no cgo) and `native-macos` (darwin amd64 and arm64
  on `macos-latest`, ad-hoc signed) bundle the matching CLI archive's
  binary and upload `servef-native_<version>_linux_<arch>.tar.gz` and
  `servef-native_<version>_darwin_<arch>.zip`. Go 1.27.3 comes from
  `GOTOOLCHAIN`, verified against sum.golang.org. `apps/native/scripts/package.sh`
  holds the steps.
- `scripts/install-native.sh` (latest) and `install-native-edge.sh` (edge)
  install the app only: Linux in `~/.local/share/servef-native` with a desktop
  entry for `servef-native://` and an icon, macOS `servef-native.app` in
  `/Applications` (or `~/Applications`); both link `servef-native` into
  `~/.local/bin` (or `~/bin`). `uninstall.sh` removes it and keeps its data.
- Docs label it experimental.

### Validation

- `mise run test:native` is opt-in, never part of `check:pr`: a `native`
  Dagger group outside the eleven, with its own preparation (the Go 1.27.3
  tarball, sha256-checked). It runs shellcheck on `apps/native/scripts`,
  `go mod tidy -diff`, `go vet`, and `go test -race` (headless `ui.Tester`
  view tests and view-model unit tests against a fake servef over real
  TCP and Unix-socket transports).
- `fmt:check` covers `apps/native`; `tidy:check` does not, since the locked
  Go cannot load a Go 1.27 module.

## Graduation criteria

- macOS verified by hand: single instance, links as Apple Events, the
  bundled daemon, signing and notarization.
- Accessibility and IME verified on Linux and macOS.
- Memory and CPU measured against Electron with the 0077 scenario on real
  GPUs.
- A Mermaid decision (render natively, or keep code).
- An image read in `pkg/client`.

## Known gaps

- Mermaid shows as code.
- Review covers uncommitted changes only, one file at a time. Collapsed
  gaps do not expand in place (the full-file toggle shows them), there is no
  "no newline at end of file" marker, and line selection, links, and hunk
  steps are not wired.
- Chroma's tokens differ from Highlights' (0029): categories match, token
  boundaries and some classifications do not.
- Accessibility and IME on Linux are unverified; under Xvfb AT-SPI was not
  running.
- macOS is built and smoke-checked in CI only.
- No Format, search, code navigation, walkthroughs, or tabs.
- Xvfb samples (llvmpipe, no GPU), not comparable with 0077: 124 MiB RSS
  for the app and 21 MiB for the daemon with one Markdown preview open;
  120 MiB and 25 MiB with a 35-line Go diff open in the full-file view.

## Consequences

- The Electron app remains the desktop app; the native app is a separate
  artifact with its own scheme, so both can be installed.
- Every new server capability the native app shows must reach it through
  `pkg/contract` and `pkg/client`, which keeps 0080's single source of truth
  honest.
