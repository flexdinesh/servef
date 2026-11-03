import path from "node:path"
import { isRecord } from "../../../web/src/api/http.ts"

// Reveal in file manager (decision 0081): a local page names a project (its
// absolute ID) and a project-relative path; main reveals the file only when
// the project lies in a root the local daemon serves and the path, symlinks
// evaluated, stays inside the project.

export const MAX_PATH_LENGTH = 4096

export interface RevealRequest {
  project: string
  // path is project-relative with "/" separators.
  path: string
}

export type RevealOutcome = { ok: true; target: string } | { ok: false; reason: string }

// RealPath resolves symlinks of an existing path; it rejects otherwise.
export type RealPath = (name: string) => Promise<string>

export const NOT_SERVED = "The project is not in a folder servef serves."
export const PROJECT_UNAVAILABLE = "The project is unavailable."
export const PATH_MISSING = "The file no longer exists."
export const PATH_ESCAPES = "The path leads outside its project."

// isPlainAbsolute accepts an absolute, normalized path without NUL bytes or a
// trailing separator (the root itself excepted), within MAX_PATH_LENGTH.
export function isPlainAbsolute(name: string): boolean {
  return name.length <= MAX_PATH_LENGTH && !name.includes("\0") && path.isAbsolute(name)
    && path.normalize(name) === name && (name === path.parse(name).root || !name.endsWith(path.sep))
}

// relativeSegments splits a project-relative path into its segments: no
// leading "/", backslashes, empty, ".", or ".." segments, no NUL bytes. A
// directory's trailing "/" is allowed. null when invalid.
export function relativeSegments(name: string): string[] | null {
  if (name === "" || name.length > MAX_PATH_LENGTH || name.includes("\0") || name.includes("\\") || name.startsWith("/")) return null
  const segments = (name.endsWith("/") ? name.slice(0, -1) : name).split("/")
  return segments.every((segment) => segment !== "" && segment !== "." && segment !== "..") ? segments : null
}

// parseRevealRequest validates {project, path} from a renderer.
export function parseRevealRequest(value: unknown): RevealRequest | null {
  if (!isRecord(value)) return null
  const { project, path: relative } = value
  if (typeof project !== "string" || !isPlainAbsolute(project)) return null
  if (typeof relative !== "string" || relativeSegments(relative) === null) return null
  return { project, path: relative }
}

// within reports whether child is parent or below it; both are real paths.
export function within(parent: string, child: string): boolean {
  if (child === parent) return true
  const prefix = parent.endsWith(path.sep) ? parent : parent + path.sep
  return child.startsWith(prefix)
}

// servedProject resolves project's real path when it lies within one of the
// daemon's roots (real paths too); null otherwise. Roots that no longer
// resolve are skipped.
export async function servedProject(roots: readonly string[], project: string, realpath: RealPath): Promise<string | null> {
  if (!isPlainAbsolute(project)) return null
  let real: string
  try {
    real = await realpath(project)
  } catch {
    return null
  }
  for (const root of roots) {
    try {
      if (within(await realpath(root), real)) return real
    } catch {
      // A root missing on disk serves nothing.
    }
  }
  return null
}

// resolveRevealTarget returns the real path to reveal for request within the
// daemon's roots, or why it is refused.
export async function resolveRevealTarget(roots: readonly string[], request: RevealRequest, realpath: RealPath): Promise<RevealOutcome> {
  const segments = relativeSegments(request.path)
  if (segments === null) return { ok: false, reason: PATH_ESCAPES }
  try {
    await realpath(request.project)
  } catch {
    return { ok: false, reason: PROJECT_UNAVAILABLE }
  }
  const project = await servedProject(roots, request.project, realpath)
  if (project === null) return { ok: false, reason: NOT_SERVED }
  let target: string
  try {
    target = await realpath(path.join(project, ...segments))
  } catch {
    return { ok: false, reason: PATH_MISSING }
  }
  return within(project, target) ? { ok: true, target } : { ok: false, reason: PATH_ESCAPES }
}
