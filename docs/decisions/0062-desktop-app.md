# 0062: Desktop app

- Status: Proposed
- Date: 2026-11-01

## Context

servef runs in a browser tab on a localhost port. Tabs mix with web browsing,
ports 7971 to 7980 shift between runs, and browser storage is origin-scoped,
so tabs, last project, and settings fragment across ports. Notifications and
native actions (reveal in file manager, open in editor) are unavailable. The
daemon (0061) gives one long-lived process to attach to; users also run CLI
servers and work on remote machines (0064).

## Decision

### Shell

- An Electron thin client over the existing embedded UI. No UI fork: the app
  loads the same build the browser does. An optional preload bridge exposes
  native actions; the UI feature-detects it and works without it.

### Connections

- The main process is a connection manager: the local daemon, running CLI
  servers found through the control registry (`internal/control`), and ssh
  remotes (0064).
- Each connection is proxied through a custom protocol,
  `servef-app://<connection>/`, so every server has one stable origin. Memory
  in localStorage survives port changes, and CSP `'self'` holds.

### Lifetime and notifications

- Closing the last window quits the app. macOS keeps its dock convention: the
  app stays until Cmd-Q.
- Notifications (a new commit or branch switch, a walkthrough published,
  edits settled after a quiet period) only while the app runs, through
  watched leases (0061). There is no background agent.

### Nothing installed

- The app never installs or copies anything, locally or on a remote. When
  something is missing (servef on PATH, a remote binary, a protocol match),
  it shows host-specific instructions and a Retry, as language-server install
  guidance does (0018).

### Revisions when accepted

- Scopes the AGENTS.md Distribution rules: the CLI stays one self-contained
  Go binary; the desktop app is a separate artifact that bundles it and ships
  Chromium and Node at runtime.
- Revises 0021: release builds add per-OS app packages beside the tag and
  `edge` Go installs.

## Consequences

- Signing and notarization, auto-update, and per-OS packaging are new release
  costs.
- Electron memory is measured separately from servef RSS and language-server
  RSS; the memory priority for servef itself is unchanged.
- The browser remains a supported client (`servef browser`, 0060); features
  that need the bridge degrade there.
