# 0070: Desktop ssh connections

- Status: Accepted; links, resolution, confirmation, and Connect page revised by [0076](0076-remote-servers-over-http.md); install instructions (the install script, no Homebrew or `go install`) by [0079](0079-releases-edge-prerelease-and-install-scripts.md)
- Superseded by: [0095](0095-desktop-app-is-mygo-electron-removed.md): Electron and its ssh connections are removed
- Date: 2026-11-02
- Implements: [0064](0064-ssh-remotes.md) Transport, Host identity, Missing
  or mismatched srcz, Notifications; [0062](0062-desktop-app.md) ssh
  connections
- Revises: [0067](0067-desktop-shell-and-protocol-proxy.md) (connection
  kinds, per-origin windows and navigation, local-only bridge,
  per-connection subscriptions, the `app` pages host); [0064](0064-ssh-remotes.md)
  (app-owned ControlMaster, runtime forward, slug origins)
- Consumes: [0069](0069-srcz-remote-and-ssh-links.md) (ssh link grammar,
  `testdata/remote-links.json`)

## Context

0069 prints `srcz://ssh/<host>/…` links on the host. The app must turn one
into a connection to that host's daemon without prompting for secrets,
without touching the user's own SSH sessions, and without ever passing a
link string to a shell. The M3 app knew one connection (`local`), one window
origin, and one subscription set.

## Decision

### Connections

- Kinds: `daemon` (`local`), `ssh` (`ssh-<slug>`), and the app pages host
  `app` (`srcz-app://app/connect`, served by the app, no upstream).
- Known ssh hosts live in `userData/connections.json`
  (`{"version":1,"ssh":[{id,destination,port,hints,addedAt}]}`, 0600, atomic
  write). A malformed or unknown-version file is renamed to
  `connections.json.invalid` and the app starts empty.
- `id` is assigned once and never changes, so the origin and its
  localStorage survive: `ssh-` plus a slug of the destination (lowercase,
  runs of `[^a-z0-9]` to `-`, at most 40 characters), plus `-` and 4 hex of
  sha256(destination) on collision. Chromium lowercases standard-scheme
  hosts and aliases may hold `_` or uppercase, so origins use the slug, not
  0064's raw `ssh-<alias>`.
- `destination` is an ssh config alias or `user@host` (no leading `-`, `%`,
  whitespace, quotes, or `/`); `port` is 1 to 65535 or null.

### ssh invocation

- The system `ssh` (`ssh` on `PATH`, else `/usr/bin/ssh`), argv only, never
  a local shell; `--` always precedes the destination. Environment: the
  app's plus `SSH_ASKPASS_REQUIRE=never`. `BatchMode=yes` everywhere: no
  password, keyboard-interactive, OTP, or host-key prompt ever.
- `ssh -V` preflight: missing (`sshMissing`) or OpenSSH older than 6.7
  (`sshTooOld`); a passing result is cached for the app run.
- One app-owned ControlMaster per connection, never the user's:
  `ssh -M -N -o ControlMaster=yes -o ControlPath=<run>/c-<h12>
  -o ControlPersist=no -o BatchMode=yes -o ConnectTimeout=15
  -o ServerAliveInterval=15 -o ServerAliveCountMax=3
  -o ClearAllForwardings=yes -o StreamLocalBindUnlink=yes
  -o StreamLocalBindMask=0177 -o ForwardAgent=no -o ForwardX11=no
  -o PermitLocalCommand=no -o RemoteCommand=none -o RequestTTY=no
  -o LogLevel=ERROR [-p port] -- <destination>`. The user's config applies
  (ProxyJump, keys, agent, `StrictHostKeyChecking`, `SetEnv`) except its
  forwards.
- Mux clients: `-o ControlMaster=no -o ControlPath=<same> -o BatchMode=yes
  -o ProxyCommand=false -F none`. `-F none` keeps the user's `LocalForward`
  entries from being pushed into the master by any mux client;
  `ProxyCommand=false` stops a session client from silently opening a
  direct connection when the master is gone. Control operations:
  `-O check|exit|forward|cancel`; remote commands add `-n -T`.
- Remote commands are compile-time constants; nothing from a link, alias,
  path, or remote output is interpolated:

  ```sh
  sh -c 'PATH="$PATH:$HOME/.local/bin:${GOPATH:-$HOME/go}/bin:$HOME/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin:/usr/local/bin"; export PATH; command -v srcz >/dev/null 2>&1 || { echo "srcz-missing $(uname -s) $(uname -m)" >&2; exit 127; }; exec srcz daemon ensure --json'
  ```

  The stop command is the same with `exec srcz daemon stop`. Existing
  `PATH` entries win; the appended directories cover Homebrew, `go
  install`, and `~/.local/bin` for non-interactive sessions. The remote
  environment comes from the master's session (`SetEnv` on a mux client is
  ignored).
- The remote socket from ensure is used in `-L` only when absolute, at most
  107 bytes, and free of `:`, whitespace, control characters, and `%`.

### Local runtime files

