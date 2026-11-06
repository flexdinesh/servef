# 0099: Desktop notifications

- Status: Accepted
- Date: 2026-11-05
- Revises: [0095](0095-desktop-app-is-mygo-electron-removed.md) (notifications
  were not ported), [0098](0098-remote-connections.md) (a remote page's bridge
  also has `configureNotifications`),
  [0066](0066-lease-levels-and-desktop-events.md) (`head` carries `parent`;
  project streams carry walkthroughs)

## Context

The desktop app is the main UI and shows nothing while a window is hidden: the
page closes its event stream then, which is exactly when a notice matters.
Agents commit, switch branches, edit files, and publish walkthroughs in projects
the user is not looking at, on this computer or on a server opened with
`srcz connect`. Local and remote must behave the same; they differ only in
transport (Unix socket, pinned TLS).

## Decision

**One notifier per connection window.** `apps/desktop/internal/notifier` holds
its own `/api/events` streams through `client.Client`, so one code path serves
both transports. It starts with the page's first
`window.srczDesktop.configureNotifications` call and stops when the window
closes. A closed window ends its streams, so a `--remote` server still stops when
its last UI goes (0098). It knows no mygo: the app shows what it decides.

**The page chooses, the app decides.** Settings live with the page's other
settings, so they are per connection: each window has its own origin and storage.
The page sends `{project, commits, branches, edits, walkthroughs}` on load, on
change, and on focus. The Notifications category and its palette commands exist
only where the bridge has the method, so browsers never show them.

| Setting | Default | Fires when | Lease |
| --- | --- | --- | --- |
| New Commits | On | HEAD moves on the same branch | `watched` |
| Branch Switches | On | HEAD changes branch or detaches | `watched` |
| File Edits | Off | edits have stopped for 10 seconds | `open`, current project only |
| Walkthroughs | On | one is published or updated | `watched` |

Git and walkthrough kinds follow the projects opened in this run, 16 at most per
connection, within the server's watched cap (0066). File Edits follows only the
project the page shows, because an `open` lease keeps a watcher on its folders.
All kinds off means no streams at all.

**What counts.**

- The first head and first walkthrough list are a baseline. Each later value is
  compared with what the notifier last saw, not with the stream's, so a reconnect
  reports once what it missed.
- A head whose `parent` is the last commit is "New commit"; any other move is
  "main moved".
- In a repository, edits report only when the set of changed paths differs from
  the last settled set, so ignored build output and clean trees say nothing.
  Editing an already-dirty file does not change the set and says nothing. A folder
  without git has no set: every path counts.
- A focused window suppresses notices for the project it shows, without spending
  the limits. Other projects still notify.

**A server is not trusted with the user's attention.** Each project and kind gets
one notice per 30 seconds, each connection 10 per minute, whatever a server sends.
Server text is stripped of control and format characters, truncated, and escaped
for Linux notification services, which read markup. A remote notice's body starts
with the connection's label, so a server cannot pass as this computer. A notice ID
is `<connection>/<project hash>/<kind>`: it carries no path or secret, and a newer
notice replaces the one it succeeds. A click loads a web app route on the
connection's own origin; a notice from an earlier run only raises the app.

**Failures.** A stream that ends reconnects with backoff to 30 seconds; a local
daemon that has gone is ensured again. A wrong fingerprint, a refused token, or a
forgotten connection stops every stream until the page next configures, since
retrying cannot fix them. A project the server no longer has is forgotten. When
the OS has no notification service or has denied them, Settings says so.

**Transport changes.** `head` adds `parent`, the first parent's hash, so a client
can tell a new commit from a reset without walking history. Project streams carry
`walkthrough` events, so a remote app needs no daemon control stream, which a TCP
listener does not serve. An older server sends neither: moves read as "moved" and
walkthroughs are found at the next reconnect.

## Consequences

- Each followed project costs a stream, and File Edits a watcher. Git kinds never
  list a project's tree.
- Notices are per window: with no window on a connection, nothing watches it.
- Linux notices depend on a notification service; macOS asks permission on first
  show. macOS behavior is untested in CI, which has no notification center.
- End-to-end builds replace the display and focus through `notices_e2e.go`, under
  the `e2e` tag, because Xvfb has neither a service nor a window manager.
