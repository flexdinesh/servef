import { isRecord } from "../../api/http.ts"

// Files marked Viewed in review tabs (decision 0035), remembered in this
// browser under servef-review-viewed as project -> path -> blob. A mark
// counts only while the file's blob matches, so changed content unmarks
// itself. Pure except for readViewed/writeViewed, which never throw.

export const VIEWED_STORAGE_KEY = "servef-review-viewed"

const maxProjects = 50
const maxFiles = 2_000

export type ViewedMarks = Readonly<Record<string, Readonly<Record<string, string>>>>

function isBlob(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{40}$/.test(value)
}

export function parseViewed(value: unknown): ViewedMarks {
  if (!isRecord(value)) return {}
  const marks: Record<string, Record<string, string>> = {}
  for (const [project, files] of Object.entries(value)) {
    if (project === "__proto__" || !isRecord(files)) continue
    const kept: Record<string, string> = {}
    for (const [path, blob] of Object.entries(files)) {
      if (path !== "__proto__" && isBlob(blob)) kept[path] = blob
    }
    marks[project] = kept
  }
  return marks
}

// isViewed reports a file viewed at blob; an unknown blob is never viewed.
export function isViewed(marks: ViewedMarks, project: string, path: string, blob: string): boolean {
  if (!blob || !Object.hasOwn(marks, project)) return false
  const files = marks[project]
  return files !== undefined && Object.hasOwn(files, path) && files[path] === blob
}

// withViewed marks or unmarks a file. The most recent project and file go
// last, so the oldest drop first past the caps.
export function withViewed(marks: ViewedMarks, project: string, path: string, blob: string, viewed: boolean): ViewedMarks {
  const { [project]: files = {}, ...others } = marks
  const { [path]: _previous, ...rest } = files
  const kept = Object.entries(rest).slice(-(maxFiles - 1))
  const nextFiles = Object.fromEntries(viewed && blob ? [...kept, [path, blob]] : kept)
  return { ...Object.fromEntries(Object.entries(others).slice(-(maxProjects - 1))), [project]: nextFiles }
}

export function readViewed(): ViewedMarks {
  try {
    const raw = window.localStorage.getItem(VIEWED_STORAGE_KEY)
    return raw ? parseViewed(JSON.parse(raw)) : {}
  } catch {
    return {}
  }
}

export function writeViewed(marks: ViewedMarks) {
  try {
    window.localStorage.setItem(VIEWED_STORAGE_KEY, JSON.stringify(marks))
  } catch {
    // Storage is a convenience; marks still apply for this session.
  }
}