- A private directory per userData: Linux `$XDG_RUNTIME_DIR/srcz-desktop-<h8>`
  (absolute only) else `<tmpdir>/srcz-desktop-<h8>`; macOS
  `$TMPDIR/srcz-desktop-<h8>`; `/tmp/srcz-desktop-<uid>-<h8>` when a
  ControlPath plus OpenSSH's 17-byte suffix would not fit `sun_path`
  (Linux 107, macOS 103) or the path holds `%` or `:`.
- Created 0700 or verified with `lstat`: a symlink or another owner is
  refused (`runtimeDir`); group and other bits are tightened. Files:
  `c-<h12>` (ControlPath) and `f-<h12>.sock` (forward), `<h12>` = 12 hex of
  sha256(id).
- ssh leaves the forward socket behind after `-O exit`, SIGTERM, and
  `-O cancel`, so the app unlinks it itself, and the master keeps
  `StreamLocalBindUnlink=yes`. On startup (single instance, everything is
  stale) the app runs `-O exit` against each `c-*` and unlinks `c-*` and
  `f-*.sock`, only those patterns, only in that directory; ssh connections
  wait for that sweep.

### Lifecycle

- Connect: preflight, spawn the master, wait for the ControlPath (25 s;
  master exit first is classified from its stderr), `-O check`, ensure over
  the mux (20 s), validate protocol 1, unlink the stale forward socket,
  `-O forward -L <local>:<remote>`, probe `GET /_daemon/status` over it
  (5 s), ready.
- Re-ensure when a forwarded request gets no response head (refused,
  missing, reset): ensure again over the mux; a changed remote socket
  cancels the old forward and adds the new one. GET and HEAD retry once,
  others answer 503 `daemonRestarted` (as local).
- A connection is held while any window shows its origin and closes 30 s
  after its last window (or after it started, if no window ever held it):
  `-O exit` (2 s), SIGTERM if alive, unlink both files. Quit does SIGTERM
  and synchronous unlinks.
- Transient failures (`unreachable`, `failed`, the master dying after ready)
  retry with backoff from 1 s doubling to 30 s; the window shows a
  reconnecting page that refreshes itself. Permanent kinds (`sshMissing`,
  `sshTooOld`, `hostKey`, `auth`, `srczMissing`, `srczTooOld`,
  `mismatch`, `forwardDenied`, `runtimeDir`) stop until a reload retries.
- The remote daemon cannot idle while connected: the app holds its
  `/_daemon/events` stream. After disconnect it exits on its own idle timer
  (default 30 minutes; the app passes no idle flag).

### Failures

Pages reuse the failure page (own CSP, no scripts). Instructions are shown
text, never run:

| Kind | Signal | Page |
| --- | --- | --- |
| `sshMissing`, `sshTooOld` | `ssh -V` cannot run, or older than 6.7 | install or upgrade the OpenSSH client |
| `hostKey` | `Host key verification failed`, `REMOTE HOST IDENTIFICATION HAS CHANGED`, `No <type> host key is known` | run `ssh <dest>` in a terminal once, then Retry; never offers to accept keys |
| `auth` | `Permission denied (`, `Too many authentication failures` | load a key with `ssh-add`; make `ssh -o BatchMode=yes <dest> true` succeed |
| `unreachable` | resolve, refused, timeout, no route, unreachable network, ControlPath wait timeout | detail; retried while held |
| `srczMissing` | exit 127 with `srcz-missing <os> <arch>` | Homebrew, `go install`, or the release archive for that arch into `~/.local/bin`; the PATH directories; "The app never installs anything" |
| `srczTooOld` | `unknown command "daemon"`, `unknown flag: --json`, `unknown shorthand flag` | upgrade srcz on the host |
| `mismatch` | status protocol is not 1 | upgrade the older side; "Stop the running daemon and retry" runs the stop command |
| `forwardDenied` | inferred: ensure succeeds, the probe gets no response head (reset or broken pipe), re-ensure returns the same socket, the probe fails the same way again | `AllowStreamLocalForwarding yes`, no `DisableForwarding`, OpenSSH 6.7+ on the host |
| `runtimeDir` | private directory refused | path and reason |
| `failed` | anything else | stderr tail (2 KB); the remote `<socket>.log` when known |

`forwardDenied` cannot be read from stderr: at `LogLevel` INFO and above a
denied forward and a missing remote socket both print `channel N: open
failed: connect failed: open failed`, and at ERROR nothing.

### Links, resolution, confirmation

- `srcz://ssh/<host>/<route>?…` parses per 0069; `ssh-*` keys are
  stripped before the web query reaches the renderer. `<host>` may be any
  non-empty decoded text (a host name with spaces still parses); only
  resolved destinations must be valid.
- Resolution: a known connection whose destination, id slug, or remembered
  hint hostname equals `<host>`; a known connection with matching
  remembered hints (`hostname` and `user`, or `addr`, port, and `user`);
  else candidates from `~/.ssh/config` `Host` aliases (and `Include`s;
  patterns and `Match` blocks skipped) equal to `<host>` or whose `ssh -G`
  hostname equals `<host>`, `ssh-hostname`, or `ssh-addr` (excluding
  aliases whose user or port differ from the hints); else `user@addr` and
  `user@hostname`. `ssh -G` runs only on aliases read from config (at most
  64, four at a time, 3 s each).
