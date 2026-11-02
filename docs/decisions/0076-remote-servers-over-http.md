# 0076: Remote servers over http; ssh driven from the app

- Status: Accepted
- Date: 2026-11-02
- Supersedes: [0069](0069-servef-remote-and-ssh-links.md) (`servef remote`,
  ssh links, link hints, `testdata/remote-links.json`)
- Revises: [0063](0063-cli-commands-and-url-scheme.md) (scheme grammar; no
  `servef remote`), [0064](0064-ssh-remotes.md) (host identity; forwards to
  TCP servers), [0068](0068-cli-opens-desktop-app-and-desktop-releases.md)
  step 2 (no-UI machines), [0070](0070-desktop-ssh-connections.md) (links,
  resolution, confirmation, Connect page),
  [0071](0071-cli-server-connections-and-pasted-urls.md) (scheme form of
  server URLs; non-loopback servers)

## Context

0069 taught the CLI about ssh: `servef remote`, link hints, and a grammar
the app resolved to ssh destinations through heuristics and a first-contact
dialog. A link should say where a server is, not how to reach the machine,
and the CLI should not care how the user got to a terminal. ssh is a
convenience of the desktop app only. None of 0069 or 0070 has been released.

## Decision

### Links name an http server

- A server URL has two forms: `http://<host>:<port>/<route>?<query>` (web)
  and `servef://<host>:<port>/<route>?<query>` (desktop), the same URL with
  the scheme swapped. `servef://local/…` stays the local daemon.
  `servef://ssh/…` is gone.
- The CLI prints both forms as OSC 8 hyperlinks when stdout is a terminal
  and `TERM` is not `dumb` (0069's writer); the visible text is the plain
  URL either way.

### CLI

- No-UI machine: `SSH_CONNECTION`, `SSH_CLIENT`, or `SSH_TTY` is set, or on
  Linux neither `DISPLAY` nor `WAYLAND_DISPLAY` is. The CLI never builds ssh
  details into a link.
- Bare `servef [path]` decides: `--host` or `--port` set → browser mode; a
  no-UI machine → browser mode bound to the detected address, never opening
  anything; otherwise the 0068 daemon flow, unchanged.
- Detected address: the server IP of `SSH_CONNECTION` (`client_ip
  client_port server_ip server_port`) when it parses; else the local address
  of a UDP socket "connected" to a public address (nothing is sent); else
  `localhost` with a notice. It binds that one IP, never all interfaces.
  These servers have no authentication: anyone who reaches the address can
  read the served files. The startup output says so.
- `servef browser` keeps its loopback default everywhere and never opens the
  desktop app. In SSH it still never opens a browser.
- Browser-mode startup prints `web:` and `desktop:` lines. The local daemon
  flow prints `opened <project> in servef` and `desktop:
  servef://local/?project=…`; the daemon has no http listener, so there is
  no web URL.
- `servef remote` is removed. `servef walkthrough publish` loses its ssh
  link; with a CLI server it prints the web URL and the desktop URL.

### Daemon: directory listing

`GET /_daemon/directories?path=<absolute>` (empty `path`: the user's home
directory), behind the control API's browser-request guard:

```json
{"path": "/home/dee", "parent": "/", "entries": [{"name": "code", "path": "/home/dee/code", "repository": false, "hidden": false}]}
```

- `parent` is `""` at the filesystem root. Entries are directories and
  symlinks resolving to directories, sorted by name, without the
  always-skipped directories (`files.Excluded`), at most 2000 (`truncated:
  true` when more). `repository` marks a `.git` marker inside; `hidden` a
  leading dot.
- Errors (`{error, code}`): relative path 400 `invalidPath`; missing 404
  `pathUnavailable`; not a directory 400 `notDirectory`; unreadable 403
  `pathUnreadable`. Additive: protocol stays 1; an older daemon answers 404
  and the app asks to upgrade servef on the host.

### Desktop: links

- `servef://<loopback>:<port>/…` opens `cli-<port>` (0071), verified with
  the local `servef control list`.
- `servef://<other host>:<port>/…` opens kind `http`, id `http-<slug of
  host>-<port>`, upstream TCP to that host. First contact is confirmed in a
  native dialog once per host and port ("A link asked servef to open the
  server at <host>:<port>. It is not on this computer. Open only servers you
  trust."); the answer is stored in `connections.json`. Cancel stores
  nothing. `shareableOrigin` maps it back to `http://<host>:<port>`.
- No subscriptions or notices for `http` connections, as for `cli`.

### Desktop: ssh from the Connect page

- ssh starts only from the Connect page: hosts are config aliases plus known
  hosts. The user chose the host, so there is no confirmation dialog; known
  hosts store `{id, destination, port, addedAt}` without hints. The 0070
  transport stays: app-owned ControlMaster, BatchMode, constant remote
  commands, failure kinds, runtime directory.
- Host page `servef-app://app/ssh/<id>` (static, no scripts): the remote
  daemon's roots (from ensure), running remote servers (`servef control list
  --json`, a constant command behind the 0070 PATH wrapper), and Browse
  folders.
- Picker `servef-app://app/ssh/<id>/browse?path=` lists
  `/_daemon/directories` over the forward. "Open this folder" is a POST form;
  main posts `/_daemon/open` and opens `servef-app://ssh-<slug>/?project=…`.
  Paths travel in HTTP bodies over the forward, never in remote commands.
- A remote TCP server is kind `ssh-port`, id `ssh-<slug>-<port>`: the master
  forwards `<runtime>/p-<h12>.sock` to the listed host and port. When the
  forward is denied or fails, it falls back to direct `http://<host>:<port>`
  (kind `http`, no confirmation: the user chose it): the listed host when it
  is not loopback, else the `ssh -G` hostname. A loopback-only server has no
  fallback.
- Daemon folders need the forward: when it is denied, the `forwardDenied`
  page also links the remote servers reachable by IP.

## Consequences

- An IP-bound server on a no-UI machine is readable by anything that reaches
  its address; `servef browser` (loopback) plus a user-made port forward is
  the private option.
- A `servef://localhost:<port>` link printed on one machine and opened on
  another reaches whatever servef server runs on that port there; no-UI
  machines print their address to avoid this.
- macOS Terminal.app has no OSC 8; its users copy the desktop URL. tmux
  needs `terminal-features` with `hyperlinks`.
- The IP fallback changes a remote server's origin, so its localStorage does
  not carry over from the forwarded origin.
- Any web page can fire `servef://<host>:<port>` links; confirmation and
  isolated origins without the bridge bound what a hostile server shows.
