# 0064: SSH remotes

- Status: Proposed
- Date: 2026-11-01

## Context

Users and agents work on remote machines over SSH. Reading remote files over
SFTP would duplicate traversal, git, and language-server work locally and
send every byte across the network. servef already runs anywhere a Go binary
does, and the daemon (0061) speaks over a Unix socket that SSH can forward.

## Decision

### Transport

- Remote work runs servef on the remote; the app never reads files over SFTP.
- The app runs the system `ssh` binary, so `~/.ssh/config`, the agent,
  ProxyJump, and ControlMaster apply. Aliases come from `ssh -G` and `Host`
  entries.
- Connect: `ssh <alias> servef daemon ensure --json` (socket path, protocol
  version, roots), then `ssh -N -L <local socket>:<remote socket>`. The
  protocol proxy serves the forwarded socket as `servef-app://ssh-<alias>/`
  (0062).
- The remote daemon is ephemeral: it exits when idle (0061) and nothing
  persistent is installed.

### Host identity

- `servef remote` links (0063) carry hints: `user`, `hostname`, the
  `SSH_CONNECTION` server address, and an optional `--alias`.
- The app resolves a link to a known connection first, then to an `ssh -G`
  alias whose HostName matches, else asks once and remembers the answer.

### Missing or mismatched servef

- When servef is missing on the remote or its protocol version does not
  match, the app shows install instructions (Homebrew, `go install`, release
  download) and Retry. It never uploads binaries (0062).

### Notifications

- Notifications from a remote arrive only while connected; the UI says so.
- Agents on the remote publishing walkthroughs (`servef walkthrough
  publish`) notify the local app while connected.

## Consequences

- Remote browsing needs servef installed on the remote with a matching
  protocol version.
- Forwarding Unix sockets needs OpenSSH 6.7 or later on both ends.
- Latency is one SSH round trip per request; traversal, git, and language
  servers run beside the files.
