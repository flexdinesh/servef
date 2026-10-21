import type { DiffGrammar } from "./diff-grammars.ts"

// Shiki grammars by document family ID (internal/document.Builtins). Diffs
// always pass a language explicitly, so Pierre never guesses one from the file
// name. Families key the map rather than Chroma languages because TSX shares
// Chroma's TypeScript lexer but needs Shiki's TSX grammar. Every registered
// family needs an entry; `text` turns highlighting off (decision 0024).
const familyGrammars = {
  c: "c",
  cpp: "cpp",
  css: "css",
  diff: "diff",
  dockerfile: "docker",
  fish: "fish",
  go: "go",
  hcl: "hcl",
  html: "html",
  image: "text",
  ini: "ini",
  java: "java",
  javascript: "javascript",
  json: "json",
  jsx: "jsx",
  kotlin: "kotlin",
  lua: "lua",
  makefile: "make",
  markdown: "markdown",
  mermaid: "text",
  php: "php",
  powershell: "powershell",
  properties: "properties",
  python: "python",
  rst: "rst",
  ruby: "ruby",
  rust: "rust",
  scss: "scss",
  shell: "bash",
  sql: "sql",
  svg: "xml",
  swift: "swift",
  terraform: "terraform",
  text: "text",
  toml: "toml",
  tsx: "tsx",
  typescript: "typescript",
  xml: "xml",
  yaml: "yaml",
} satisfies Record<string, DiffGrammar | "text">

export type DiffLanguage = DiffGrammar | "text"

// A Map keeps IDs such as "constructor" from reaching prototype keys.
const grammars: ReadonlyMap<string, DiffLanguage> = new Map(Object.entries(familyGrammars))

// mappedFamilies lists the family IDs with an explicit grammar choice.
export const mappedFamilies: readonly string[] = [...grammars.keys()]

// diffLanguage maps a document family to its Shiki grammar.
export function diffLanguage(family: string): DiffLanguage {
  return grammars.get(family) ?? "text"
}
