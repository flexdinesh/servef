import type { Lang } from "@pierre/highlights"

// Highlights languages by document family ID (internal/document.Builtins).
// Diffs always pass a language explicitly, so Pierre never guesses one from
// the file name. Every registered family needs an entry; `text` turns
// highlighting off, also for families Highlights has no lexer for
// (decision 0029).
const familyLanguages = {
  c: "c",
  cpp: "cpp",
  css: "css",
  diff: "diff",
  dockerfile: "dockerfile",
  fish: "text",
  go: "go",
  hcl: "terraform",
  html: "html",
  image: "text",
  ini: "text",
  java: "java",
  javascript: "js",
  json: "json",
  jsx: "jsx",
  kotlin: "kotlin",
  lua: "lua",
  makefile: "makefile",
  markdown: "markdown",
  mermaid: "text",
  php: "php",
  powershell: "powershell",
  properties: "text",
  python: "python",
  rst: "text",
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
  typescript: "ts",
  xml: "xml",
  yaml: "yaml",
} satisfies Record<string, Lang>

// A Map keeps IDs such as "constructor" from reaching prototype keys.
const languages: ReadonlyMap<string, Lang> = new Map(Object.entries(familyLanguages))

// mappedFamilies lists the family IDs with an explicit language choice.
export const mappedFamilies: readonly string[] = [...languages.keys()]

// diffLanguage maps a document family to its Highlights language.
export function diffLanguage(family: string): Lang {
  return languages.get(family) ?? "text"
}
