// Folder paths the OS hands the app (decision 0072): `--open-path <paths…>`
// argv from file manager actions, and second-instance additionalData.
import path from "node:path"
import { fileURLToPath } from "node:url"

export const OPEN_PATH_FLAG = "--open-path"
// MAX_OPEN_PATHS bounds folders per request: the daemon's open-level project cap (decision 0066).
export const MAX_OPEN_PATHS = 8

// InstanceData is what a second instance sends the first through the single-instance lock.
export interface InstanceData {
  openPaths: string[]
}

const URL_SCHEME = /^[A-Za-z][A-Za-z0-9+.-]*:\/\//

// parseOpenPaths returns absolute paths for the arguments after the first
// --open-path, up to the next argument starting with "--". file:// URLs with
// an empty or "localhost" host convert to paths; other URLs are dropped;
// plain arguments resolve against cwd. Deduplicated, order kept.
export function parseOpenPaths(argv: readonly string[], cwd: string): string[] {
  const start = argv.indexOf(OPEN_PATH_FLAG)
  if (start === -1) return []
  const paths: string[] = []
  for (const argument of argv.slice(start + 1)) {
    if (argument.startsWith("--")) break
    const resolved = resolveArgument(argument, cwd)
    if (resolved !== null && !paths.includes(resolved)) paths.push(resolved)
  }
  return paths
}

function resolveArgument(argument: string, cwd: string): string | null {
  if (argument === "") return null
  if (!URL_SCHEME.test(argument)) return path.resolve(cwd, argument)
  if (!argument.toLowerCase().startsWith("file:")) return null
  try {
    // Throws for hosts other than "" and localhost, and for encoded slashes.
    return path.resolve(fileURLToPath(argument))
  } catch {
    return null
  }
}

// openPathsOf validates second-instance additionalData: {openPaths: string[]}
// with absolute entries only; anything else yields [].
export function openPathsOf(data: unknown): string[] {
  if (typeof data !== "object" || data === null || !("openPaths" in data)) return []
  const paths: unknown = data.openPaths
  if (!Array.isArray(paths)) return []
  const valid: string[] = []
  for (const entry of paths) {
    if (typeof entry !== "string" || !path.isAbsolute(entry)) return []
    valid.push(entry)
  }
  return valid
}

// projectLanding is the landing URL of project on a servef-app origin.
export function projectLanding(origin: string, project: string): string {
  return `${origin}/?project=${encodeURIComponent(project)}`
}