- First contact is always confirmed in a native dialog listing at most four
  choices with their `ssh -G` user, hostname, and port, plus "A link asked
  srcz to connect. srcz runs srcz on that host with your SSH
  configuration. Connect only to hosts you trust." The choice is stored
  with the link's hints, so it is asked once. Cancel stores nothing.
- Known-host links never prompt, as local links. An unknown `ssh-*` origin
  is 404 at the proxy, so a page cannot reach an unconfirmed host.

### Windows, navigation, titles, bridge

- Windows key by origin, then project; a window keeps its first origin for
  life. Navigation to another `srcz-app://` origin routes to a window of
  that origin only for `local`, `app`, and known ssh ids.
- Titles: `<page> - <dest> (SSH)`, `<page> - <dest> (SSH, reconnecting)`
  while not ready, and `srcz - Connect`.
- The preload bridge (0067) exists only in `srcz-app://local` windows and
  main honors Open Folder only from them. Remote roots come from `srcz
  remote` on the host; the UI hides Open Folder by feature detection.
- Copied agent instructions in a remote window name `srcz://ssh/<slug>`.

### Subscriptions and notices

- One subscription set per daemon-like connection (local and each ready
  ssh connection), fed the projects its origin's windows show (16 watched
  per connection). ssh sets start on ready and stop when the connection
  closes.
- Notices carry their connection; targets use its origin; the repeat gate
  keys `(connection, project, kind)`; focus suppression compares connection
  and project. Remote bodies read `<project> on <dest>`. Walkthroughs
  published on the host arrive through its control stream.
- Notifications from SSH hosts arrive only while a window for that host is
  open; the Connect page and docs say so.

### Connect page

`srcz-app://app/connect` (File > Connect…): static HTML, CSP
`default-src 'none'; style-src 'unsafe-inline'; form-action 'self'`, no
scripts. It lists the connections with their state (links
`srcz://ssh/<slug>/`; a Forget POST form per ssh host that closes the
connection and removes it) and the config aliases not yet known (links
`srcz://ssh/<alias>/`, confirmed on first contact). State changes are POST
only, behind the proxy's Origin guard.

### Dev and test seams

`SRCZ_DESKTOP_SSH`, `SRCZ_DESKTOP_SSH_CONFIG` (master and alias lister
only), `SRCZ_DESKTOP_RUNTIME_DIR`, and `SRCZ_DESKTOP_AUTO_CONFIRM=1`
with `SRCZ_DESKTOP_DIALOG_LOG` apply only to unpackaged builds.

## Spike

Linux, OpenSSH 10.6p1 client against 10.6p1 (fish login shell) and 9.2p1
(Debian bookworm) servers:

- `-O forward -L <sock>:<sock>` through a `ClearAllForwardings` master works
  only with `-F none` mux clients; without it, any mux client pushes the
  user's `LocalForward` entries into the master, and a mux client with
  `ClearAllForwardings=yes` binds nothing. A second `ssh -N -L` process has
  the same flaw and is not used.
- BatchMode fails in 60 to 120 ms for unknown, changed, and `ask` host keys
  and for a wrong key; no hangs.
- SIGTERM and `-O exit` remove the ControlPath; none of SIGTERM, `-O exit`,
  or `-O cancel` removes the forward socket.
- SSE streams incrementally through the forward and closing it ends the
  remote request.
- The constant ensure command passes through sh and fish login shells
  unchanged; the fallback directories find srcz.
- Unverified: a Finder-launched macOS app seeing `SSH_AUTH_SOCK` and
  `/usr/bin/ssh`, and macOS `open-url` for ssh links.

## Consequences

- BatchMode excludes password, keyboard-interactive, and OTP logins: hosts
  need agent or unencrypted keys and a host key verified in a terminal.
- Not reusing the user's ControlMaster costs one extra connection and
  authentication per host (noticeable with 2FA bastions).
- `srcz remote` and the app's ensure must resolve the same remote socket
  (same `XDG_RUNTIME_DIR` in interactive and exec sessions); otherwise the
  link's project shows unavailable until that daemon restarts with the
  persisted root.
- A malicious or compromised host controls content in its own origin only:
  isolated storage, no bridge, the Origin guard, denied permissions, and
  the navigation policy.
- An app killed with SIGKILL leaves masters alive until the next start's
  sweep or a network drop; they hold no requests, so remote daemons still
  idle out.
- `ssh -G` evaluates the user's `Match exec` lines, bounded by alias count
  and timeouts.
- Memory: one ssh master per connection and no extra renderers beyond
  windows; remote daemon memory is on the host. Measured on Linux (OpenSSH
  10.6p1, connected and idle 30 s): ssh master RSS 9.8 MiB. Electron total
  with a remote window is unmeasured (no display on the measuring host).
