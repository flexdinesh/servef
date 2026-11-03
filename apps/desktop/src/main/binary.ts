import { accessSync, constants, statSync } from "node:fs"
import path from "node:path"

// Locates the servef binary the app runs `daemon ensure` with. Never installs or copies.
export interface LocateInput {
  packaged: boolean
  resourcesPath: string
  env: NodeJS.ProcessEnv
  // onPath returns the absolute path of an executable on PATH, else null.
  onPath: (name: string) => string | null
}

export function locateServef(input: LocateInput): { path: string } | { missing: string } {
  if (input.packaged) return { path: path.join(input.resourcesPath, "bin", "servef") }
  const override = input.env.SERVEF_DESKTOP_BINARY
  if (override !== undefined && override !== "") return { path: override }
  const found = input.onPath("servef")
  if (found !== null) return { path: found }
  return { missing: "servef is not on PATH and SERVEF_DESKTOP_BINARY is not set." }
}

// findOnPath searches PATH for an executable file named name. Relative PATH
// entries are skipped so the result is always absolute.
export function findOnPath(name: string, env: NodeJS.ProcessEnv): string | null {
  for (const dir of (env.PATH ?? "").split(path.delimiter)) {
    if (!path.isAbsolute(dir)) continue
    const candidate = path.join(dir, name)
    if (isExecutableFile(candidate)) return candidate
  }
  return null
}

function isExecutableFile(file: string): boolean {
  try {
    if (!statSync(file).isFile()) return false
    accessSync(file, constants.X_OK)
    return true
  } catch {
    return false
  }
}
