# 0014: Command palette

- Status: Accepted
- Date: 2026-10-15

## Context

servef follows VS Code conventions. Settings are reachable from a footer gear
or Cmd/Ctrl+,, but there was no keyboard-first way to jump to one setting, and
no general home for commands as they grow.

## Decision

- Cmd/Ctrl+Shift+P or F1 opens a command palette that looks and behaves like
  VS Code's: `>` prompt, filter as you type, arrows, Enter, Escape.
- Commands are data (`web/src/commands.ts`): a category, a title, and an
  action. Matching is pure and unit-tested: every term must appear in the
  category or title; title word-prefix matches rank first.
- For now the commands are `Preferences › Open Settings` (showing its
  shortcut), then each setting as `Settings › <Title>` with its current value. Running one opens Settings, scrolls to the setting,
  focuses its control, and briefly highlights it.
- F1 is offered because some browsers reserve Shift+Cmd/Ctrl+P.

## Consequences

- New commands add an action kind and a handler in `App`; the palette UI does
  not change.
- File search keeps Cmd/Ctrl+P/K; the palette has no `>`-less file mode.
