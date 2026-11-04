// web-highlights.ts writes what the web app renders for each Highlights
// sample (packages/mygo/highlights/testdata/samples, <name>.<language>):
// highlightLines' rows from apps/web/src/highlighting/syntax-highlight.ts
// with its SYNTAX_THEME, or null for plain text. The native highlight
// package's test renders its spans the same way and compares, and checks
// the hashes of the web files it mirrors.
//
//   node apps/native/scripts/web-highlights.ts
import { createHash } from "node:crypto"
import { readdirSync, readFileSync, writeFileSync } from "node:fs"
import { createRequire } from "node:module"
import { dirname, join, relative } from "node:path"
import { fileURLToPath, pathToFileURL } from "node:url"

import { highlightLines } from "../../web/src/highlighting/syntax-highlight.ts"

const repo = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..")
const require = createRequire(join(repo, "apps", "web", "package.json"))
const highlights = await import(pathToFileURL(require.resolve("@pierre/highlights")).href)

const mirrored = [
  "apps/web/src/highlighting/syntax-theme.ts",
  "apps/web/src/highlighting/syntax-highlight.ts",
  "apps/web/src/lib/line-selection.ts",
]
const inputs = Object.fromEntries(mirrored.map((path) => [path, createHash("sha256").update(readFileSync(join(repo, path))).digest("hex")]))

const samples = join(repo, "packages", "mygo", "highlights", "testdata", "samples")
const rows: Record<string, string[] | null> = {}
for (const file of readdirSync(samples).sort()) {
  const language = file.slice(file.lastIndexOf(".") + 1)
  rows[file] = highlightLines(highlights, readFileSync(join(samples, file), "utf8"), language)
}
const out = join(repo, "apps", "native", "internal", "highlight", "testdata", "web-rows.json")
writeFileSync(out, `${JSON.stringify({ inputs, rows }, null, 1)}\n`)
console.log(`${relative(repo, out)}: ${Object.keys(rows).length} samples`)
