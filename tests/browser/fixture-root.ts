import { realpathSync } from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"

// The browser tests serve testdata/markdown; projects are absolute paths.
export const fixtureRoot = realpathSync(fileURLToPath(new URL("../../testdata/markdown", import.meta.url)))

// projectParam is the URL query value naming a project below the fixture root.
export function projectParam(...segments: string[]): string {
  return encodeURIComponent(path.join(fixtureRoot, ...segments))
}

export const rootParam = projectParam()

// endsWith matches a URL ending in the given text.
export function endsWith(text: string): RegExp {
  return new RegExp(`${text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`)
}
