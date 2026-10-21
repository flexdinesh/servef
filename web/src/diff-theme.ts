import { registerCustomCSSVariableTheme } from "@pierre/diffs"

// servef's Shiki theme: every token color is a servef syntax token, so diffs
// follow the active theme without a Shiki theme per palette. The variables
// cross Pierre's shadow root by inheritance.
export const DIFF_THEME = "servef"

registerCustomCSSVariableTheme(DIFF_THEME, {
  foreground: "var(--color-text-primary)",
  background: "var(--color-surface)",
  "token-keyword": "var(--color-syntax-keyword)",
  "token-string": "var(--color-syntax-string)",
  "token-string-expression": "var(--color-syntax-string)",
  "token-comment": "var(--color-syntax-comment)",
  "token-function": "var(--color-syntax-function)",
  "token-constant": "var(--color-syntax-number)",
  "token-parameter": "var(--color-syntax-variable)",
  "token-punctuation": "var(--color-syntax-operator)",
  "token-link": "var(--color-accent)",
  "token-inserted": "var(--color-success)",
  "token-deleted": "var(--color-destructive)",
  "token-changed": "var(--color-warning)",
})
