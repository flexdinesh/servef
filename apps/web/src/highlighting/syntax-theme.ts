import type { Theme } from "@pierre/highlights"

// SYNTAX_THEME is servef's Highlights theme: every syntax category resolves to
// a `--color-syntax-*` token, so highlighting follows the active servef theme
// without a palette per theme (decision 0029). Categories without an entry
// inherit from their nearest parent (`keyword.control` from `keyword`);
// unmapped ones use the foreground.
export const SYNTAX_THEME: Theme = {
  name: "servef",
  appearance: "dark",
  cssVariables: {
    prefix: "--color-syntax-",
    defaults: {
      foreground: "var(--color-text-primary)",
      background: "var(--color-surface)",
      inserted: "var(--color-success)",
      deleted: "var(--color-destructive)",
      link: "var(--color-accent)",
    },
  },
  style: {
    "editor.foreground": "foreground",
    "editor.background": "background",
    syntax: {
      comment: { color: "comment", font_style: "italic" },
      keyword: "keyword",
      preproc: "keyword",
      boolean: "keyword",
      operator: "operator",
      "keyword.operator": "operator",
      type: "type",
      constructor: "type",
      namespace: "type",
      tag: "type",
      selector: "type",
      "property.json_key": "type",
      function: "function",
      "constant.builtin": "function",
      "variable.special": "function",
      variable: "variable",
      "variable.parameter": "variable",
      property: "variable",
      attribute: "variable",
      label: "variable",
      string: "string",
      "text.literal": "string",
      number: "number",
      constant: "number",
      enum: "number",
      "diff.plus": "inserted",
      "diff.minus": "deleted",
      link_text: "link",
      link_uri: "link",
      emphasis: { font_style: "italic" },
      "emphasis.strong": { font_weight: 700 },
      title: { font_weight: 700 },
    },
  },
}
