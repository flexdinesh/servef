# 0037: Dark is the default theme

- Status: Accepted
- Date: 2026-10-24
- Supersedes: [0013](0013-ui-settings-and-base-font-size.md) theme default

## Context

The theme defaulted to System, so servef opened light on light-mode hosts.
servef is a code browser used beside dark editors and terminals; Dark is the
expected first look.

## Decision

- `DEFAULT_THEME` is Dark. A missing, unreadable, or unknown `servef-theme`
  resolves to Dark; Reset all and the Theme reset restore Dark.
- System stays an option and still follows the OS live. Stored choices,
  including `system`, are kept.

## Consequences

- New browsers, and anyone who never chose a theme, now see Dark regardless
  of OS mode; choosing System restores the old behavior.
- Theme is "Modified" only when it differs from Dark, so a stored `system`
  now shows as modified.
